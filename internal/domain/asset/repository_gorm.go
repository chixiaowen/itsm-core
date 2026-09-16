package asset

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// 本文件是 asset 域唯一允许直接使用 *gorm.DB 的文件（Migrate 在 model.go）。

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// gormAssetRepository 是 AssetRepository 的 GORM 实现。
type gormAssetRepository struct{ db *gorm.DB }

// NewAssetRepository 构造资产仓储。
func NewAssetRepository(db *gorm.DB) AssetRepository { return &gormAssetRepository{db: db} }

func (r *gormAssetRepository) Create(ctx context.Context, a *Asset) error {
	return r.db.WithContext(ctx).Create(a).Error
}

func (r *gormAssetRepository) Update(ctx context.Context, a *Asset) error {
	return r.db.WithContext(ctx).Save(a).Error
}

func (r *gormAssetRepository) GetByID(ctx context.Context, id uint64) (*Asset, error) {
	var a Asset
	if err := r.db.WithContext(ctx).First(&a, id).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &a, nil
}

func (r *gormAssetRepository) GetByAssetNo(ctx context.Context, assetNo string) (*Asset, error) {
	var a Asset
	if err := r.db.WithContext(ctx).Where("asset_no = ?", assetNo).First(&a).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &a, nil
}

func (r *gormAssetRepository) GetByCIID(ctx context.Context, ciID uint64) (*Asset, error) {
	var a Asset
	if err := r.db.WithContext(ctx).Where("ci_id = ?", ciID).First(&a).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return &a, nil
}

func (r *gormAssetRepository) List(ctx context.Context, q AssetListQuery) ([]Asset, int64, error) {
	tx := r.db.WithContext(ctx).Model(&Asset{})
	if q.Category != "" {
		tx = tx.Where("category = ?", q.Category)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.UserID != nil {
		tx = tx.Where("user_id = ?", *q.UserID)
	}
	if q.WarrantyBefore != nil {
		tx = tx.Where("warranty_end IS NOT NULL AND warranty_end <= ?", *q.WarrantyBefore)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []Asset
	query := tx.Order("id DESC")
	if q.Limit > 0 {
		query = query.Limit(q.Limit).Offset(q.Offset)
	}
	if err := query.Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *gormAssetRepository) Delete(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Delete(&Asset{}, id).Error
}

// gormHistoryRepository 是 HistoryRepository 的 GORM 实现（只追加，不软删除）。
type gormHistoryRepository struct{ db *gorm.DB }

// NewHistoryRepository 构造资产历史仓储。
func NewHistoryRepository(db *gorm.DB) HistoryRepository { return &gormHistoryRepository{db: db} }

func (r *gormHistoryRepository) Append(ctx context.Context, h *AssetHistory) error {
	return r.db.WithContext(ctx).Create(h).Error
}

func (r *gormHistoryRepository) ListByAsset(ctx context.Context, assetID uint64) ([]AssetHistory, error) {
	var items []AssetHistory
	if err := r.db.WithContext(ctx).Where("asset_id = ?", assetID).
		Order("id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
