// 本文件声明 problem 域状态机表与 guard 函数（ARCHITECTURE §6.3.3）。
package problem

import (
	"fmt"
	"sort"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
	"github.com/chixiaowen/itsm-core/internal/pkg/statemachine"
)

// problemMachine 是问题状态机（from -> action -> Transition）。
var problemMachine = statemachine.Table{
	StatusNew: {
		ActionTriage: {To: StatusTriage, Roles: role.ProblemManagers},
		ActionCancel: {To: StatusCancelled, Roles: role.ProblemManagers, Guard: guardReason},
	},
	StatusTriage: {
		ActionInvestigate: {To: StatusInvestigating, Roles: role.ProblemManagers, Guard: guardAssigneeSet},
		ActionCancel:      {To: StatusCancelled, Roles: role.ProblemManagers, Guard: guardReason},
	},
	StatusInvestigating: {
		ActionMarkKnownError: {To: StatusKnownError, Roles: role.ProblemManagers, Guard: guardKnownError},
		ActionResolve:        {To: StatusResolved, Roles: role.ProblemManagers, Guard: guardResolve},
	},
	StatusKnownError: {
		ActionUpdateWorkaround: {To: StatusKnownError, Roles: role.ProblemManagers, Guard: guardKnownError},
		ActionResolve:          {To: StatusResolved, Roles: role.ProblemManagers, Guard: guardResolve},
	},
	StatusResolved: {
		ActionClose: {To: StatusClosed, Roles: role.ProblemManagers},
		ActionRecur: {To: StatusInvestigating, Roles: role.ProblemManagers, Guard: guardReason},
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

// resolveTarget 依据动作在问题状态机表中的语义解析其目标状态。
//
// 当 (from, action) 组合非法时，仍可从其它合法起始状态解析该动作的规范目标状态，
// 供 409 消息回显「当前状态 -> 目标状态」；未知动作原样兜底。
// 按状态名排序遍历，保证同一动作得到确定结果（每个动作在表中只指向唯一目标）。
func resolveTarget(action string) string {
	statuses := make([]string, 0, len(problemMachine))
	for s := range problemMachine {
		statuses = append(statuses, s)
	}
	sort.Strings(statuses)
	for _, s := range statuses {
		if tr, ok := problemMachine[s][action]; ok {
			return tr.To
		}
	}
	return action
}

// guardReason 校验原因非空。
func guardReason(in statemachine.GuardInput) error {
	if v, _ := in.Params["reason"].(string); v == "" {
		return httpx.ErrPrecondition("原因必填")
	}
	return nil
}

// guardAssigneeSet 校验指派人非空。
func guardAssigneeSet(in statemachine.GuardInput) error {
	if v, _ := in.Params["assignee_id"].(uint64); v == 0 {
		return httpx.ErrPrecondition("开始调查须指定 assignee_id")
	}
	return nil
}

// guardKnownError 校验 root_cause 与 workaround 均非空。
func guardKnownError(in statemachine.GuardInput) error {
	p := in.Entity.(*Problem)
	if p.RootCause == "" || p.Workaround == "" {
		return httpx.ErrPrecondition("标记已知错误须填写 root_cause 与 workaround")
	}
	return nil
}

// guardResolve 校验解决约束：关联 ≥1 个 closed 变更 或 no_change_reason 非空。
func guardResolve(in statemachine.GuardInput) error {
	if n, _ := in.Params["closed_changes"].(int); n >= 1 {
		return nil
	}
	p := in.Entity.(*Problem)
	if p.NoChangeReason != "" {
		return nil
	}
	if v, _ := in.Params["no_change_reason"].(string); v != "" {
		return nil
	}
	return httpx.ErrPrecondition("resolved 须关联 ≥1 个已关闭变更，或填写无需变更原因")
}
