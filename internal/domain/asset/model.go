// Package asset 实现 IT 资产域：资产台账与生命周期。
//
// 覆盖：资产生命周期状态机（ARCHITECTURE §6.3.6）、每次流转写 AssetHistory、
// 资产与 CI 的**全局 1:1 唯一**绑定（ci_id 唯一、可空；一个 CI 至多被一个资产绑定，
// 已绑定的 CI 再被第二个资产绑定时返回 409，见 PRD §5 Q4）。
//
// 本域依赖共享内核 platform 与 cmdb（ARCHITECTURE §1.3 规则 1/5）；
// cmdb 不反向依赖 asset。
package asset

import (
	"time"

	"gorm.io/gorm"

	"github.com/chixiaowen/itsm-core/internal/domain/cmdb"
)

// 资产生命周期状态常量：与 CMDB 的 CI 状态枚举共用同一组取值（ARCHITECTURE §6.3.6）。
const (
	// StatusPlanned 规划/采购申请。
	StatusPlanned = cmdb.StatusPlanned
	// StatusInStock 已入库。
	StatusInStock = cmdb.StatusInStock
	// StatusInUse 在用。
	StatusInUse = cmdb.StatusInUse
	// StatusMaintenance 维护中。
	StatusMaintenance = cmdb.StatusMaintain
	// StatusRetired 已退役。
	StatusRetired = cmdb.StatusRetired
	// StatusDisposed 已报废（终态）。
	StatusDisposed = cmdb.StatusDisposed
)

// AssetPrefix 是自动生成资产编号的前缀（形如 AST-20260916-0001）。
const AssetPrefix = "AST"

// Asset 资产台账。
type Asset struct {
	ID           uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	AssetNo      string         `gorm:"type:varchar(64);not null;uniqueIndex:idx_ast_no" json:"asset_no"`
	Name         string         `gorm:"type:varchar(255);not null" json:"name"`
	Category     string         `gorm:"type:varchar(32);not null;index:idx_ast_category" json:"category"`
	Status       string         `gorm:"type:varchar(16);not null;index:idx_ast_status" json:"status"`
	CIID         *uint64        `gorm:"uniqueIndex:idx_ast_ci" json:"ci_id"` // 可空 1:1
	UserID       *uint64        `gorm:"index:idx_ast_user" json:"user_id"`
	Location     string         `gorm:"type:varchar(128)" json:"location"`
	Vendor       string         `gorm:"type:varchar(128)" json:"vendor"`
	PurchaseDate *time.Time     `json:"purchase_date"`
	WarrantyEnd  *time.Time     `json:"warranty_end"`
	CreatedAt    time.Time      `gorm:"index:idx_ast_created" json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index:idx_ast_deleted" json:"-"`
}

// TableName 指定表名。
func (Asset) TableName() string { return "assets" }

// AssetHistory 资产生命周期历史（每次流转追加一条，不软删除）。
type AssetHistory struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	AssetID    uint64    `gorm:"not null;index:idx_asth_asset" json:"asset_id"`
	FromStatus string    `gorm:"type:varchar(16)" json:"from_status"`
	ToStatus   string    `gorm:"type:varchar(16);not null" json:"to_status"`
	Remark     string    `gorm:"type:text" json:"remark"`
	ActorID    uint64    `gorm:"not null" json:"actor_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// TableName 指定表名。
func (AssetHistory) TableName() string { return "asset_histories" }

// Migrate 迁移本域实体（由 bootstrap 按拓扑序调用）。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Asset{}, &AssetHistory{})
}

// ValidStatus 判断资产状态是否为受支持枚举。
func ValidStatus(s string) bool { return cmdb.ValidCIStatus(s) }
