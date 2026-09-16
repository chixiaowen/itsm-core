// 本文件定义 problem 域的持久化契约与跨域消费者侧接口。
package problem

import (
	"context"
	"errors"
	"time"

	"github.com/chixiaowen/itsm-core/internal/domain/platform"
)

// ErrNotFound 是仓储层「记录不存在」哨兵错误。
var ErrNotFound = errors.New("问题记录不存在")

// ListQuery 是问题列表查询条件。
type ListQuery struct {
	Status     string
	AssigneeID *uint64
	KnownError *bool
	Keyword    string
	Offset     int
	Limit      int
	SortBy     string
	Order      string
}

// Repository 定义问题持久化契约。
type Repository interface {
	Create(ctx context.Context, p *Problem) error
	Update(ctx context.Context, p *Problem) error
	Get(ctx context.Context, id uint64) (*Problem, error)
	Delete(ctx context.Context, id uint64) error
	List(ctx context.Context, q ListQuery) ([]Problem, int64, error)
	MaxDailySeq(ctx context.Context, prefix string, day time.Time) (int, error)
}

// ProblemChangeRepository 定义问题↔变更关联持久化契约。
type ProblemChangeRepository interface {
	Add(ctx context.Context, problemID uint64, changeIDs []uint64) error
	Remove(ctx context.Context, problemID, changeID uint64) error
	ListChangeIDs(ctx context.Context, problemID uint64) ([]uint64, error)
}

// ---------------- 跨域消费者侧接口（由 T11 注入真实实现）----------------

// IncidentReader 消费 incident 域能力（incident.Service 直接实现）。
type IncidentReader interface {
	// GetIncidentStatus 返回事件状态与已挂载问题（found=false 表示不存在）。
	GetIncidentStatus(ctx context.Context, incidentID uint64) (status string, problemID uint64, hasProblem bool, found bool, err error)
	// AttachIncidentsToProblem 批量写入 incidents.problem_id（跨域写）。
	AttachIncidentsToProblem(ctx context.Context, incidentIDs []uint64, problemID uint64) error
	// ListIncidentIDsByProblem 返回挂载到某问题的事件 id 列表。
	ListIncidentIDsByProblem(ctx context.Context, problemID uint64) ([]uint64, error)
	// CountIncidentsByCI 统计某 CI 在 since 之后的事件数（建议聚合）。
	CountIncidentsByCI(ctx context.Context, ciID uint64, since time.Time) (int, error)
	// CountIncidentsByKeyword 统计含关键词且在 since 之后的事件数（建议聚合）。
	CountIncidentsByKeyword(ctx context.Context, keyword string, since time.Time) (int, error)
}

// ChangeReader 消费 change 域能力（change.Service 直接实现）。
type ChangeReader interface {
	// ClosedChangeCount 返回给定变更中状态为 closed 的数量。
	ClosedChangeCount(ctx context.Context, changeIDs []uint64) (int, error)
}

// Auditor 消费 platform 审计能力（platform.Service 直接实现）。
type Auditor interface {
	AppendAudit(ctx context.Context, e platform.AuditEntry) error
}

// UserDirectory 消费 platform 用户查询能力（platform.Service 直接实现）。
type UserDirectory interface {
	GetUser(ctx context.Context, id uint64) (*platform.User, error)
}
