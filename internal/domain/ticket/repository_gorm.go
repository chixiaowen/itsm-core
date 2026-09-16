// 本文件是 ticket 域唯一允许直接使用 *gorm.DB 的文件（另一处是 model.Migrate）。
//
// 双库约束：不使用 JSONB/数组/ILIKE/ON CONFLICT/now()/SERIAL；
// 关键字查询在 service 层小写后 LIKE；分页 LIMIT/OFFSET；时间由 Go 传入。
package ticket

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/chixiaowen/itsm-core/internal/pkg/idgen"
)

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// gormRepository 是 Repository 的 GORM 实现。
type gormRepository struct{ db *gorm.DB }

// NewRepository 构造工单仓储。
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) Create(ctx context.Context, t *Ticket) error {
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *gormRepository) Update(ctx context.Context, t *Ticket) error {
	return r.db.WithContext(ctx).Save(t).Error
}

func (r *gormRepository) Get(ctx context.Context, id uint64) (*Ticket, error) {
	var t Ticket
	if err := r.db.WithContext(ctx).First(&t, id).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &t, nil
}

func (r *gormRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&Ticket{}, id).Error
}

func (r *gormRepository) List(ctx context.Context, q ListQuery) ([]Ticket, int64, error) {
	tx := r.db.WithContext(ctx).Model(&Ticket{})
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.Priority != "" {
		tx = tx.Where("priority = ?", q.Priority)
	}
	if q.Type != "" {
		tx = tx.Where("type = ?", q.Type)
	}
	if q.CategoryID != nil {
		tx = tx.Where("category_id = ?", *q.CategoryID)
	}
	if q.AssigneeID != nil {
		tx = tx.Where("assignee_id = ?", *q.AssigneeID)
	}
	if q.RequesterID != nil {
		tx = tx.Where("requester_id = ?", *q.RequesterID)
	}
	if q.From != nil {
		tx = tx.Where("created_at >= ?", *q.From)
	}
	if q.To != nil {
		tx = tx.Where("created_at <= ?", *q.To)
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		like := "%" + strings.ToLower(kw) + "%"
		tx = tx.Where("LOWER(title) LIKE ? OR LOWER(code) LIKE ?", like, like)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	order := sortColumn(q.SortBy) + " " + orderDir(q.Order)
	var items []Ticket
	query := tx.Order(order)
	if q.Limit > 0 {
		query = query.Limit(q.Limit).Offset(q.Offset)
	}
	if err := query.Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *gormRepository) MaxDailySeq(ctx context.Context, prefix string, day time.Time) (int, error) {
	like := prefix + "-" + day.Format("20060102") + "-%"
	var code string
	err := r.db.WithContext(ctx).Model(&Ticket{}).
		Where("code LIKE ?", like).Order("code DESC").Limit(1).Pluck("code", &code).Error
	if err != nil {
		return 0, err
	}
	if code == "" {
		return 0, nil
	}
	seq, ok := idgen.ParseSeq(code, prefix, day)
	if !ok {
		return 0, nil
	}
	return seq, nil
}

func (r *gormRepository) ListCIs(ctx context.Context, ticketID uint64) ([]uint64, error) {
	var ids []uint64
	if err := r.db.WithContext(ctx).Model(&TicketCI{}).
		Where("ticket_id = ?", ticketID).Pluck("ci_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *gormRepository) AddCIs(ctx context.Context, ticketID uint64, ciIDs []uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, ci := range ciIDs {
			var cnt int64
			if err := tx.Model(&TicketCI{}).Where("ticket_id = ? AND ci_id = ?", ticketID, ci).Count(&cnt).Error; err != nil {
				return err
			}
			if cnt > 0 {
				continue
			}
			if err := tx.Create(&TicketCI{TicketID: ticketID, CIID: ci}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *gormRepository) RemoveCI(ctx context.Context, ticketID, ciID uint64) error {
	res := r.db.WithContext(ctx).Where("ticket_id = ? AND ci_id = ?", ticketID, ciID).Delete(&TicketCI{})
	return res.Error
}

// gormCategoryRepository 是 CategoryRepository 的 GORM 实现。
type gormCategoryRepository struct{ db *gorm.DB }

// NewCategoryRepository 构造工单分类仓储。
func NewCategoryRepository(db *gorm.DB) CategoryRepository { return &gormCategoryRepository{db: db} }

func (r *gormCategoryRepository) Create(ctx context.Context, c *TicketCategory) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *gormCategoryRepository) Update(ctx context.Context, c *TicketCategory) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func (r *gormCategoryRepository) Get(ctx context.Context, id uint64) (*TicketCategory, error) {
	var c TicketCategory
	if err := r.db.WithContext(ctx).First(&c, id).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &c, nil
}

func (r *gormCategoryRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&TicketCategory{}, id).Error
}

func (r *gormCategoryRepository) List(ctx context.Context) ([]TicketCategory, error) {
	var items []TicketCategory
	if err := r.db.WithContext(ctx).Order("sort_order ASC, id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *gormCategoryRepository) CountChildren(ctx context.Context, parentID uint64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&TicketCategory{}).Where("parent_id = ?", parentID).Count(&n).Error
	return n, err
}

func (r *gormCategoryRepository) CountByCategory(ctx context.Context, categoryID uint64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&Ticket{}).Where("category_id = ?", categoryID).Count(&n).Error
	return n, err
}

// sortColumn 将排序字段映射为列名（白名单，防注入）。
func sortColumn(sortBy string) string {
	switch sortBy {
	case "priority":
		return "priority"
	case "status":
		return "status"
	case "id":
		return "id"
	default:
		return "created_at"
	}
}

// orderDir 归一化排序方向。
func orderDir(order string) string {
	if strings.EqualFold(order, "asc") {
		return "ASC"
	}
	return "DESC"
}
