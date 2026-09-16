// Package incident 实现 ITIL 事件域：上报、优先级矩阵、升级历史、转工单、解决与复盘。
//
// 本文件仅声明 GORM 实体与状态/动作常量，只允许 import time 与 gorm。
package incident

import (
	"time"

	"gorm.io/gorm"
)

// 事件状态常量（PRD §5.3）。
const (
	StatusReported   = "reported"
	StatusTriage     = "triage"
	StatusInProgress = "in_progress"
	StatusEscalated  = "escalated"
	StatusPending    = "pending"
	StatusResolved   = "resolved"
	StatusClosed     = "closed"
	StatusCancelled  = "cancelled"
)

// 事件动作常量。
const (
	ActionTriage   = "triage"
	ActionCancel   = "cancel"
	ActionConfirm  = "confirm"
	ActionFalsePos = "false_positive"
	ActionEscalate = "escalate"
	ActionTakeOver = "take_over"
	ActionPending  = "pending"
	ActionResume   = "resume"
	ActionResolve  = "resolve"
	ActionClose    = "close"
	ActionRevert   = "revert"
)

// 影响度 / 紧急度取值。
const (
	ImpactHigh   = "high"
	ImpactMedium = "medium"
	ImpactLow    = "low"
	UrgencyHigh  = "high"
	UrgencyMed   = "medium"
	UrgencyLow   = "low"
)

// 优先级取值。
const (
	PriorityP1 = "P1"
	PriorityP2 = "P2"
	PriorityP3 = "P3"
	PriorityP4 = "P4"
)

// 升级类型。
const (
	EscFunc = "functional"
	EscHier = "hierarchical"
)

// 最大升级级别。
const MaxEscalationLevel = 3

// RevertWindow 是 resolved 后允许回退的时长（PRD §5.3：24 小时）。
const RevertWindow = 24 * time.Hour

// Incident 事件实体。
type Incident struct {
	ID                 uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Code               string         `gorm:"type:varchar(32);not null;uniqueIndex:idx_inc_code" json:"code"`
	Title              string         `gorm:"type:varchar(255);not null" json:"title"`
	Description        string         `gorm:"type:text" json:"description"`
	Status             string         `gorm:"type:varchar(32);not null;index:idx_inc_status" json:"status"`
	Impact             string         `gorm:"type:varchar(16);not null" json:"impact"`
	Urgency            string         `gorm:"type:varchar(16);not null" json:"urgency"`
	Priority           string         `gorm:"type:varchar(8);not null;index:idx_inc_priority" json:"priority"`
	PriorityOverridden bool           `gorm:"not null" json:"priority_overridden"`
	EscalationLevel    int            `gorm:"not null" json:"escalation_level"` // 0..3
	ReporterID         uint64         `gorm:"not null;index:idx_inc_reporter" json:"reporter_id"`
	AssigneeID         *uint64        `gorm:"index:idx_inc_assignee" json:"assignee_id"`
	ProblemID          *uint64        `gorm:"index:idx_inc_problem" json:"problem_id"`
	TicketID           *uint64        `gorm:"index:idx_inc_ticket" json:"ticket_id"`
	SLAPolicyID        *uint64        `json:"sla_policy_id"`
	Solution           string         `gorm:"type:text" json:"solution"`
	ReviewConclusion   string         `gorm:"type:text" json:"review_conclusion"`
	OccurredAt         *time.Time     `json:"occurred_at"`
	ResolvedAt         *time.Time     `json:"resolved_at"`
	ClosedAt           *time.Time     `json:"closed_at"`
	ResponseDueAt      *time.Time     `json:"response_due_at"`
	ResolveDueAt       *time.Time     `json:"resolve_due_at"`
	CreatedAt          time.Time      `gorm:"index:idx_inc_created" json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index:idx_inc_deleted" json:"-"`
	// SLAStatus 是计算字段（非持久化）。
	SLAStatus string `gorm:"-" json:"sla_status"`
}

// TableName 指定表名。
func (Incident) TableName() string { return "incidents" }

// IncidentEscalation 事件升级历史（只追加）。
type IncidentEscalation struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	IncidentID   uint64    `gorm:"not null;index:idx_esc_incident" json:"incident_id"`
	Level        int       `gorm:"not null" json:"level"`
	Type         string    `gorm:"type:varchar(16);not null" json:"type"` // functional/hierarchical
	Reason       string    `gorm:"type:text;not null" json:"reason"`
	FromAssignee *uint64   `json:"from_assignee_id"`
	ToAssignee   *uint64   `json:"to_assignee_id"`
	ActorID      uint64    `gorm:"not null" json:"actor_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// TableName 指定表名。
func (IncidentEscalation) TableName() string { return "incident_escalations" }

// IncidentCI 事件↔CI 多对多连接表（硬删除）。
type IncidentCI struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	IncidentID uint64 `gorm:"not null;uniqueIndex:idx_incci_uq" json:"incident_id"`
	CIID       uint64 `gorm:"not null;uniqueIndex:idx_incci_uq" json:"ci_id"`
}

// TableName 指定表名。
func (IncidentCI) TableName() string { return "incident_cis" }

// Migrate 迁移本域实体。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Incident{}, &IncidentEscalation{}, &IncidentCI{})
}
