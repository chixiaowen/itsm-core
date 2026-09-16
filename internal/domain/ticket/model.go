// Package ticket 实现 ITIL 工单域：工单全生命周期、分类树、SLA 计时、挂起恢复、评价与重开。
//
// 分层铁律（ARCHITECTURE §1.2）：本文件仅声明 GORM 实体与状态/动作常量，
// 只允许 import time 与 gorm（DeletedAt）；不得 import 其它业务域。
package ticket

import (
	"time"

	"gorm.io/gorm"
)

// 工单状态常量（与 PRD §5.2 / ARCHITECTURE §4.3 一致）。
const (
	StatusDraft      = "draft"
	StatusNew        = "new"
	StatusAssigned   = "assigned"
	StatusInProgress = "in_progress"
	StatusPending    = "pending"
	StatusResolved   = "resolved"
	StatusClosed     = "closed"
	StatusReopened   = "reopened"
	StatusCancelled  = "cancelled"
)

// 工单动作常量（状态机 action）。
const (
	ActionSubmit  = "submit"
	ActionCancel  = "cancel"
	ActionAssign  = "assign"
	ActionStart   = "start"
	ActionReturn  = "return"
	ActionPending = "pending"
	ActionResume  = "resume"
	ActionResolve = "resolve"
	ActionClose   = "close"
	ActionReopen  = "reopen"
)

// 工单来源类型常量。
const (
	TypeManual   = "manual"
	TypeService  = "service"
	TypeIncident = "incident"
)

// 优先级常量（P1~P4）。
const (
	PriorityP1 = "P1"
	PriorityP2 = "P2"
	PriorityP3 = "P3"
	PriorityP4 = "P4"
)

// 重开窗口：resolved 后允许重开的时长（PRD §5.2：7 天）。
const ReopenWindow = 7 * 24 * time.Hour

// Ticket 工单实体。
type Ticket struct {
	ID               uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Code             string         `gorm:"type:varchar(32);not null;uniqueIndex:idx_tkt_code" json:"code"`
	Title            string         `gorm:"type:varchar(255);not null" json:"title"`
	Description      string         `gorm:"type:text" json:"description"`
	Status           string         `gorm:"type:varchar(32);not null;index:idx_tkt_status" json:"status"`
	Priority         string         `gorm:"type:varchar(8);not null;index:idx_tkt_priority" json:"priority"` // P1..P4
	Type             string         `gorm:"type:varchar(16);not null;index:idx_tkt_type" json:"type"`        // manual/service/incident
	CategoryID       *uint64        `gorm:"index:idx_tkt_category" json:"category_id"`
	RequesterID      uint64         `gorm:"not null;index:idx_tkt_requester" json:"requester_id"`
	AssigneeID       *uint64        `gorm:"index:idx_tkt_assignee" json:"assignee_id"`
	ServiceItemID    *uint64        `gorm:"index:idx_tkt_service_item" json:"service_item_id"`
	SourceIncidentID *uint64        `gorm:"index:idx_tkt_src_inc" json:"source_incident_id"`
	SLAPolicyID      *uint64        `json:"sla_policy_id"`
	FormData         string         `gorm:"type:text" json:"form_data"` // 服务目录动态表单快照(JSON 字符串)
	Solution         string         `gorm:"type:text" json:"solution"`
	PausedMinutes    int            `gorm:"not null" json:"paused_minutes"`
	PausedAt         *time.Time     `json:"paused_at"`
	FirstRespondedAt *time.Time     `json:"first_responded_at"`
	ResponseDueAt    *time.Time     `json:"response_due_at"`
	ResolveDueAt     *time.Time     `json:"resolve_due_at"`
	ResolvedAt       *time.Time     `json:"resolved_at"`
	ClosedAt         *time.Time     `json:"closed_at"`
	ReopenedAt       *time.Time     `json:"reopened_at"`
	Rating           *int           `json:"rating"` // 1..5，仅可写一次
	RatingComment    string         `gorm:"type:varchar(500)" json:"rating_comment"`
	RatedAt          *time.Time     `json:"rated_at"`
	CreatedAt        time.Time      `gorm:"index:idx_tkt_created" json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index:idx_tkt_deleted" json:"-"`
	// SLAStatus 是计算字段（非持久化），由 service 调 pkg/sla 计算后注入。
	SLAStatus string `gorm:"-" json:"sla_status"`
}

// TableName 指定表名。
func (Ticket) TableName() string { return "tickets" }

// TicketCategory 工单分类（多级树）。
type TicketCategory struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string         `gorm:"type:varchar(128);not null" json:"name"`
	ParentID  *uint64        `gorm:"index:idx_tktcat_parent" json:"parent_id"`
	SortOrder int            `gorm:"not null" json:"sort_order"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index:idx_tktcat_deleted" json:"-"`
}

// TableName 指定表名。
func (TicketCategory) TableName() string { return "ticket_categories" }

// TicketCI 工单↔CI 多对多连接表（硬删除，无软删除）。
type TicketCI struct {
	ID       uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	TicketID uint64 `gorm:"not null;uniqueIndex:idx_tktci_uq" json:"ticket_id"`
	CIID     uint64 `gorm:"not null;uniqueIndex:idx_tktci_uq" json:"ci_id"`
}

// TableName 指定表名。
func (TicketCI) TableName() string { return "ticket_cis" }

// Migrate 迁移本域实体（由 bootstrap 按拓扑序调用；仅迁移本域表）。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&TicketCategory{}, &Ticket{}, &TicketCI{})
}
