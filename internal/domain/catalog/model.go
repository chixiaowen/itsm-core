// Package catalog 实现服务目录域：服务分类树与服务项。
//
// 覆盖：分类树 CRUD 与排序、服务项发布/下线状态机（ARCHITECTURE §6.3.5）、
// 动态表单（form_schema）解析与校验、用户侧仅返回 published、下单转工单。
//
// 本域可依赖共享内核 platform（ARCHITECTURE §1.3 规则 1），但**不**依赖任何兄弟业务域；
// 生成工单通过本包内定义的消费侧接口 TicketCreator 由装配层注入。
package catalog

import (
	"time"

	"gorm.io/gorm"
)

// 服务项状态常量（对齐 ARCHITECTURE §4.7 / PRD §5.6）。
const (
	// StatusDraft 草稿（新建/编辑退回）。
	StatusDraft = "draft"
	// StatusPendingApproval 待审核（提交发布审核后）。
	StatusPendingApproval = "pending_approval"
	// StatusPublished 已发布（终端用户可见且可下单）。
	StatusPublished = "published"
	// StatusOffline 已下线（用户不可见）。
	StatusOffline = "offline"
	// StatusArchived 已归档（终态）。
	StatusArchived = "archived"
)

// 服务项动作常量（对齐 ARCHITECTURE §6.3.5）。
const (
	// ActionSubmitReview 提交发布审核：draft -> pending_approval。
	ActionSubmitReview = "submit_review"
	// ActionArchive 归档：draft/offline -> archived。
	ActionArchive = "archive"
	// ActionPublish 发布：pending_approval -> published。
	ActionPublish = "publish"
	// ActionReject 审核驳回：pending_approval -> draft。
	ActionReject = "reject"
	// ActionEdit 编辑退回：published -> draft。
	ActionEdit = "edit"
	// ActionOffline 下线：published -> offline。
	ActionOffline = "offline"
	// ActionRepublish 重新上架：offline -> published。
	ActionRepublish = "republish"
)

// ServiceCategory 服务分类（多级树）。
type ServiceCategory struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string         `gorm:"type:varchar(128);not null" json:"name"`
	ParentID  *uint64        `gorm:"index:idx_scat_parent" json:"parent_id"`
	SortOrder int            `gorm:"not null" json:"sort_order"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index:idx_scat_deleted" json:"-"`
}

// TableName 指定表名。
func (ServiceCategory) TableName() string { return "service_categories" }

// ServiceItem 服务项。
type ServiceItem struct {
	ID               uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Name             string         `gorm:"type:varchar(128);not null" json:"name"`
	Description      string         `gorm:"type:text" json:"description"`
	Status           string         `gorm:"type:varchar(32);not null;index:idx_sitm_status" json:"status"`
	CategoryID       uint64         `gorm:"not null;index:idx_sitm_category" json:"category_id"`
	SLAPolicyID      *uint64        `json:"sla_policy_id"`
	DefaultPriority  string         `gorm:"type:varchar(8)" json:"default_priority"`
	RequiresApproval bool           `gorm:"not null" json:"requires_approval"`
	FormSchema       string         `gorm:"type:text" json:"form_schema"` // JSON 字符串
	CreatedAt        time.Time      `gorm:"index:idx_sitm_created" json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index:idx_sitm_deleted" json:"-"`
}

// TableName 指定表名。
func (ServiceItem) TableName() string { return "service_items" }

// Migrate 迁移本域实体（由 bootstrap 按拓扑序调用）。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&ServiceCategory{}, &ServiceItem{})
}
