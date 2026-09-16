// 本文件定义 ticket 域的持久化契约（Go interface）与跨域消费者侧接口。
//
// 铁律：仓储接口参数/返回值不得出现 gorm 类型；跨域调用通过本包内定义的
// 最小消费者接口完成（由 T11 装配时注入真实实现），业务域之间零编译耦合。
package ticket

import (
	"context"
	"errors"
	"time"

	"github.com/chixiaowen/itsm-core/internal/domain/platform"
)

// ErrNotFound 是仓储层「记录不存在」哨兵错误，由 service 映射为 404。
var ErrNotFound = errors.New("工单记录不存在")

// ListQuery 是工单列表查询条件（分页在仓储层用 LIMIT/OFFSET）。
type ListQuery struct {
	Status      string
	Priority    string
	Type        string
	CategoryID  *uint64
	AssigneeID  *uint64
	RequesterID *uint64
	// SLAStatus 由 service 在应用层过滤（见 service.List）；仓储仅透传。
	SLAStatus string
	From      *time.Time
	To        *time.Time
	Keyword   string
	Offset    int
	Limit     int // <=0 表示不分页（取全部，用于应用层 SLA 过滤）
	SortBy    string
	Order     string
}

// Repository 定义工单持久化契约。
type Repository interface {
	Create(ctx context.Context, t *Ticket) error
	Update(ctx context.Context, t *Ticket) error
	Get(ctx context.Context, id uint64) (*Ticket, error)
	Delete(ctx context.Context, id uint64) error
	List(ctx context.Context, q ListQuery) ([]Ticket, int64, error)
	// MaxDailySeq 返回指定前缀 + 日期当日已存在的最大编号序号（无则 0）。
	MaxDailySeq(ctx context.Context, prefix string, day time.Time) (int, error)

	// 关联 CI（工单↔CI 多对多）。
	ListCIs(ctx context.Context, ticketID uint64) ([]uint64, error)
	AddCIs(ctx context.Context, ticketID uint64, ciIDs []uint64) error
	RemoveCI(ctx context.Context, ticketID, ciID uint64) error
}

// CategoryRepository 定义工单分类持久化契约。
type CategoryRepository interface {
	Create(ctx context.Context, c *TicketCategory) error
	Update(ctx context.Context, c *TicketCategory) error
	Get(ctx context.Context, id uint64) (*TicketCategory, error)
	Delete(ctx context.Context, id uint64) error
	List(ctx context.Context) ([]TicketCategory, error)
	CountChildren(ctx context.Context, parentID uint64) (int64, error)
	CountByCategory(ctx context.Context, categoryID uint64) (int64, error)
}

// ---------------- 跨域消费者侧接口（由 T11 注入真实实现）----------------

// Auditor 消费 platform 的审计写入能力（platform.Service 直接实现）。
type Auditor interface {
	AppendAudit(ctx context.Context, e platform.AuditEntry) error
}

// UserDirectory 消费 platform 的用户查询能力（platform.Service 直接实现）。
type UserDirectory interface {
	GetUser(ctx context.Context, id uint64) (*platform.User, error)
}

// SLAPolicyReader 消费 platform 的 SLA 策略列表能力（platform.Service 直接实现）。
type SLAPolicyReader interface {
	ListSLAPolicies(ctx context.Context) ([]platform.SLAPolicy, error)
}

// CommentStore 消费 platform 的评论能力（platform.Service 直接实现）。
type CommentStore interface {
	Auditor
	ListComments(ctx context.Context, bizType string, bizID uint64, includeInternal bool) ([]platform.Comment, error)
	CreateComment(ctx context.Context, op platform.Operator, req platform.CommentRequest) (*platform.Comment, error)
}
