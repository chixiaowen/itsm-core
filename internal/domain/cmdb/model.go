// Package cmdb 实现配置管理数据库（CMDB）域：配置项（CI）台账与 CI 关系、拓扑展开。
//
// 覆盖：CI CRUD（类型枚举、attrs JSON 字符串、同域 code 唯一）、关系增删
// （自环 400 / 重复 409）、退役/软删除校验（带未清理关系 409 + 冲突清单）、
// 拓扑 graph.go BFS 纯函数（默认 2 层）。
//
// 本域可依赖共享内核 platform（ARCHITECTURE §1.3 规则 1）；**禁止**依赖 asset。
package cmdb

import (
	"time"

	"gorm.io/gorm"
)

// CI 生命周期状态常量（与 asset 域共用枚举，见 ARCHITECTURE §6.3.6）。
const (
	// StatusPlanned 规划/采购申请。
	StatusPlanned = "planned"
	// StatusInStock 已入库。
	StatusInStock = "in_stock"
	// StatusInUse 在用。
	StatusInUse = "in_use"
	// StatusMaintain 维护中。
	StatusMaintain = "maintenance"
	// StatusRetired 已退役。
	StatusRetired = "retired"
	// StatusDisposed 已报废。
	StatusDisposed = "disposed"
)

// CI 类型枚举常量。
const (
	// CITypeServer 服务器。
	CITypeServer = "server"
	// CITypeNetwork 网络设备。
	CITypeNetwork = "network"
	// CITypeDatabase 数据库。
	CITypeDatabase = "database"
	// CITypeApp 应用。
	CITypeApp = "application"
	// CITypeTerminal 终端。
	CITypeTerminal = "terminal"
	// CITypeOther 其他。
	CITypeOther = "other"
)

// CI 关系类型常量。
const (
	// RelationDependsOn 依赖。
	RelationDependsOn = "depends_on"
	// RelationContains 包含。
	RelationContains = "contains"
	// RelationConnectsTo 连接。
	RelationConnectsTo = "connects_to"
)

// CI 配置项。
type CI struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Code      string         `gorm:"type:varchar(64);not null;uniqueIndex:idx_ci_code" json:"code"`
	Name      string         `gorm:"type:varchar(255);not null" json:"name"`
	CIType    string         `gorm:"type:varchar(32);not null;index:idx_ci_type" json:"ci_type"`
	Status    string         `gorm:"type:varchar(16);not null;index:idx_ci_status" json:"status"`
	Attrs     string         `gorm:"type:text" json:"attrs"` // 自定义属性 JSON 字符串
	OwnerID   *uint64        `gorm:"index:idx_ci_owner" json:"owner_id"`
	CreatedAt time.Time      `gorm:"index:idx_ci_created" json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index:idx_ci_deleted" json:"-"`
}

// TableName 指定表名。
func (CI) TableName() string { return "cis" }

// CIRelation CI 之间的有向关系。
type CIRelation struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	SourceCIID   uint64    `gorm:"not null;uniqueIndex:idx_cirel_uq;index:idx_cirel_source" json:"source_ci_id"`
	TargetCIID   uint64    `gorm:"not null;uniqueIndex:idx_cirel_uq;index:idx_cirel_target" json:"target_ci_id"`
	RelationType string    `gorm:"type:varchar(16);not null" json:"relation_type"`
	CreatedAt    time.Time `json:"created_at"`
}

// TableName 指定表名。
func (CIRelation) TableName() string { return "ci_relations" }

// Migrate 迁移本域实体（由 bootstrap 按拓扑序调用）。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&CI{}, &CIRelation{})
}

// ValidCIType 判断 CI 类型是否为受支持枚举。
func ValidCIType(t string) bool {
	switch t {
	case CITypeServer, CITypeNetwork, CITypeDatabase, CITypeApp, CITypeTerminal, CITypeOther:
		return true
	default:
		return false
	}
}

// ValidCIStatus 判断 CI 状态是否为受支持枚举。
func ValidCIStatus(s string) bool {
	switch s {
	case StatusPlanned, StatusInStock, StatusInUse, StatusMaintain, StatusRetired, StatusDisposed:
		return true
	default:
		return false
	}
}

// CITypes 返回全部 CI 类型（供 GET /ci-types）。
func CITypes() []string {
	return []string{CITypeServer, CITypeNetwork, CITypeDatabase, CITypeApp, CITypeTerminal, CITypeOther}
}

// RelationTypes 返回全部关系类型。
func RelationTypes() []string {
	return []string{RelationDependsOn, RelationContains, RelationConnectsTo}
}

// ValidRelationType 判断关系类型是否为受支持枚举。
func ValidRelationType(t string) bool {
	switch t {
	case RelationDependsOn, RelationContains, RelationConnectsTo:
		return true
	default:
		return false
	}
}
