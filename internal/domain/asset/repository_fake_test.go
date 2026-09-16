package asset

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/chixiaowen/itsm-core/internal/domain/cmdb"
	"github.com/chixiaowen/itsm-core/internal/domain/platform"
)

// 本文件提供 asset 各仓储/协作接口的内存 fake，供单元测试零依赖运行。

var errFakeDuplicate = errors.New("fake: duplicate")

// ---------- AssetRepository ----------

type fakeAssetRepo struct {
	mu    sync.Mutex
	seq   uint64
	items map[uint64]*Asset
}

func newFakeAssetRepo() *fakeAssetRepo { return &fakeAssetRepo{items: map[uint64]*Asset{}} }

func (r *fakeAssetRepo) Create(_ context.Context, a *Asset) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.items {
		if e.AssetNo == a.AssetNo {
			return errFakeDuplicate
		}
		if e.CIID != nil && a.CIID != nil && *e.CIID == *a.CIID {
			return errFakeDuplicate
		}
	}
	r.seq++
	a.ID = r.seq
	cp := *a
	r.items[a.ID] = &cp
	return nil
}

func (r *fakeAssetRepo) Update(_ context.Context, a *Asset) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[a.ID]; !ok {
		return ErrNotFound
	}
	for id, e := range r.items {
		if id == a.ID {
			continue
		}
		if e.AssetNo == a.AssetNo {
			return errFakeDuplicate
		}
		if e.CIID != nil && a.CIID != nil && *e.CIID == *a.CIID {
			return errFakeDuplicate
		}
	}
	cp := *a
	r.items[a.ID] = &cp
	return nil
}

func (r *fakeAssetRepo) GetByID(_ context.Context, id uint64) (*Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (r *fakeAssetRepo) GetByAssetNo(_ context.Context, assetNo string) (*Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.items {
		if a.AssetNo == assetNo {
			cp := *a
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeAssetRepo) GetByCIID(_ context.Context, ciID uint64) (*Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.items {
		if a.CIID != nil && *a.CIID == ciID {
			cp := *a
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeAssetRepo) List(_ context.Context, q AssetListQuery) ([]Asset, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var all []Asset
	for _, a := range r.items {
		if q.Category != "" && a.Category != q.Category {
			continue
		}
		if q.Status != "" && a.Status != q.Status {
			continue
		}
		if q.UserID != nil && (a.UserID == nil || *a.UserID != *q.UserID) {
			continue
		}
		if q.WarrantyBefore != nil && (a.WarrantyEnd == nil || a.WarrantyEnd.After(*q.WarrantyBefore)) {
			continue
		}
		all = append(all, *a)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	total := int64(len(all))
	if q.Offset >= len(all) {
		return []Asset{}, total, nil
	}
	end := len(all)
	if q.Limit > 0 && q.Offset+q.Limit < end {
		end = q.Offset + q.Limit
	}
	return all[q.Offset:end], total, nil
}

func (r *fakeAssetRepo) Delete(_ context.Context, id uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

// ---------- HistoryRepository ----------

type fakeHistoryRepo struct {
	mu    sync.Mutex
	seq   uint64
	items []AssetHistory
}

func newFakeHistoryRepo() *fakeHistoryRepo { return &fakeHistoryRepo{} }

func (r *fakeHistoryRepo) Append(_ context.Context, h *AssetHistory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	h.ID = r.seq
	r.items = append(r.items, *h)
	return nil
}

func (r *fakeHistoryRepo) ListByAsset(_ context.Context, assetID uint64) ([]AssetHistory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []AssetHistory
	for _, h := range r.items {
		if h.AssetID == assetID {
			out = append(out, h)
		}
	}
	return out, nil
}

func (r *fakeHistoryRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.items)
}

// ---------- CIReader ----------

type fakeCIReader struct {
	mu    sync.Mutex
	items map[uint64]cmdb.CI
}

func newFakeCIReader() *fakeCIReader { return &fakeCIReader{items: map[uint64]cmdb.CI{}} }

func (r *fakeCIReader) put(ci cmdb.CI) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[ci.ID] = ci
}

func (r *fakeCIReader) GetByID(_ context.Context, id uint64) (*cmdb.CI, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ci, ok := r.items[id]
	if !ok {
		return nil, cmdb.ErrNotFound
	}
	cp := ci
	return &cp, nil
}

// ---------- AuditWriter ----------

type fakeAuditWriter struct {
	mu      sync.Mutex
	entries []platform.AuditEntry
}

func newFakeAuditWriter() *fakeAuditWriter { return &fakeAuditWriter{} }

func (w *fakeAuditWriter) AppendAudit(_ context.Context, e platform.AuditEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entries = append(w.entries, e)
	return nil
}

func (w *fakeAuditWriter) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.entries)
}
