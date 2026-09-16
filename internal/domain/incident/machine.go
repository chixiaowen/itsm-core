// 本文件声明 incident 域状态机表与 guard 函数（ARCHITECTURE §6.3.2）。
package incident

import (
	"fmt"
	"sort"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
	"github.com/chixiaowen/itsm-core/internal/pkg/statemachine"
)

// escalateRoles 是「升级」动作允许的角色（PRD §5.3：agent, resolver, problem_manager, admin）。
//
// 问题经理持有 perm.incident.escalate 且路由放行，状态机必须同步放行
// （问题管理需能主动升级关联事件），否则出现「路由授权与状态机授权不一致」。
var escalateRoles = []string{role.Agent, role.Resolver, role.ProblemManager, role.Admin}

// incidentMachine 是事件状态机（from -> action -> Transition）。
var incidentMachine = statemachine.Table{
	StatusReported: {
		ActionTriage: {To: StatusTriage, Roles: role.Agents},
		ActionCancel: {To: StatusCancelled, Roles: []string{role.Requestor}, Guard: guardOwner},
	},
	StatusTriage: {
		ActionConfirm:  {To: StatusInProgress, Roles: role.Agents, Guard: guardAssigneeSet},
		ActionFalsePos: {To: StatusResolved, Roles: role.Agents, Guard: guardSolution},
		ActionCancel:   {To: StatusCancelled, Roles: role.Agents, Guard: guardReason},
	},
	StatusInProgress: {
		ActionEscalate: {To: StatusEscalated, Roles: escalateRoles, Guard: guardEscalate},
		// 挂起/解决：被指派人本人 OR agent OR admin（PRD §5.3 触发角色列为「assignee, agent」）。
		ActionPending: {To: StatusPending, Roles: role.Resolvers, Guard: composeGuards(guardAssigneeOrAgent, guardReason)},
		ActionResolve: {To: StatusResolved, Roles: role.Resolvers, Guard: composeGuards(guardAssigneeOrAgent, guardResolve)},
	},
	StatusEscalated: {
		// 接手处理：被指派人本人 OR admin（PRD §5.3 触发角色为 assignee）。
		ActionTakeOver: {To: StatusInProgress, Roles: role.Resolvers, Guard: guardAssigneeOrAdmin},
	},
	StatusPending: {
		// 恢复/解决：被指派人本人 OR agent OR admin。
		ActionResume:  {To: StatusInProgress, Roles: role.Resolvers, Guard: guardAssigneeOrAgent},
		ActionResolve: {To: StatusResolved, Roles: role.Resolvers, Guard: composeGuards(guardAssigneeOrAgent, guardResolve)},
	},
	StatusResolved: {
		ActionClose:  {To: StatusClosed, Roles: role.Agents, Guard: guardCloseReview},
		ActionRevert: {To: StatusInProgress, Roles: role.Agents, Guard: guardRevertWindow},
	},
}

// illegalTransitionMsg 构造跨域统一的「非法流转」409 消息文本（对齐 docs/API.md）：
//
//	非法流转 current=<from> -> target=<to>: <原因>
//
// 仅用于充实 message，不改变 HTTP 状态码（调用方仍用 httpx.ErrConflict → 409）。
func illegalTransitionMsg(from, action, reason string) string {
	return fmt.Sprintf("非法流转 current=%s -> target=%s: %s", from, resolveTarget(action), reason)
}

// resolveTarget 依据动作在事件状态机表中的语义解析其目标状态。
//
// 当 (from, action) 组合非法时，仍可从其它合法起始状态解析该动作的规范目标状态，
// 供 409 消息回显「当前状态 -> 目标状态」；未知动作原样兜底。
// 按状态名排序遍历，保证同一动作得到确定结果（每个动作在表中只指向唯一目标）。
func resolveTarget(action string) string {
	statuses := make([]string, 0, len(incidentMachine))
	for s := range incidentMachine {
		statuses = append(statuses, s)
	}
	sort.Strings(statuses)
	for _, s := range statuses {
		if tr, ok := incidentMachine[s][action]; ok {
			return tr.To
		}
	}
	return action
}

// guardAssigneeOrAgent 资源级校验：被指派人本人、agent 或 admin 放行（PRD「assignee, agent」）。
func guardAssigneeOrAgent(in statemachine.GuardInput) error {
	if in.Actor.Role == role.Agent || in.Actor.Role == role.Admin {
		return nil
	}
	it := in.Entity.(*Incident)
	if it.AssigneeID != nil && *it.AssigneeID == in.Actor.UserID {
		return nil
	}
	return httpx.ErrForbidden("仅被指派人、坐席或管理员可执行该操作")
}

// guardAssigneeOrAdmin 资源级校验：被指派人本人或 admin 放行（PRD「assignee, admin」）。
func guardAssigneeOrAdmin(in statemachine.GuardInput) error {
	if in.Actor.Role == role.Admin {
		return nil
	}
	it := in.Entity.(*Incident)
	if it.AssigneeID != nil && *it.AssigneeID == in.Actor.UserID {
		return nil
	}
	return httpx.ErrForbidden("仅被指派人或管理员可执行该操作")
}

// composeGuards 顺序执行多个 guard，返回首个错误（用于「资源级角色校验 + 前置条件」组合）。
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

// guardOwner 校验操作者为本事件上报人（admin 放行）。
func guardOwner(in statemachine.GuardInput) error {
	it := in.Entity.(*Incident)
	if in.Actor.Role == role.Admin {
		return nil
	}
	if it.ReporterID == in.Actor.UserID {
		return nil
	}
	return httpx.ErrForbidden("仅本人上报的事件可执行该操作")
}

// guardAssigneeSet 校验指派人非空。
func guardAssigneeSet(in statemachine.GuardInput) error {
	if assigneeOf(in) == 0 {
		return httpx.ErrPrecondition("确认事件须指定 assignee_id")
	}
	return nil
}

// guardSolution 校验解决方案非空。
func guardSolution(in statemachine.GuardInput) error {
	if solutionOf(in) == "" {
		return httpx.ErrPrecondition("解决方案必填")
	}
	return nil
}

// guardReason 校验原因非空。
func guardReason(in statemachine.GuardInput) error {
	if reasonOf(in) == "" {
		return httpx.ErrPrecondition("原因必填")
	}
	return nil
}

// guardEscalate 校验升级：原因非空；级别只能 +1（越级/超上限 409）。
func guardEscalate(in statemachine.GuardInput) error {
	it := in.Entity.(*Incident)
	if reasonOf(in) == "" {
		return httpx.ErrPrecondition("升级原因必填")
	}
	target := it.EscalationLevel + 1
	if lv, ok := in.Params["level"].(int); ok && lv != 0 {
		target = lv
	}
	if it.EscalationLevel >= MaxEscalationLevel {
		return httpx.ErrConflict("已达最高升级级别，无法继续升级")
	}
	if target != it.EscalationLevel+1 {
		return httpx.ErrConflict("升级级别不可越级，只能 +1")
	}
	return nil
}

// guardResolve 校验解决：solution 非空；P1/P2 还须填写影响与恢复说明（reason）。
func guardResolve(in statemachine.GuardInput) error {
	it := in.Entity.(*Incident)
	if solutionOf(in) == "" {
		return httpx.ErrPrecondition("解决方案必填")
	}
	if (it.Priority == PriorityP1 || it.Priority == PriorityP2) && reasonOf(in) == "" {
		return httpx.ErrPrecondition("P1/P2 事件解决须填写影响与恢复说明")
	}
	return nil
}

// guardCloseReview 校验关闭：P1/P2 事件须填写复盘结论。
func guardCloseReview(in statemachine.GuardInput) error {
	it := in.Entity.(*Incident)
	if it.Priority == PriorityP1 || it.Priority == PriorityP2 {
		if s, _ := in.Params["review_conclusion"].(string); s == "" {
			return httpx.ErrPrecondition("P1/P2 事件关闭须填写复盘结论")
		}
	}
	return nil
}

// guardRevertWindow 校验回退窗口（距 resolved_at ≤ 24h，否则 409）。
func guardRevertWindow(in statemachine.GuardInput) error {
	it := in.Entity.(*Incident)
	if it.ResolvedAt == nil || in.Now.Sub(*it.ResolvedAt) > RevertWindow {
		return httpx.ErrConflict("超过 24 小时不可回退")
	}
	return nil
}

func solutionOf(in statemachine.GuardInput) string {
	s, _ := in.Params["solution"].(string)
	return s
}

func reasonOf(in statemachine.GuardInput) string {
	s, _ := in.Params["reason"].(string)
	return s
}

func assigneeOf(in statemachine.GuardInput) uint64 {
	v, _ := in.Params["assignee_id"].(uint64)
	return v
}
