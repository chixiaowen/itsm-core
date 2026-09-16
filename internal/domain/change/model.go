// Package change 实现 ITIL 变更域：申请、风险评估、CAB 会签、窗口校验、实施与回滚、回顾。
//
// 本文件仅声明 GORM 实体与状态/动作常量。
package change

import (
	"time"

	"gorm.io/gorm"
)

// 变更状态常量（PRD §5.5）。
const (
	StatusDraft           = "draft"
	StatusAssessment      = "assessment"
	StatusPendingApproval = "pending_approval"
	StatusApproved        = "approved"
	StatusRejected        = "rejected"
	StatusScheduled       = "scheduled"
	StatusImplementing    = "implementing"
	StatusImplemented     = "implemented"
	StatusReview          = "review"
	StatusClosed          = "closed"
	StatusRolledBack      = "rolled_back"
	StatusCancelled       = "cancelled"
)

// 变更动作常量。
const (
	ActionSubmitAssessment = "submit_assessment"
	ActionPreAuthorize     = "pre_authorize"
	ActionCancel           = "cancel"
	ActionSubmitApproval   = "submit_approval"
	ActionApprove          = "approve"
	ActionReject           = "reject"
	ActionRevise           = "revise"
	ActionSchedule         = "schedule"
	ActionStartImplement   = "start_implement"
	ActionComplete         = "complete"
	ActionRollback         = "rollback"
	ActionReview           = "review"
	ActionClose            = "close"
	ActionResubmit         = "resubmit"
)

// 变更类型与风险等级。
const (
	TypeStandard  = "standard"
	TypeNormal    = "normal"
	TypeEmergency = "emergency"

	RiskHigh   = "high"
	RiskMedium = "medium"
	RiskLow    = "low"
)

// 审批决定。
const (
	DecisionApprove = "approved"
	DecisionReject  = "rejected"
)

// Change 变更实体。
type Change struct {
	ID               uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Code             string         `gorm:"type:varchar(32);not null;uniqueIndex:idx_chg_code" json:"code"`
	Title            string         `gorm:"type:varchar(255);not null" json:"title"`
	Description      string         `gorm:"type:text" json:"description"`
	ChangeType       string         `gorm:"type:varchar(16);not null;index:idx_chg_type" json:"change_type"`
	Status           string         `gorm:"type:varchar(32);not null;index:idx_chg_status" json:"status"`
	RiskLevel        string         `gorm:"type:varchar(16);not null;index:idx_chg_risk" json:"risk_level"`
	ImpactAnalysis   string         `gorm:"type:text" json:"impact_analysis"`
	Plan             string         `gorm:"type:text" json:"plan"`
	RollbackPlan     string         `gorm:"type:text" json:"rollback_plan"`
	ImplementResult  string         `gorm:"type:text" json:"implement_result"`
	RollbackReason   string         `gorm:"type:text" json:"rollback_reason"`
	ReviewConclusion string         `gorm:"type:text" json:"review_conclusion"`
	RequesterID      uint64         `gorm:"not null;index:idx_chg_requester" json:"requester_id"`
	ManagerID        *uint64        `gorm:"index:idx_chg_manager" json:"manager_id"`
	PreAuthorized    bool           `gorm:"not null" json:"pre_authorized"`
	WindowStart      *time.Time     `json:"window_start"`
	WindowEnd        *time.Time     `json:"window_end"`
	ClosedAt         *time.Time     `json:"closed_at"`
	CreatedAt        time.Time      `gorm:"index:idx_chg_created" json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index:idx_chg_deleted" json:"-"`
}

// TableName 指定表名。
func (Change) TableName() string { return "changes" }

// ChangeApproval 变更审批记录（CAB 会签）。
//
// 四眼原则（四眼/双人复核）依赖「同一人对同一变更只能投一票」，通过两层保证：
//   - 数据库层：`(change_id, approver_id)` 复合唯一索引 idx_apv_change_approver（双库通用，23 字符）；
//   - 应用层：RecordApproval 先查后写（FindByChangeAndApprover），重复投票返回 409。
//
// 前提：审批记录**不做软删除**（无 DeletedAt 字段），故复合唯一索引不会误伤「驳回后重投」。
// 变更驳回后经 revise 回到 draft 再重投，由 Service 在重提时清理旧审批记录（DeleteByChange）。
type ChangeApproval struct {
	ID         uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	ChangeID   uint64         `gorm:"not null;index:idx_apv_change;uniqueIndex:idx_apv_change_approver" json:"change_id"`
	ApproverID uint64         `gorm:"not null;index:idx_apv_approver;uniqueIndex:idx_apv_change_approver" json:"approver_id"`
	Decision   string         `gorm:"type:varchar(16)" json:"decision"` // 空=待审
	Comment    string         `gorm:"type:text" json:"comment"`
	DecidedAt  *time.Time     `json:"decided_at"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index:idx_apv_deleted" json:"-"`
}

// TableName 指定表名。
func (ChangeApproval) TableName() string { return "change_approvals" }

// Migrate 迁移本域实体。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Change{}, &ChangeApproval{})
}
