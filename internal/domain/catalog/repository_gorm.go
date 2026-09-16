package catalog

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

// 本文件是 catalog 域唯一允许直接使用 *gorm.DB 的文件（Migrate 在 model.go）。

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// gormCategoryRepository 是 CategoryRepository 的 GORM 实现。
type gormCategoryRepository struct{ db *gorm.DB }

// NewCategoryRepository 构造服务分类仓储。
func NewCategoryRepository(db *gorm.DB) CategoryRepository { return &gormCategoryRepository{db: db} }

func (r *gormCategoryRepository) Create(ctx context.Context, c *ServiceCategory) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *gormCategoryRepository) Update(ctx context.Context, c *ServiceCategory) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func (r *gormCategoryRepository) GetByID(ctx context.Context, id uint64) (*ServiceCategory, error) {
	var c ServiceCategory
	if err := r.db.WithContext(ctx).First(&c, id).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &c, nil
}

func (r *gormCategoryRepository) List(ctx context.Context) ([]ServiceCategory, error) {
	var items []ServiceCategory
	if err := r.db.WithContext(ctx).Order("sort_order ASC, id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *gormCategoryRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&ServiceCategory{}, id).Error
}

func (r *gormCategoryRepository) CountChildren(ctx context.Context, parentID uint64) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&ServiceCategory{}).
		Where("parent_id = ?", parentID).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// gormItemRepository 是 ItemRepository 的 GORM 实现。
type gormItemRepository struct{ db *gorm.DB }

// NewItemRepository 构造服务项仓储。
func NewItemRepository(db *gorm.DB) ItemRepository { return &gormItemRepository{db: db} }

func (r *gormItemRepository) Create(ctx context.Context, it *ServiceItem) error {
	return r.db.WithContext(ctx).Create(it).Error
}

func (r *gormItemRepository) Update(ctx context.Context, it *ServiceItem) error {
	return r.db.WithContext(ctx).Save(it).Error
}

func (r *gormItemRepository) GetByID(ctx context.Context, id uint64) (*ServiceItem, error) {
	var it ServiceItem
	if err := r.db.WithContext(ctx).First(&it, id).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &it, nil
}

// applyItemFilters 组装服务项筛选条件（双库约束：不使用 ILIKE）。
func applyItemFilters(tx *gorm.DB, status string, categoryID *uint64, keyword string) *gorm.DB {
	if status != "" {
		tx = tx.Where("status = ?", status)
	}
	if categoryID != nil {
		tx = tx.Where("category_id = ?", *categoryID)
	}
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + strings.ToLower(kw) + "%"
		tx = tx.Where("LOWER(name) LIKE ? OR LOWER(description) LIKE ?", like, like)
	}
	return tx
}

func (r *gormItemRepository) List(ctx context.Context, q ItemListQuery) ([]ServiceItem, int64, error) {
	tx := applyItemFilters(r.db.WithContext(ctx).Model(&ServiceItem{}), q.Status, q.CategoryID, q.Keyword)

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []ServiceItem
	query := tx.Order("id DESC")
	if q.Limit > 0 {
		query = query.Limit(q.Limit).Offset(q.Offset)
	}
	if err := query.Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *gormItemRepository) ListPublished(ctx context.Context, categoryID *uint64, keyword string) ([]ServiceItem, error) {
	tx := applyItemFilters(r.db.WithContext(ctx).Model(&ServiceItem{}), StatusPublished, categoryID, keyword)
	var items []ServiceItem
	if err := tx.Order("id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *gormItemRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&ServiceItem{}, id).Error
}

func (r *gormItemRepository) CountByCategory(ctx context.Context, categoryID uint64) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&ServiceItem{}).
		Where("category_id = ?", categoryID).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}
