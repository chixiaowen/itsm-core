package catalog

import (
	"fmt"
	"strings"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
	"github.com/chixiaowen/itsm-core/internal/pkg/statemachine"
)

// itemMachine 是服务项状态机表（ARCHITECTURE §6.3.5）。
//
// published -> edit -> draft 表示「published 状态下编辑自动退回 draft」；
// archived 无表项，为终态，任何流转均返回 409。
var itemMachine = statemachine.Table{
	StatusDraft: {
		ActionSubmitReview: {To: StatusPendingApproval, Roles: []string{role.Admin}, Guard: guardItemComplete},
		ActionArchive:      {To: StatusArchived, Roles: []string{role.Admin}},
	},
	StatusPendingApproval: {
		ActionPublish: {To: StatusPublished, Roles: []string{role.Admin}},
		ActionReject:  {To: StatusDraft, Roles: []string{role.Admin}, Guard: guardRejectReason},
	},
	StatusPublished: {
		ActionEdit:    {To: StatusDraft, Roles: []string{role.Admin}},
		ActionOffline: {To: StatusOffline, Roles: []string{role.Admin}},
	},
	StatusOffline: {
		ActionRepublish: {To: StatusPublished, Roles: []string{role.Admin}},
		ActionArchive:   {To: StatusArchived, Roles: []string{role.Admin}},
	},
}

// ItemActions 返回某状态下允许的 action 列表（供前端渲染按钮组；后端仍强校验）。
func ItemActions(status string) []string { return itemMachine.Actions(status) }

// guardItemComplete 校验提交审核前景条件：名称/分类/SLA 策略/表单定义完整。
func guardItemComplete(in statemachine.GuardInput) error {
	it, ok := in.Entity.(*ServiceItem)
	if !ok {
		return httpx.ErrPrecondition("实体类型错误")
	}
	if strings.TrimSpace(it.Name) == "" {
		return httpx.ErrPrecondition("服务项名称不能为空")
	}
	if it.CategoryID == 0 {
		return httpx.ErrPrecondition("服务项必须归属分类")
	}
	if it.SLAPolicyID == nil || *it.SLAPolicyID == 0 {
		return httpx.ErrPrecondition("服务项必须绑定 SLA 策略")
	}
	if _, err := ParseFormSchemaStrict(it.FormSchema); err != nil {
		return httpx.ErrPrecondition("服务项表单定义不完整")
	}
	return nil
}

// guardRejectReason 校验驳回必须填写原因。
func guardRejectReason(in statemachine.GuardInput) error {
	if paramString(in.Params, "reason") == "" {
		return httpx.ErrPrecondition("驳回必须填写原因")
	}
	return nil
}

// paramString 从 GuardInput.Params 中读取字符串参数（去空格）。
func paramString(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	if v, ok := params[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// itemStatusOrder 固定服务项状态的遍历顺序，保证 resolveItemTarget 结果确定。
var itemStatusOrder = []string{
	StatusDraft, StatusPendingApproval, StatusPublished, StatusOffline, StatusArchived,
}

// illegalTransitionMsg 构造统一的「非法流转」409 消息文本。
//
// 统一格式（跨域一致）：`非法流转 current=<当前状态> -> target=<目标状态>: <拒绝原因>`。
// 本函数仅用于充实 message 文本，不改变 HTTP 状态码（调用方仍使用 httpx.ErrConflict → 409）。
func illegalTransitionMsg(current, target, reason string) string {
	return fmt.Sprintf("非法流转 current=%s -> target=%s: %s", current, target, reason)
}

// resolveItemTarget 依据动作在服务项状态机表中的语义解析其目标状态。
//
// 当 (from, action) 组合非法（statemachine.Resolve ok=false）时，仍可从其他合法起始状态
// 解析出该动作的目标状态，供 409 消息回显「当前状态 -> 目标状态」。服务项状态机中每个动作
// 只指向唯一目标，故按 itemStatusOrder 顺序查找即得确定结果；若为未知动作则原样返回 action 兜底。
func resolveItemTarget(action string) string {
	for _, from := range itemStatusOrder {
		if tr, ok := itemMachine[from][action]; ok && tr.To != "" {
			return tr.To
		}
	}
	return action
}
