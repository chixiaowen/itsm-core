// 本文件定义 incident 域的持久化契约与跨域消费者侧接口。
package incident

import (
	"context"
	"errors"
	"time"

	"github.com/chixiaowen/itsm-core/internal/domain/platform"
)

// ErrNotFound 是仓储层「记录不存在」哨兵错误。
var ErrNotFound = errors.New("事件记录不存在")

// ListQuery 是事件列表查询条件。
type ListQuery struct {
	Status          string
	Priority        string
	Impact          string
	Urgency         string
	EscalationLevel *int
	From            *time.Time
	To              *time.Time
	Keyword         string
	Offset          int
	Limit           int
	SortBy          string
	Order           string
}

// Repository 定义事件持久化契约。
type Repository interface {
	Create(ctx context.Context, i *Incident) error
	Update(ctx context.Context, i *Incident) error
	Get(ctx context.Context, id uint64) (*Incident, error)
	Delete(ctx context.Context, id uint64) error
	List(ctx context.Context, q ListQuery) ([]Incident, int64, error)
	MaxDailySeq(ctx context.Context, prefix string, day time.Time) (int, error)

	// CI 关联（事件↔CI 多对多）。
	ListCIs(ctx context.Context, incidentID uint64) ([]uint64, error)
	AddCIs(ctx context.Context, incidentID uint64, ciIDs []uint64) error
	RemoveCI(ctx context.Context, incidentID, ciID uint64) error

	// 问题聚合（跨域写：由 problem 域发起，写入 incidents.problem_id）。
	AttachToProblem(ctx context.Context, incidentIDs []uint64, problemID uint64) error
	// ListIDsByProblem 返回挂载到某问题的事件 id 列表（供 problem 详情展示）。
	ListIDsByProblem(ctx context.Context, problemID uint64) ([]uint64, error)
	// CountByCISince 统计某 CI 在 since 之后的事件数（建议聚合）。
	CountByCISince(ctx context.Context, ciID uint64, since time.Time) (int, error)
	// CountByKeywordSince 统计标题/描述含关键词且在 since 之后的事件数（建议聚合）。
	CountByKeywordSince(ctx context.Context, keyword string, since time.Time) (int, error)

	// 看板统计。
	CountByStatus(ctx context.Context) (map[string]int64, error)
	CountByPriority(ctx context.Context) (map[string]int64, error)
}

// EscalationRepository 定义事件升级历史持久化契约（只追加）。
type EscalationRepository interface {
	Create(ctx context.Context, e *IncidentEscalation) error
	ListByIncident(ctx context.Context, incidentID uint64) ([]IncidentEscalation, error)
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

// SLAPolicyReader 消费 platform SLA 策略列表能力（platform.Service 直接实现）。
type SLAPolicyReader interface {
	ListSLAPolicies(ctx context.Context) ([]platform.SLAPolicy, error)
}

// TicketCreator 由 ticket service 实现（消费者侧接口，仅用基础类型）。
type TicketCreator interface {
	CreateFromIncident(ctx context.Context, title, description string, requesterID, incidentID uint64, priority string) (ticketID uint64, code string, err error)
}
