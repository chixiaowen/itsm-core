package platform

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound 是仓储层的「记录不存在」哨兵错误。
//
// 仓储实现把底层驱动的 not-found 归一化为该错误，service 再映射为 404，
// 从而避免 gorm 错误类型泄漏到 service / handler 层。
var ErrNotFound = errors.New("记录不存在")

// UserListQuery 是用户列表查询条件。
type UserListQuery struct {
	Role    string
	Status  string
	Keyword string
	Offset  int
	Limit   int
}

// AuditListQuery 是审计日志查询条件。
//
// BizType/BizID 与 EntityType/EntityID 为同一组过滤条件的两种命名：
// 面向「业务对象」（biz_type/biz_id）与面向「实体」（entity_type/entity_id）的调用方
// 均可使用；两者同时存在时以 BizType/BizID 优先，最终都映射到 biz_type/biz_id 列。
type AuditListQuery struct {
	ActorID    uint64
	BizType    string
	BizID      uint64
	EntityType string
	EntityID   uint64
	Action     string
	From       *time.Time
	To         *time.Time
	Offset     int
	Limit      int
}

// auditBizFilter 归一化 biz/entity 两组过滤条件，返回最终用于 biz_type/biz_id 列的取值。
func (q AuditListQuery) auditBizFilter() (string, uint64) {
	bizType, bizID := q.BizType, q.BizID
	if bizType == "" {
		bizType = q.EntityType
	}
	if bizID == 0 {
		bizID = q.EntityID
	}
	return bizType, bizID
}

// UserRepository 定义用户持久化契约。
type UserRepository interface {
	Create(ctx context.Context, u *User) error
	Update(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uint64) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	Delete(ctx context.Context, id uint64) error
	List(ctx context.Context, q UserListQuery) ([]User, int64, error)
	// ListOptions 返回候选用户的下拉项（仅 active 且未软删除；role 为空表示不过滤）。
	ListOptions(ctx context.Context, role string) ([]UserOption, error)
}

// SLAPolicyRepository 定义 SLA 策略持久化契约。
type SLAPolicyRepository interface {
	Create(ctx context.Context, p *SLAPolicy) error
	Update(ctx context.Context, p *SLAPolicy) error
	GetByID(ctx context.Context, id uint64) (*SLAPolicy, error)
	GetByPriority(ctx context.Context, priority string) (*SLAPolicy, error)
	List(ctx context.Context) ([]SLAPolicy, error)
	Delete(ctx context.Context, id uint64) error
}

// AuditRepository 定义审计日志持久化契约（只追加）。
type AuditRepository interface {
	Append(ctx context.Context, entry *AuditLog) error
	List(ctx context.Context, q AuditListQuery) ([]AuditLog, int64, error)
}

// CommentRepository 定义评论持久化契约（多态 by biz_type+biz_id）。
type CommentRepository interface {
	Create(ctx context.Context, c *Comment) error
	ListByBiz(ctx context.Context, bizType string, bizID uint64, includeInternal bool) ([]Comment, error)
}

// AttachmentRepository 定义附件持久化契约（多态）。
type AttachmentRepository interface {
	Create(ctx context.Context, a *Attachment) error
	GetByID(ctx context.Context, id uint64) (*Attachment, error)
	ListByBiz(ctx context.Context, bizType string, bizID uint64) ([]Attachment, error)
	Delete(ctx context.Context, id uint64) error
}
