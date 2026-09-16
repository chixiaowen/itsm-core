package asset

import (
	"fmt"
	"strings"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
	"github.com/chixiaowen/itsm-core/internal/pkg/statemachine"
)

// 资产动作常量（对齐 ARCHITECTURE §6.3.6）。
const (
	// ActionStockIn 入库：planned -> in_stock。
	ActionStockIn = "stock_in"
	// ActionDeploy 部署领用：in_stock -> in_use。
	ActionDeploy = "deploy"
	// ActionRetire 退役：in_stock/in_use -> retired。
	ActionRetire = "retire"
	// ActionMaintain 送修：in_use -> maintenance。
	ActionMaintain = "maintain"
	// ActionFinishMaintain 维护完成：maintenance -> in_use。
	ActionFinishMaintain = "finish_maintain"
	// ActionDispose 报废处置：retired -> disposed。
	ActionDispose = "dispose"
)

// assetMachine 是资产生命周期状态机表（ARCHITECTURE §6.3.6）。
//
// disposed 无表项，为终态，任何流转均返回 409。
var assetMachine = statemachine.Table{
	StatusPlanned: {
		ActionStockIn: {To: StatusInStock, Roles: role.CmdbManagers, Guard: guardStockIn},
	},
	StatusInStock: {
		ActionDeploy: {To: StatusInUse, Roles: role.CmdbManagers, Guard: guardDeploy},
		ActionRetire: {To: StatusRetired, Roles: role.CmdbManagers, Guard: guardRetireReason},
	},
	StatusInUse: {
		ActionMaintain: {To: StatusMaintenance, Roles: role.CmdbManagers, Guard: guardMaintain},
		ActionRetire:   {To: StatusRetired, Roles: role.CmdbManagers, Guard: guardRetireInUse},
	},
	StatusMaintenance: {
		ActionFinishMaintain: {To: StatusInUse, Roles: role.CmdbManagers, Guard: guardFinishMaintain},
	},
	StatusRetired: {
		ActionDispose: {To: StatusDisposed, Roles: role.CmdbManagers, Guard: guardDispose},
	},
}

// AssetActions 返回某状态下允许的动作（前端按钮组渲染）。
func AssetActions(status string) []string { return assetMachine.Actions(status) }

// guardStockIn 入库前须填写采购信息（采购日期）。
func guardStockIn(in statemachine.GuardInput) error {
	a, ok := in.Entity.(*Asset)
	if !ok {
		return httpx.ErrPrecondition("实体类型错误")
	}
	if strings.TrimSpace(a.AssetNo) == "" || strings.TrimSpace(a.Category) == "" {
		return httpx.ErrPrecondition("入库前须填写资产编号与类别")
	}
	if a.PurchaseDate == nil {
		return httpx.ErrPrecondition("入库前须填写采购信息（采购日期）")
	}
	return nil
}

// guardDeploy 部署领用前须填写使用人与位置。
func guardDeploy(in statemachine.GuardInput) error {
	a, ok := in.Entity.(*Asset)
	if !ok {
		return httpx.ErrPrecondition("实体类型错误")
	}
	if a.UserID == nil {
		return httpx.ErrPrecondition("部署领用须填写使用人")
	}
	if strings.TrimSpace(a.Location) == "" {
		return httpx.ErrPrecondition("部署领用须填写位置")
	}
	return nil
}

// guardRetireReason 未使用直接退役须填写原因。
func guardRetireReason(in statemachine.GuardInput) error {
	if paramString(in.Params, "reason") == "" {
		return httpx.ErrPrecondition("退役须填写原因")
	}
	return nil
}

// guardRetireInUse 在用资产退役前须先解绑 CI（否则 409）。
func guardRetireInUse(in statemachine.GuardInput) error {
	a, ok := in.Entity.(*Asset)
	if !ok {
		return httpx.ErrPrecondition("实体类型错误")
	}
	if a.CIID != nil {
		return httpx.ErrConflict("退役前请先解绑关联的 CI")
	}
	return nil
}

// guardMaintain 送修前须填写维修原因/单号。
func guardMaintain(in statemachine.GuardInput) error {
	if paramString(in.Params, "remark") == "" {
		return httpx.ErrPrecondition("送修须填写维修原因或维修单号")
	}
	return nil
}

// guardFinishMaintain 维护完成须记录维修结论。
func guardFinishMaintain(in statemachine.GuardInput) error {
	if paramString(in.Params, "remark") == "" {
		return httpx.ErrPrecondition("维护完成须填写维修结论")
	}
	return nil
}

// guardDispose 报废须填写处置方式。
func guardDispose(in statemachine.GuardInput) error {
	if paramString(in.Params, "remark") == "" {
		return httpx.ErrPrecondition("报废须填写处置方式")
	}
	return nil
}

// paramString 从 GuardInput.Params 读取字符串参数（去空格）。
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

// assetStatusOrder 固定资产生命周期状态的遍历顺序，保证 resolveAssetTarget 结果确定。
var assetStatusOrder = []string{
	StatusPlanned, StatusInStock, StatusInUse, StatusMaintenance, StatusRetired, StatusDisposed,
}

// illegalTransitionMsg 构造统一的「非法流转」409 消息文本。
//
// 统一格式（跨域一致）：`非法流转 current=<当前状态> -> target=<目标状态>: <拒绝原因>`。
// 本函数仅用于充实 message 文本，不改变 HTTP 状态码（调用方仍使用 httpx.ErrConflict → 409）。
func illegalTransitionMsg(current, target, reason string) string {
	return fmt.Sprintf("非法流转 current=%s -> target=%s: %s", current, target, reason)
}

// resolveAssetTarget 依据动作在资产生命周期状态机表中的语义解析其目标状态。
//
// 当 (from, action) 组合非法（statemachine.Resolve ok=false）时，仍可从其他合法起始状态
// 解析出该动作的目标状态，供 409 消息回显「当前状态 -> 目标状态」。资产生命周期状态机中每个
// 动作只指向唯一目标，故按 assetStatusOrder 顺序查找即得确定结果；未知动作则原样返回 action 兜底。
func resolveAssetTarget(action string) string {
	for _, from := range assetStatusOrder {
		if tr, ok := assetMachine[from][action]; ok && tr.To != "" {
			return tr.To
		}
	}
	return action
}
