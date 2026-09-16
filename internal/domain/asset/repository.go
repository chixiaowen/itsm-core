package asset

import (
	"context"
	"errors"
	"time"

	"github.com/chixiaowen/itsm-core/internal/domain/cmdb"
)

// ErrNotFound 是仓储层的「记录不存在」哨兵错误。
var ErrNotFound = errors.New("记录不存在")

// AssetListQuery 是资产列表查询条件。
type AssetListQuery struct {
	Category       string
	Status         string
	UserID         *uint64
	WarrantyBefore *time.Time
	Offset         int
	Limit          int
}

// AssetRepository 定义资产持久化契约。
type AssetRepository interface {
	Create(ctx context.Context, a *Asset) error
	Update(ctx context.Context, a *Asset) error
	GetByID(ctx context.Context, id uint64) (*Asset, error)
	// GetByAssetNo 按资产编号查询（编号唯一）。
	GetByAssetNo(ctx context.Context, assetNo string) (*Asset, error)
	// GetByCIID 查询绑定了指定 CI 的资产（用于 1:1 唯一性校验）。
	GetByCIID(ctx context.Context, ciID uint64) (*Asset, error)
	List(ctx context.Context, q AssetListQuery) ([]Asset, int64, error)
	Delete(ctx context.Context, id uint64) error
}

// HistoryRepository 定义资产生命周期历史持久化契约（只追加）。
type HistoryRepository interface {
	Append(ctx context.Context, h *AssetHistory) error
	ListByAsset(ctx context.Context, assetID uint64) ([]AssetHistory, error)
}

// CIReader 是 asset 对 cmdb 域的消费者侧接口（由 cmdb 的 CIRepository 实现）。
//
// 仅暴露按 ID 读取 CI 的最小能力，用于绑定校验与详情展示。
type CIReader interface {
	GetByID(ctx context.Context, id uint64) (*cmdb.CI, error)
}
