// Package platform 是共享内核域：用户/角色、SLA 策略、审计、评论、附件。
//
// 所有业务域均可依赖本域（见 ARCHITECTURE §1.3）。
package platform

import (
	"time"

	"gorm.io/gorm"
)

// 用户状态常量。
const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
)

// 评论/附件业务类型常量（多态 biz_type）。
const (
	BizTypeTicket   = "ticket"
	BizTypeIncident = "incident"
	BizTypeProblem  = "problem"
	BizTypeChange   = "change"
)

// User 系统用户（单租户）。
type User struct {
	ID           uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string         `gorm:"type:varchar(64);not null;uniqueIndex:idx_usr_username" json:"username"`
	DisplayName  string         `gorm:"type:varchar(128);not null" json:"display_name"`
	Role         string         `gorm:"type:varchar(32);not null;index:idx_usr_role" json:"role"`
	PasswordHash string         `gorm:"type:varchar(255);not null" json:"-"`
	Email        string         `gorm:"type:varchar(128)" json:"email"`
	Status       string         `gorm:"type:varchar(16);not null;index:idx_usr_status" json:"status"` // active/disabled
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index:idx_usr_deleted" json:"-"`
}

// TableName 指定表名。
func (User) TableName() string { return "users" }

// SLAPolicy 按优先级定义的 SLA 策略（(priority) 唯一）。
type SLAPolicy struct {
	ID              uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Name            string         `gorm:"type:varchar(64);not null" json:"name"`
	Priority        string         `gorm:"type:varchar(8);not null;uniqueIndex:idx_sla_priority" json:"priority"` // P1..P4
	ResponseMinutes int            `gorm:"not null" json:"response_minutes"`
	ResolveMinutes  int            `gorm:"not null" json:"resolve_minutes"`
	PauseOnPending  bool           `gorm:"not null" json:"pause_on_pending"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index:idx_sla_deleted" json:"-"`
}

// TableName 指定表名。
func (SLAPolicy) TableName() string { return "sla_policies" }

// AuditLog 审计日志（只追加，不软删除）。
type AuditLog struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ActorID     uint64    `gorm:"not null;index:idx_audit_actor" json:"actor_id"`
	Action      string    `gorm:"type:varchar(64);not null;index:idx_audit_action" json:"action"`
	BizType     string    `gorm:"type:varchar(32);not null;index:idx_audit_biz" json:"biz_type"`
	BizID       uint64    `gorm:"not null;index:idx_audit_biz" json:"biz_id"`
	FromStatus  string    `gorm:"type:varchar(32)" json:"from_status"`
	ToStatus    string    `gorm:"type:varchar(32)" json:"to_status"`
	BeforeValue string    `gorm:"type:text" json:"before_value"`
	AfterValue  string    `gorm:"type:text" json:"after_value"`
	ClientIP    string    `gorm:"type:varchar(64)" json:"client_ip"`
	CreatedAt   time.Time `gorm:"index:idx_audit_created" json:"created_at"`
}

// TableName 指定表名。
func (AuditLog) TableName() string { return "audit_logs" }

// Comment 评论（多态：biz_type + biz_id）。
type Comment struct {
	ID         uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	BizType    string         `gorm:"type:varchar(32);not null;index:idx_cmt_biz" json:"biz_type"` // ticket/incident/problem/change
	BizID      uint64         `gorm:"not null;index:idx_cmt_biz" json:"biz_id"`
	AuthorID   uint64         `gorm:"not null;index:idx_cmt_author" json:"author_id"`
	Content    string         `gorm:"type:text;not null" json:"content"`
	IsInternal bool           `gorm:"not null" json:"is_internal"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index:idx_cmt_deleted" json:"-"`
}

// TableName 指定表名。
func (Comment) TableName() string { return "comments" }

// Attachment 附件（多态）。
type Attachment struct {
	ID         uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	BizType    string         `gorm:"type:varchar(32);not null;index:idx_att_biz" json:"biz_type"`
	BizID      uint64         `gorm:"not null;index:idx_att_biz" json:"biz_id"`
	Filename   string         `gorm:"type:varchar(255);not null" json:"filename"`
	FilePath   string         `gorm:"type:varchar(512);not null" json:"file_path"`
	Size       int64          `gorm:"not null" json:"size"`
	MimeType   string         `gorm:"type:varchar(128)" json:"mime_type"`
	UploaderID uint64         `gorm:"not null;index:idx_att_uploader" json:"uploader_id"`
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index:idx_att_deleted" json:"-"`
}

// TableName 指定表名。
func (Attachment) TableName() string { return "attachments" }

// Migrate 迁移本域实体（由 bootstrap 按拓扑序调用）。
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&User{}, &SLAPolicy{}, &AuditLog{}, &Comment{}, &Attachment{})
}
