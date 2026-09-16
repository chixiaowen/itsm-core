// 本文件是 incident 域唯一允许直接使用 *gorm.DB 的文件。
package incident

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

// NewRepository 构造事件仓储。
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) Create(ctx context.Context, i *Incident) error {
	return r.db.WithContext(ctx).Create(i).Error
}

func (r *gormRepository) Update(ctx context.Context, i *Incident) error {
	return r.db.WithContext(ctx).Save(i).Error
}

func (r *gormRepository) Get(ctx context.Context, id uint64) (*Incident, error) {
	var i Incident
	if err := r.db.WithContext(ctx).First(&i, id).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &i, nil
}

func (r *gormRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&Incident{}, id).Error
}

func (r *gormRepository) List(ctx context.Context, q ListQuery) ([]Incident, int64, error) {
	tx := r.db.WithContext(ctx).Model(&Incident{})
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.Priority != "" {
		tx = tx.Where("priority = ?", q.Priority)
	}
	if q.Impact != "" {
		tx = tx.Where("impact = ?", q.Impact)
	}
	if q.Urgency != "" {
		tx = tx.Where("urgency = ?", q.Urgency)
	}
	if q.EscalationLevel != nil {
		tx = tx.Where("escalation_level = ?", *q.EscalationLevel)
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
	var items []Incident
	query := tx.Order(incidentSortColumn(q.SortBy) + " " + orderDir(q.Order))
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
	err := r.db.WithContext(ctx).Model(&Incident{}).
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

func (r *gormRepository) ListCIs(ctx context.Context, incidentID uint64) ([]uint64, error) {
	var ids []uint64
	if err := r.db.WithContext(ctx).Model(&IncidentCI{}).
		Where("incident_id = ?", incidentID).Pluck("ci_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *gormRepository) AddCIs(ctx context.Context, incidentID uint64, ciIDs []uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, ci := range ciIDs {
			var cnt int64
			if err := tx.Model(&IncidentCI{}).Where("incident_id = ? AND ci_id = ?", incidentID, ci).Count(&cnt).Error; err != nil {
				return err
			}
			if cnt > 0 {
				continue
			}
			if err := tx.Create(&IncidentCI{IncidentID: incidentID, CIID: ci}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *gormRepository) RemoveCI(ctx context.Context, incidentID, ciID uint64) error {
	return r.db.WithContext(ctx).Where("incident_id = ? AND ci_id = ?", incidentID, ciID).Delete(&IncidentCI{}).Error
}

func (r *gormRepository) AttachToProblem(ctx context.Context, incidentIDs []uint64, problemID uint64) error {
	if len(incidentIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&Incident{}).
		Where("id IN ?", incidentIDs).Update("problem_id", problemID).Error
}

func (r *gormRepository) ListIDsByProblem(ctx context.Context, problemID uint64) ([]uint64, error) {
	var ids []uint64
	if err := r.db.WithContext(ctx).Model(&Incident{}).
		Where("problem_id = ?", problemID).Order("id ASC").Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *gormRepository) CountByCISince(ctx context.Context, ciID uint64, since time.Time) (int, error) {
	sub := r.db.Model(&IncidentCI{}).Select("incident_id").Where("ci_id = ?", ciID)
	var n int64
	err := r.db.WithContext(ctx).Model(&Incident{}).
		Where("id IN (?)", sub).Where("created_at >= ?", since).Count(&n).Error
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (r *gormRepository) CountByKeywordSince(ctx context.Context, keyword string, since time.Time) (int, error) {
	like := "%" + strings.ToLower(keyword) + "%"
	var n int64
	err := r.db.WithContext(ctx).Model(&Incident{}).
		Where("LOWER(title) LIKE ? OR LOWER(description) LIKE ?", like, like).
		Where("created_at >= ?", since).Count(&n).Error
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (r *gormRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	type row struct {
		Status string
		N      int64
	}
	var rows []row
	if err := r.db.WithContext(ctx).Model(&Incident{}).
		Select("status, COUNT(*) AS n").Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, x := range rows {
		out[x.Status] = x.N
	}
	return out, nil
}

func (r *gormRepository) CountByPriority(ctx context.Context) (map[string]int64, error) {
	type row struct {
		Priority string
		N        int64
	}
	var rows []row
	if err := r.db.WithContext(ctx).Model(&Incident{}).
		Select("priority, COUNT(*) AS n").Group("priority").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, x := range rows {
		out[x.Priority] = x.N
	}
	return out, nil
}

// gormEscalationRepository 是 EscalationRepository 的 GORM 实现。
type gormEscalationRepository struct{ db *gorm.DB }

// NewEscalationRepository 构造升级历史仓储。
func NewEscalationRepository(db *gorm.DB) EscalationRepository {
	return &gormEscalationRepository{db: db}
}

func (r *gormEscalationRepository) Create(ctx context.Context, e *IncidentEscalation) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *gormEscalationRepository) ListByIncident(ctx context.Context, incidentID uint64) ([]IncidentEscalation, error) {
	var items []IncidentEscalation
	if err := r.db.WithContext(ctx).Where("incident_id = ?", incidentID).Order("id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func incidentSortColumn(sortBy string) string {
	switch sortBy {
	case "priority":
		return "priority"
	case "status":
		return "status"
	case "escalation_level":
		return "escalation_level"
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
