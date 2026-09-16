// 本文件定义 change 域的持久化契约与跨域消费者侧接口。
package change

import (
	"context"
	"errors"
	"time"

	"github.com/chixiaowen/itsm-core/internal/domain/platform"
)

// ErrNotFound 是仓储层「记录不存在」哨兵错误。
var ErrNotFound = errors.New("变更记录不存在")

// ListQuery 是变更列表查询条件。
type ListQuery struct {
	Status     string
	ChangeType string
	RiskLevel  string
	ManagerID  *uint64
	WindowFrom *time.Time
	WindowTo   *time.Time
	Keyword    string
	Offset     int
	Limit      int
	SortBy     string
	Order      string
}

// Repository 定义变更持久化契约。
type Repository interface {
	Create(ctx context.Context, c *Change) error
	Update(ctx context.Context, c *Change) error
	Get(ctx context.Context, id uint64) (*Change, error)
	Delete(ctx context.Context, id uint64) error
	List(ctx context.Context, q ListQuery) ([]Change, int64, error)
	MaxDailySeq(ctx context.Context, prefix string, day time.Time) (int, error)
	// CountClosedByIDs 统计给定变更中状态为 closed 的数量（供 problem 解得约束校验）。
	CountClosedByIDs(ctx context.Context, ids []uint64) (int, error)
}

// ApprovalRepository 定义审批记录持久化契约。
type ApprovalRepository interface {
	Create(ctx context.Context, a *ChangeApproval) error
	ListByChange(ctx context.Context, changeID uint64) ([]ChangeApproval, error)
	DeleteByChange(ctx context.Context, changeID uint64) error
	// FindByChangeAndApprover 查询某审批人对某变更已有的投票记录（无则返回 nil, nil）。
	// 用于 RecordApproval 的「先查后写」防重复投票（对齐 PRD §8.6：禁 ON CONFLICT）。
	FindByChangeAndApprover(ctx context.Context, changeID, approverID uint64) (*ChangeApproval, error)
}

// ---------------- 跨域消费者侧接口（由 T11 注入真实实现）----------------

// Auditor 消费 platform 审计能力（platform.Service 直接实现）。
type Auditor interface {
	AppendAudit(ctx context.Context, e platform.AuditEntry) error
}

// UserDirectory 消费 platform 用户查询能力（platform.Service 直接实现）。
type UserDirectory interface {
	GetUser(ctx context.Context, id uint64) (*platform.User, error)
}
