// 本文件是 change 域唯一允许直接使用 *gorm.DB 的文件。
package change

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

// NewRepository 构造变更仓储。
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) Create(ctx context.Context, c *Change) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *gormRepository) Update(ctx context.Context, c *Change) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func (r *gormRepository) Get(ctx context.Context, id uint64) (*Change, error) {
	var c Change
	if err := r.db.WithContext(ctx).First(&c, id).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &c, nil
}

func (r *gormRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&Change{}, id).Error
}

func (r *gormRepository) List(ctx context.Context, q ListQuery) ([]Change, int64, error) {
	tx := r.db.WithContext(ctx).Model(&Change{})
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.ChangeType != "" {
		tx = tx.Where("change_type = ?", q.ChangeType)
	}
	if q.RiskLevel != "" {
		tx = tx.Where("risk_level = ?", q.RiskLevel)
	}
	if q.ManagerID != nil {
		tx = tx.Where("manager_id = ?", *q.ManagerID)
	}
	if q.WindowFrom != nil {
		tx = tx.Where("window_start >= ?", *q.WindowFrom)
	}
	if q.WindowTo != nil {
		tx = tx.Where("window_end <= ?", *q.WindowTo)
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		like := "%" + strings.ToLower(kw) + "%"
		tx = tx.Where("LOWER(title) LIKE ? OR LOWER(code) LIKE ?", like, like)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []Change
	query := tx.Order(changeSortColumn(q.SortBy) + " " + orderDir(q.Order))
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
	err := r.db.WithContext(ctx).Model(&Change{}).
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

func (r *gormRepository) CountClosedByIDs(ctx context.Context, ids []uint64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var n int64
	if err := r.db.WithContext(ctx).Model(&Change{}).
		Where("id IN ?", ids).Where("status = ?", StatusClosed).Count(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// gormApprovalRepository 是 ApprovalRepository 的 GORM 实现。
type gormApprovalRepository struct{ db *gorm.DB }

// NewApprovalRepository 构造审批记录仓储。
func NewApprovalRepository(db *gorm.DB) ApprovalRepository {
	return &gormApprovalRepository{db: db}
}

func (r *gormApprovalRepository) Create(ctx context.Context, a *ChangeApproval) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *gormApprovalRepository) ListByChange(ctx context.Context, changeID uint64) ([]ChangeApproval, error) {
	var items []ChangeApproval
	if err := r.db.WithContext(ctx).Where("change_id = ?", changeID).Order("id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *gormApprovalRepository) DeleteByChange(ctx context.Context, changeID uint64) error {
	return r.db.WithContext(ctx).Where("change_id = ?", changeID).Delete(&ChangeApproval{}).Error
}

// FindByChangeAndApprover 先查后写：返回某审批人对某变更已有投票（无则 nil）。
func (r *gormApprovalRepository) FindByChangeAndApprover(ctx context.Context, changeID, approverID uint64) (*ChangeApproval, error) {
	var a ChangeApproval
	err := r.db.WithContext(ctx).
		Where("change_id = ? AND approver_id = ?", changeID, approverID).
		First(&a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

func changeSortColumn(sortBy string) string {
	switch sortBy {
	case "risk_level":
		return "risk_level"
	case "status":
		return "status"
	case "id":
		return "id"
	default:
		return "created_at"
	}
}

func orderDir(order string) string {
	if strings.EqualFold(order, "asc") {
		return "ASC"
	}
	return "DESC"
}
