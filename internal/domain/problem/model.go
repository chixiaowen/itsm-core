// Package problem 实现 ITIL 问题域：由事件聚合、RCA 根因分析、已知错误与规避、
// 关联变更、解决约束与「建议聚合」提示。
//
// 本文件仅声明 GORM 实体与状态/动作常量。
package problem

import (
	"time"

	"gorm.io/gorm"
)

// 问题状态常量（PRD §5.4）。
const (
	StatusNew           = "new"
	StatusTriage        = "triage"
	StatusInvestigating = "investigating"
	StatusKnownError    = "known_error"
	StatusResolved      = "resolved"
	StatusClosed        = "closed"
	StatusCancelled     = "cancelled"
)

// 问题动作常量。
const (
	ActionTriage           = "triage"
	ActionCancel           = "cancel"
	ActionInvestigate      = "investigate"
	ActionMarkKnownError   = "mark_known_error"
	ActionUpdateWorkaround = "update_workaround"
	ActionResolve          = "resolve"
	ActionClose            = "close"
	ActionRecur            = "recur"
)

// 来源常量。
const (
	SourceAggregate = "aggregate"
	SourceManual    = "manual"
)

// Problem 问题实体。
type Problem struct {
	ID             uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Code           string         `gorm:"type:varchar(32);not null;uniqueIndex:idx_prb_code" json:"code"`
	Title          string         `gorm:"type:varchar(255);not null" json:"title"`
	Description    string         `gorm:"type:text" json:"description"`
	Status         string         `gorm:"type:varchar(32);not null;index:idx_prb_status" json:"status"`
	Source         string         `gorm:"type:varchar(16);not null" json:"source"` // aggregate/manual
	RootCause      string         `gorm:"type:text" json:"root_cause"`
	Analysis       string         `gorm:"type:text" json:"analysis"`
	Symptom        string         `gorm:"type:text" json:"symptom"`
	Workaround     string         `gorm:"type:text" json:"workaround"`
	NoChangeReason string         `gorm:"type:text" json:"no_change_reason"`
	AssigneeID     *uint64        `gorm:"index:idx_prb_assignee" json:"assignee_id"`
	CreatorID      uint64         `gorm:"not null" json:"creator_id"`
	ResolvedAt     *time.Time     `json:"resolved_at"`
	ClosedAt       *time.Time     `json:"closed_at"`
	CreatedAt      time.Time      `gorm:"index:idx_prb_created" json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index:idx_prb_deleted" json:"-"`
}

// TableName 指定表名。
func (Problem) TableName() string { return "problems" }

// ProblemChange 问题↔变更 多对多连接表（硬删除）。
type ProblemChange struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
	ProblemID uint64 `gorm:"not null;uniqueIndex:idx_prbchg_uq" json:"problem_id"`
	ChangeID  uint64 `gorm:"not null;uniqueIndex:idx_prbchg_uq" json:"change_id"`
}

// TableName 指定表名。
func (ProblemChange) TableName() string { return "problem_changes" }

// Migrate 迁移本域实体。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Problem{}, &ProblemChange{})
}
