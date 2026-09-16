// 本文件声明 ticket 域状态机表与 guard 函数（ARCHITECTURE §6.3.1 / §6.5）。
//
// 未在表中列出的流转一律由 service 转成 409；guard 返回的 *httpx.AppError
// 决定最终 HTTP 状态（422 前置条件不满足 / 403 越权 / 409 状态冲突）。
package ticket

import (
	"fmt"
	"sort"
	"time"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
	"github.com/chixiaowen/itsm-core/internal/pkg/statemachine"
)

// ticketMachine 是工单状态机（from -> action -> Transition）。
//
// closed / cancelled 不出现在表中 → 全部流转被 409 拒绝（PRD 非法流转示例）。
var ticketMachine = statemachine.Table{
	StatusDraft: {
		ActionSubmit: {To: StatusNew, Roles: role.Agents, Guard: guardRequiredFields},
		ActionCancel: {To: StatusCancelled, Roles: role.Agents},
	},
	StatusNew: {
		ActionAssign: {To: StatusAssigned, Roles: role.Agents, Guard: guardAssigneeValid},
		ActionCancel: {To: StatusCancelled, Roles: []string{role.Requestor}, Guard: guardOwner},
	},
	StatusAssigned: {
		// 开始处理：被指派人本人或 admin（PRD「assignee, admin」）。
		ActionStart: {To: StatusInProgress, Roles: role.Resolvers, Guard: guardAssigneeOrAdmin},
		// 退回未指派：被指派人本人、agent 或 admin（PRD「assignee, agent」）。
		ActionReturn: {To: StatusNew, Roles: role.Resolvers, Guard: guardAssigneeOrAgent},
	},
	StatusInProgress: {
		// 挂起/解决：被指派人本人、agent 或 admin（PRD「assignee, agent」；非被指派人的 resolver 被拒）。
		ActionPending: {To: StatusPending, Roles: role.Resolvers, Guard: composeGuards(guardAssigneeOrAgent, guardPauseReason)},
		ActionResolve: {To: StatusResolved, Roles: role.Resolvers, Guard: composeGuards(guardAssigneeOrAgent, guardSolution)},
	},
	StatusPending: {
		// 恢复/解决：被指派人本人、agent 或 admin。
		ActionResume:  {To: StatusInProgress, Roles: role.Resolvers, Guard: guardAssigneeOrAgent},
		ActionResolve: {To: StatusResolved, Roles: role.Resolvers, Guard: composeGuards(guardAssigneeOrAgent, guardSolution)},
	},
	StatusResolved: {
		ActionClose:  {To: StatusClosed, Roles: []string{role.Requestor, role.Agent, role.Admin}},
		ActionReopen: {To: StatusReopened, Roles: []string{role.Requestor}, Guard: guardReopenWindow},
	},
	StatusReopened: {
		ActionAssign: {To: StatusAssigned, Roles: role.Agents, Guard: guardAssigneeValid},
		// 继续处理：被指派人本人、agent 或 admin（PRD「agent, assignee」）。
		ActionStart: {To: StatusInProgress, Roles: role.Resolvers, Guard: guardAssigneeOrAgent},
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

// resolveTarget 依据动作在工单状态机表中的语义解析其目标状态。
//
// 当 (from, action) 组合非法（statemachine.Resolve ok=false）时，仍可从其它合法起始状态
// 解析出该动作的规范目标状态，供 409 消息回显「当前状态 -> 目标状态」；未知动作原样兜底。
// 按状态名排序遍历，保证同一动作得到确定结果（每个动作在表中只指向唯一目标）。
func resolveTarget(action string) string {
	statuses := make([]string, 0, len(ticketMachine))
	for s := range ticketMachine {
		statuses = append(statuses, s)
	}
	sort.Strings(statuses)
	for _, s := range statuses {
		if tr, ok := ticketMachine[s][action]; ok {
			return tr.To
		}
	}
	return action
}

// guardRequiredFields 校验提交草稿的必填字段。
func guardRequiredFields(in statemachine.GuardInput) error {
	tk := in.Entity.(*Ticket)
	if tk.Title == "" || tk.RequesterID == 0 {
		return httpx.ErrPrecondition("标题与请求人必填")
	}
	return nil
}

// guardAssigneeValid 校验指派参数存在且非零（有效性由 service 通过用户目录二次校验）。
func guardAssigneeValid(in statemachine.GuardInput) error {
	id, ok := in.Params["assignee_id"].(uint64)
	if !ok || id == 0 {
		return httpx.ErrPrecondition("指派必须指定有效的 assignee_id")
	}
	return nil
}

// guardOwner 校验操作者为本人工单（admin 放行）。
func guardOwner(in statemachine.GuardInput) error {
	tk := in.Entity.(*Ticket)
	if in.Actor.Role == role.Admin {
		return nil
	}
	if tk.RequesterID == in.Actor.UserID {
		return nil
	}
	return httpx.ErrForbidden("仅本人工单可执行该操作")
}

// guardAssigneeOrAdmin 校验操作者为被指派处理人或 admin。
func guardAssigneeOrAdmin(in statemachine.GuardInput) error {
	tk := in.Entity.(*Ticket)
	if in.Actor.Role == role.Admin {
		return nil
	}
	if tk.AssigneeID != nil && *tk.AssigneeID == in.Actor.UserID {
		return nil
	}
	return httpx.ErrForbidden("仅被指派人或管理员可执行该操作")
}

// guardAssigneeOrAgent 资源级校验：被指派人本人、agent 或 admin 放行（PRD「assignee, agent」）。
//
// 非被指派人的 resolver 会被拒绝，避免二线工程师处理他人被指的工单。
func guardAssigneeOrAgent(in statemachine.GuardInput) error {
	tk := in.Entity.(*Ticket)
	if in.Actor.Role == role.Agent || in.Actor.Role == role.Admin {
		return nil
	}
	if tk.AssigneeID != nil && *tk.AssigneeID == in.Actor.UserID {
		return nil
	}
	return httpx.ErrForbidden("仅被指派人、坐席或管理员可执行该操作")
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

// guardPauseReason 校验挂起原因非空。
func guardPauseReason(in statemachine.GuardInput) error {
	if reasonOf(in) == "" {
		return httpx.ErrPrecondition("挂起须填写原因")
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

// guardReopenWindow 校验重开窗口（resolved_at 距今 ≤ 7 天，否则 409）。
func guardReopenWindow(in statemachine.GuardInput) error {
	tk := in.Entity.(*Ticket)
	if tk.ResolvedAt == nil || in.Now.Sub(*tk.ResolvedAt) > ReopenWindow {
		return httpx.ErrConflict("超过 7 天不可重开，请新建工单")
	}
	return nil
}

// solutionOf 从 guard 参数中取解决方案。
func solutionOf(in statemachine.GuardInput) string {
	s, _ := in.Params["solution"].(string)
	return s
}

// reasonOf 从 guard 参数中取原因。
func reasonOf(in statemachine.GuardInput) string {
	s, _ := in.Params["reason"].(string)
	return s
}

// timestampPtr 便于测试构造时间指针。
func timestampPtr(t time.Time) *time.Time { return &t }
