package catalog

import (
	"context"
	"errors"
)

// ErrNotFound 是仓储层的「记录不存在」哨兵错误。
var ErrNotFound = errors.New("记录不存在")

// CategoryRepository 定义服务分类持久化契约。
type CategoryRepository interface {
	Create(ctx context.Context, c *ServiceCategory) error
	Update(ctx context.Context, c *ServiceCategory) error
	GetByID(ctx context.Context, id uint64) (*ServiceCategory, error)
	// List 返回全部分类（按 sort_order,id 升序）。
	List(ctx context.Context) ([]ServiceCategory, error)
	Delete(ctx context.Context, id uint64) error
	// CountChildren 统计直接子分类数量。
	CountChildren(ctx context.Context, parentID uint64) (int64, error)
}

// ItemListQuery 是服务项列表查询条件。
type ItemListQuery struct {
	Status     string
	CategoryID *uint64
	Keyword    string
	Offset     int
	Limit      int
}

// ItemRepository 定义服务项持久化契约。
type ItemRepository interface {
	Create(ctx context.Context, it *ServiceItem) error
	Update(ctx context.Context, it *ServiceItem) error
	GetByID(ctx context.Context, id uint64) (*ServiceItem, error)
	List(ctx context.Context, q ItemListQuery) ([]ServiceItem, int64, error)
	// ListPublished 返回 published 服务项（用户侧，支持分类与关键字过滤）。
	ListPublished(ctx context.Context, categoryID *uint64, keyword string) ([]ServiceItem, error)
	Delete(ctx context.Context, id uint64) error
	// CountByCategory 统计某分类下的服务项数量（不含已删除）。
	CountByCategory(ctx context.Context, categoryID uint64) (int64, error)
}
