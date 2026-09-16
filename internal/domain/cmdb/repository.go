package cmdb

import (
	"context"
	"errors"
)

// ErrNotFound 是仓储层的「记录不存在」哨兵错误。
var ErrNotFound = errors.New("记录不存在")

// CIListQuery 是 CI 列表查询条件。
type CIListQuery struct {
	CIType  string
	Status  string
	OwnerID *uint64
	Keyword string
	Offset  int
	Limit   int
}

// CIRepository 定义 CI 持久化契约。
type CIRepository interface {
	Create(ctx context.Context, ci *CI) error
	Update(ctx context.Context, ci *CI) error
	GetByID(ctx context.Context, id uint64) (*CI, error)
	GetByCode(ctx context.Context, code string) (*CI, error)
	List(ctx context.Context, q CIListQuery) ([]CI, int64, error)
	// ListAll 返回全部 CI（供拓扑图构建使用）。
	ListAll(ctx context.Context) ([]CI, error)
	Delete(ctx context.Context, id uint64) error
}

// RelationRepository 定义 CI 关系持久化契约。
type RelationRepository interface {
	Create(ctx context.Context, rel *CIRelation) error
	GetByID(ctx context.Context, id uint64) (*CIRelation, error)
	// Find 按 source+target 查询关系（用于重复检测）。
	Find(ctx context.Context, sourceID, targetID uint64) (*CIRelation, error)
	// ListByCI 返回与该 CI 相关的全部关系（作为 source 或 target）。
	ListByCI(ctx context.Context, ciID uint64) ([]CIRelation, error)
	// ListAll 返回全部关系（供拓扑图构建使用）。
	ListAll(ctx context.Context) ([]CIRelation, error)
	Delete(ctx context.Context, id uint64) error
}
