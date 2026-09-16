// 本文件声明 change 域状态机表、guard 函数与 CAB 会签判定纯函数（ARCHITECTURE §6.3.4 / §6.4）。
package change

import (
	"fmt"
	"sort"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
	"github.com/chixiaowen/itsm-core/internal/pkg/statemachine"
)

// creatorRoles 是「变更申请人/建单者」主体角色集合 = 持有 perm.change.submit 的全部角色 + admin。
//
// PRD §5.5「提交风险评估」的触发角色写作「requester(工程师)」，即变更创建者；
// 而 role.requestor 本身不具备 perm.change.submit、无法创建变更，故此处必须取
// 「能创建变更（持 perm.change.submit）的角色」——agent/resolver/problem_manager/
// change_manager/cmdb_manager 及 admin——否则创建者在该必经步骤会被状态机拒（403）。
var creatorRoles = []string{
	role.Agent, role.Resolver, role.ProblemManager, role.ChangeManager, role.CmdbManager, role.Admin,
}

// changeMachine 是变更状态机（from -> action -> Transition）。
//
// 早期生命周期动作（提交风险评估/免审直通/提交审批/驳回修改/回滚重提/取消）的触发主体
// 写作「requester(工程师)」= 变更创建者本人（Change.RequesterID）。因 role.requestor 不持有
// perm.change.submit、无法创建变更，故 Roles 一律用 creatorRoles（= 持 perm.change.submit 者 + admin），
// 再由 guardCreatorOrManager 按「资源」（创建者本人 / 变更经理 / 管理员）收窄到具体主体。
// 注：change 域的 Roles 中不再出现 role.Requestor（对该域而言它是不可达的死条目）。
var changeMachine = statemachine.Table{
	StatusDraft: {
		// 提交风险评估：建单者集合（agent/resolver/change_manager）+ admin，PRD §5.5。
		ActionSubmitAssessment: {To: StatusAssessment, Roles: creatorRoles, Guard: guardRequiredFields},
		// 标准变更免审直通：创建者本人 / 变更经理 / 管理员。
		ActionPreAuthorize: {To: StatusApproved, Roles: creatorRoles, Guard: composeGuards(guardStandardOnly, guardCreatorOrManager)},
		ActionCancel:       {To: StatusCancelled, Roles: creatorRoles, Guard: guardCreatorOrManager},
	},
	StatusAssessment: {
		// 提交 CAB 审批：creatorRoles 已含 change_manager 与 admin。
		ActionSubmitApproval: {To: StatusPendingApproval, Roles: creatorRoles, Guard: composeGuards(guardPlanAndWindow, guardCreatorOrManager)},
		ActionCancel:         {To: StatusCancelled, Roles: creatorRoles, Guard: guardCreatorOrManager},
	},
	StatusPendingApproval: {
		ActionApprove: {To: StatusApproved, Roles: role.ChangeManagers, Guard: guardApprovalsPassed},
		ActionReject:  {To: StatusRejected, Roles: role.ChangeManagers, Guard: guardRejectComment},
	},
	StatusRejected: {
		ActionRevise: {To: StatusDraft, Roles: creatorRoles, Guard: guardCreatorOrManager},
	},
	StatusApproved: {
		ActionSchedule: {To: StatusScheduled, Roles: role.ChangeManagers, Guard: guardWindowOrder},
	},
	StatusScheduled: {
		ActionStartImplement: {To: StatusImplementing, Roles: []string{role.Resolver, role.Admin}, Guard: guardEnforceWindow},
		ActionCancel:         {To: StatusCancelled, Roles: role.ChangeManagers},
	},
	StatusImplementing: {
		ActionComplete: {To: StatusImplemented, Roles: []string{role.Resolver, role.Admin}, Guard: guardResult},
		ActionRollback: {To: StatusRolledBack, Roles: []string{role.Resolver, role.Admin}, Guard: guardRollbackReason},
	},
	StatusImplemented: {
		ActionReview: {To: StatusReview, Roles: role.ChangeManagers},
	},
	StatusReview: {
		ActionClose:    {To: StatusClosed, Roles: role.ChangeManagers, Guard: guardConclusion},
		ActionRollback: {To: StatusRolledBack, Roles: role.ChangeManagers, Guard: guardRollbackReason},
	},
	StatusRolledBack: {
		ActionResubmit: {To: StatusDraft, Roles: creatorRoles, Guard: guardCreatorOrManager},
	},
}

// guardCreatorOrManager 按资源收窄「早期生命周期动作」的执行主体：变更创建者本人、变更经理、管理员。
//
// PRD §5.5 把这类动作的触发角色写作「requester(工程师)」，即 Change.RequesterID 本人；
// 而 role.requestor 不持有 perm.change.submit、无法创建变更，故不能按 role.Requestor 判定
// ——否则创建者本人会被状态机拒成 403（集成验证发现）。
func guardCreatorOrManager(in statemachine.GuardInput) error {
	ch, ok := in.Entity.(*Change)
	if !ok {
		return httpx.ErrInternal("guardCreatorOrManager: 实体类型异常")
	}
	a := in.Actor
	if a.UserID == ch.RequesterID { // 创建者本人
		return nil
	}
	if a.Role == role.ChangeManager || a.Role == role.Admin { // 可代推进
		return nil
	}
	return httpx.ErrForbidden("仅变更创建者本人、变更经理或管理员可推进该步骤")
}

// composeGuards 顺序执行多个 guard，返回首个错误（用于「前置条件 + 资源级主体校验」组合）。
func composeGuards(guards ...statemachine.Guard) statemachine.Guard {
	return func(in statemachine.GuardInput) error {
		for _, g := range guards {
			if g == nil {
				continue
			}
			if err := g(in); err != nil {
				return err
			}
		}
		return nil
	}
}

// illegalTransitionMsg 构造跨域统一的「非法流转」409 消息文本（对齐 docs/API.md）：
//
//	非法流转 current=<from> -> target=<to>: <原因>
//
// 仅用于充实 message，不改变 HTTP 状态码（调用方仍用 httpx.ErrConflict → 409）。
func illegalTransitionMsg(from, action, reason string) string {
	return fmt.Sprintf("非法流转 current=%s -> target=%s: %s", from, resolveTarget(action), reason)
}

// resolveTarget 依据动作在变更状态机表中的语义解析其目标状态。
//
// 当 (from, action) 组合非法时，仍可从其它合法起始状态解析该动作的规范目标状态，
// 供 409 消息回显「当前状态 -> 目标状态」；未知动作原样兜底。
// 按状态名排序遍历，保证同一动作得到确定结果（每个动作在表中只指向唯一目标）。
func resolveTarget(action string) string {
	statuses := make([]string, 0, len(changeMachine))
	for s := range changeMachine {
		statuses = append(statuses, s)
	}
	sort.Strings(statuses)
	for _, s := range statuses {
		if tr, ok := changeMachine[s][action]; ok {
			return tr.To
		}
	}
	return action
}

// EvaluateApprovals 判定 CAB 会签结果（纯函数，可独立单测）。
//
//   - 存在任一 rejected → rejected；
//   - 紧急变更（ECAB）：≥1 名审批人通过即 approved；
//   - 其它（普通/标准）：默认需 2 名审批人全部通过才 approved；
//   - 否则返回 ""（待审）。
//
// 加固：按 ApproverID **去重**计数，即使传入含重复 ApproverID 的脏数据
// （例如绕过唯一索引的历史数据），同一名审批人也只算一票，杜绝单人凑票。
func EvaluateApprovals(changeType string, approvals []ChangeApproval) string {
	if len(approvals) == 0 {
		return ""
	}
	approvedBy := make(map[uint64]bool)
	rejected := false
	for _, a := range approvals {
		switch a.Decision {
		case DecisionApprove:
			approvedBy[a.ApproverID] = true
		case DecisionReject:
			rejected = true
		}
	}
	if rejected {
		return DecisionReject
	}
	if len(approvedBy) >= requiredApprovals(changeType) {
		return DecisionApprove
	}
	return ""
}

// requiredApprovals 返回通过所需的最少审批人数。
func requiredApprovals(changeType string) int {
	if changeType == TypeEmergency {
		return 1
	}
	return 2
}

// guardRequiredFields 校验提交风险评估的必填字段。
func guardRequiredFields(in statemachine.GuardInput) error {
	ch := in.Entity.(*Change)
	if ch.Title == "" || ch.ChangeType == "" || ch.RiskLevel == "" {
		return httpx.ErrPrecondition("标题/类型/风险等级必填")
	}
	return nil
}

// guardStandardOnly 校验标准变更预授权路径（非 standard 409）。
func guardStandardOnly(in statemachine.GuardInput) error {
	ch := in.Entity.(*Change)
	if ch.ChangeType != TypeStandard {
		return httpx.ErrConflict("仅标准变更可免审批直通 approved")
	}
	if ch.Plan == "" || ch.RollbackPlan == "" {
		return httpx.ErrPrecondition("标准变更仍须填写实施与回滚方案")
	}
	return nil
}

// guardPlanAndWindow 校验提交审批：plan/rollback_plan 非空、窗口顺序正确。
func guardPlanAndWindow(in statemachine.GuardInput) error {
	ch := in.Entity.(*Change)
	if ch.Plan == "" || ch.RollbackPlan == "" {
		return httpx.ErrPrecondition("提交审批前 plan 与 rollback_plan 不可为空")
	}
	if ch.WindowStart != nil && ch.WindowEnd != nil && !ch.WindowStart.Before(*ch.WindowEnd) {
		return httpx.ErrPrecondition("变更窗口 window_start 必须早于 window_end")
	}
	return nil
}

// guardApprovalsPassed 校验会签结果（由 service 计算后写入 params）。
func guardApprovalsPassed(in statemachine.GuardInput) error {
	if v, _ := in.Params["approvals_decision"].(string); v != DecisionApprove {
		return httpx.ErrPrecondition("CAB 未通过，不可审批通过")
	}
	return nil
}

// guardRejectComment 校验驳回意见非空。
func guardRejectComment(in statemachine.GuardInput) error {
	if v, _ := in.Params["comment"].(string); v == "" {
		return httpx.ErrPrecondition("驳回须填写审批意见")
	}
	return nil
}

// guardWindowOrder 校验收窗口顺序（window_start < window_end）。
func guardWindowOrder(in statemachine.GuardInput) error {
	ch := in.Entity.(*Change)
	if ch.WindowStart == nil || ch.WindowEnd == nil {
		return httpx.ErrPrecondition("排期须设置变更窗口 window_start/window_end")
	}
	if !ch.WindowStart.Before(*ch.WindowEnd) {
		return httpx.ErrPrecondition("window_start 必须早于 window_end")
	}
	return nil
}

// guardEnforceWindow 校验实施窗口（强制开关开启时 now 须在窗口内；紧急变更可确认后窗口外实施）。
func guardEnforceWindow(in statemachine.GuardInput) error {
	ch := in.Entity.(*Change)
	enforce, _ := in.Params["enforce_window"].(bool)
	if !enforce {
		return nil
	}
	if ch.WindowStart == nil || ch.WindowEnd == nil {
		return httpx.ErrPrecondition("未设置变更窗口，无法实施")
	}
	inWindow := !in.Now.Before(*ch.WindowStart) && !in.Now.After(*ch.WindowEnd)
	if inWindow {
		return nil
	}
	// 紧急变更允许窗口外实施（需 change_manager 显式确认）。
	if ch.ChangeType == TypeEmergency {
		if confirm, _ := in.Params["confirm_out_of_window"].(bool); confirm {
			return nil
		}
		return httpx.ErrPrecondition("紧急变更窗口外实施须显式确认")
	}
	return httpx.ErrConflict("当前时间不在变更窗口内")
}

// guardResult 校验实施结论非空。
func guardResult(in statemachine.GuardInput) error {
	if v, _ := in.Params["result"].(string); v == "" {
		return httpx.ErrPrecondition("实施成功须填写实施结论")
	}
	return nil
}

// guardRollbackReason 校验回滚原因非空。
func guardRollbackReason(in statemachine.GuardInput) error {
	if v, _ := in.Params["reason"].(string); v == "" {
		return httpx.ErrPrecondition("回滚须填写原因")
	}
	return nil
}

// guardConclusion 校验回顾结论非空。
func guardConclusion(in statemachine.GuardInput) error {
	if v, _ := in.Params["conclusion"].(string); v == "" {
		return httpx.ErrPrecondition("关闭须填写回顾结论")
	}
	return nil
}
