package cmdb

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

// 本文件是 cmdb 域唯一允许直接使用 *gorm.DB 的文件（Migrate 在 model.go）。

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// gormCIRepository 是 CIRepository 的 GORM 实现。
type gormCIRepository struct{ db *gorm.DB }

// NewCIRepository 构造 CI 仓储。
func NewCIRepository(db *gorm.DB) CIRepository { return &gormCIRepository{db: db} }

func (r *gormCIRepository) Create(ctx context.Context, ci *CI) error {
	return r.db.WithContext(ctx).Create(ci).Error
}

func (r *gormCIRepository) Update(ctx context.Context, ci *CI) error {
	return r.db.WithContext(ctx).Save(ci).Error
}

func (r *gormCIRepository) GetByID(ctx context.Context, id uint64) (*CI, error) {
	var ci CI
	if err := r.db.WithContext(ctx).First(&ci, id).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &ci, nil
}

func (r *gormCIRepository) GetByCode(ctx context.Context, code string) (*CI, error) {
	var ci CI
	if err := r.db.WithContext(ctx).Where("code = ?", code).First(&ci).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &ci, nil
}

func (r *gormCIRepository) List(ctx context.Context, q CIListQuery) ([]CI, int64, error) {
	tx := r.db.WithContext(ctx).Model(&CI{})
	if q.CIType != "" {
		tx = tx.Where("ci_type = ?", q.CIType)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.OwnerID != nil {
		tx = tx.Where("owner_id = ?", *q.OwnerID)
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		// 双库约束：不使用 ILIKE；应用层小写后用 LIKE。
		like := "%" + strings.ToLower(kw) + "%"
		tx = tx.Where("LOWER(code) LIKE ? OR LOWER(name) LIKE ?", like, like)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []CI
	query := tx.Order("id DESC")
	if q.Limit > 0 {
		query = query.Limit(q.Limit).Offset(q.Offset)
	}
	if err := query.Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *gormCIRepository) ListAll(ctx context.Context) ([]CI, error) {
	var items []CI
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *gormCIRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&CI{}, id).Error
}

// gormRelationRepository 是 RelationRepository 的 GORM 实现。
type gormRelationRepository struct{ db *gorm.DB }

// NewRelationRepository 构造 CI 关系仓储。
func NewRelationRepository(db *gorm.DB) RelationRepository {
	return &gormRelationRepository{db: db}
}

func (r *gormRelationRepository) Create(ctx context.Context, rel *CIRelation) error {
	return r.db.WithContext(ctx).Create(rel).Error
}

func (r *gormRelationRepository) GetByID(ctx context.Context, id uint64) (*CIRelation, error) {
	var rel CIRelation
	if err := r.db.WithContext(ctx).First(&rel, id).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &rel, nil
}

func (r *gormRelationRepository) Find(ctx context.Context, sourceID, targetID uint64) (*CIRelation, error) {
	var rel CIRelation
	if err := r.db.WithContext(ctx).
		Where("source_ci_id = ? AND target_ci_id = ?", sourceID, targetID).
		First(&rel).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &rel, nil
}

func (r *gormRelationRepository) ListByCI(ctx context.Context, ciID uint64) ([]CIRelation, error) {
	var items []CIRelation
	if err := r.db.WithContext(ctx).
		Where("source_ci_id = ? OR target_ci_id = ?", ciID, ciID).
		Order("id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *gormRelationRepository) ListAll(ctx context.Context) ([]CIRelation, error) {
	var items []CIRelation
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *gormRelationRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&CIRelation{}, id).Error
}
