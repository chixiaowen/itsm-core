// 本文件是 problem 域唯一允许直接使用 *gorm.DB 的文件。
package problem

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

// NewRepository 构造问题仓储。
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) Create(ctx context.Context, p *Problem) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *gormRepository) Update(ctx context.Context, p *Problem) error {
	return r.db.WithContext(ctx).Save(p).Error
}

func (r *gormRepository) Get(ctx context.Context, id uint64) (*Problem, error) {
	var p Problem
	if err := r.db.WithContext(ctx).First(&p, id).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &p, nil
}

func (r *gormRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&Problem{}, id).Error
}

func (r *gormRepository) List(ctx context.Context, q ListQuery) ([]Problem, int64, error) {
	tx := r.db.WithContext(ctx).Model(&Problem{})
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.AssigneeID != nil {
		tx = tx.Where("assignee_id = ?", *q.AssigneeID)
	}
	// known_error 过滤：true 只看 known_error 状态，false 排除之。
	if q.KnownError != nil {
		if *q.KnownError {
			tx = tx.Where("status = ?", StatusKnownError)
		} else {
			tx = tx.Where("status <> ?", StatusKnownError)
		}
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		like := "%" + strings.ToLower(kw) + "%"
		tx = tx.Where("LOWER(title) LIKE ? OR LOWER(code) LIKE ?", like, like)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []Problem
	query := tx.Order(problemSortColumn(q.SortBy) + " " + orderDir(q.Order))
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
	err := r.db.WithContext(ctx).Model(&Problem{}).
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

// gormProblemChangeRepository 是 ProblemChangeRepository 的 GORM 实现。
type gormProblemChangeRepository struct{ db *gorm.DB }

// NewProblemChangeRepository 构造问题↔变更关联仓储。
func NewProblemChangeRepository(db *gorm.DB) ProblemChangeRepository {
	return &gormProblemChangeRepository{db: db}
}

func (r *gormProblemChangeRepository) Add(ctx context.Context, problemID uint64, changeIDs []uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, cid := range changeIDs {
			var cnt int64
			if err := tx.Model(&ProblemChange{}).
				Where("problem_id = ? AND change_id = ?", problemID, cid).Count(&cnt).Error; err != nil {
				return err
			}
			if cnt > 0 {
				continue // 幂等：已关联则跳过
			}
			if err := tx.Create(&ProblemChange{ProblemID: problemID, ChangeID: cid}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *gormProblemChangeRepository) Remove(ctx context.Context, problemID, changeID uint64) error {
	return r.db.WithContext(ctx).
		Where("problem_id = ? AND change_id = ?", problemID, changeID).
		Delete(&ProblemChange{}).Error
}

func (r *gormProblemChangeRepository) ListChangeIDs(ctx context.Context, problemID uint64) ([]uint64, error) {
	var ids []uint64
	if err := r.db.WithContext(ctx).Model(&ProblemChange{}).
		Where("problem_id = ?", problemID).Order("id ASC").Pluck("change_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func problemSortColumn(sortBy string) string {
	switch sortBy {
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
