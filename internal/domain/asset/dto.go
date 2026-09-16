package asset

import (
	"time"

	"github.com/chixiaowen/itsm-core/internal/domain/cmdb"
)

// AssetRequest 是资产新建/编辑请求体。
type AssetRequest struct {
	// AssetNo 资产编号；留空时由系统按 AST-YYYYMMDD-NNNN 生成。
	AssetNo      string     `json:"asset_no"`
	Name         string     `json:"name" binding:"required,max=255"`
	Category     string     `json:"category" binding:"required,max=32"`
	Vendor       string     `json:"vendor" binding:"max=128"`
	Location     string     `json:"location" binding:"max=128"`
	UserID       *uint64    `json:"user_id"`
	PurchaseDate *time.Time `json:"purchase_date"`
	WarrantyEnd  *time.Time `json:"warranty_end"`
}

// TransitionRequest 是资产生命周期流转请求体。
type TransitionRequest struct {
	Action string `json:"action" binding:"required"`
	// Remark 备注/维修结论/处置方式（依 action 而定）。
	Remark string `json:"remark"`
	// Reason 退役原因。
	Reason string `json:"reason"`
	// UserID 部署领用时填写的使用人。
	UserID *uint64 `json:"user_id"`
	// Location 部署领用时填写的位置。
	Location *string `json:"location"`
	// CIID 流转时可选绑定 CI（退役动作忽略该字段，退役前须先解绑）。
	CIID *uint64 `json:"ci_id"`
	// PurchaseDate 入库时补填采购日期。
	PurchaseDate *time.Time `json:"purchase_date"`
}

// BindCIRequest 是绑定 CI 请求体。
type BindCIRequest struct {
	CIID uint64 `json:"ci_id" binding:"required,min=1"`
}

// AssetDetail 是资产详情响应体（含生命周期历史与绑定 CI）。
type AssetDetail struct {
	Asset   Asset          `json:"asset"`
	History []AssetHistory `json:"history"`
	CI      *cmdb.CI       `json:"ci"`
}

// HistoryResponse 是生命周期历史响应体（类型别名）。
type HistoryResponse = AssetHistory
