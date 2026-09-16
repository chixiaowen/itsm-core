package cmdb

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/chixiaowen/itsm-core/internal/domain/platform"
)

// 本文件提供 cmdb 各仓储/协作接口的内存 fake，供单元测试零依赖运行。

// ---------- CIRepository ----------

type fakeCIRepo struct {
	mu    sync.Mutex
	seq   uint64
	items map[uint64]*CI
}

func newFakeCIRepo() *fakeCIRepo { return &fakeCIRepo{items: map[uint64]*CI{}} }

func (r *fakeCIRepo) Create(_ context.Context, ci *CI) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.items {
		if e.Code == ci.Code {
			return errFakeDuplicate
		}
	}
	r.seq++
	ci.ID = r.seq
	cp := *ci
	r.items[ci.ID] = &cp
	return nil
}

func (r *fakeCIRepo) Update(_ context.Context, ci *CI) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[ci.ID]; !ok {
		return ErrNotFound
	}
	cp := *ci
	r.items[ci.ID] = &cp
	return nil
}

func (r *fakeCIRepo) GetByID(_ context.Context, id uint64) (*CI, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ci, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *ci
	return &cp, nil
}

func (r *fakeCIRepo) GetByCode(_ context.Context, code string) (*CI, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ci := range r.items {
		if ci.Code == code {
			cp := *ci
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeCIRepo) List(_ context.Context, q CIListQuery) ([]CI, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kw := strings.ToLower(strings.TrimSpace(q.Keyword))
	var all []CI
	for _, ci := range r.items {
		if q.CIType != "" && ci.CIType != q.CIType {
			continue
		}
		if q.Status != "" && ci.Status != q.Status {
			continue
		}
		if q.OwnerID != nil && (ci.OwnerID == nil || *ci.OwnerID != *q.OwnerID) {
			continue
		}
		if kw != "" && !strings.Contains(strings.ToLower(ci.Code), kw) && !strings.Contains(strings.ToLower(ci.Name), kw) {
			continue
		}
		all = append(all, *ci)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	total := int64(len(all))
	if q.Offset >= len(all) {
		return []CI{}, total, nil
	}
	end := len(all)
	if q.Limit > 0 && q.Offset+q.Limit < end {
		end = q.Offset + q.Limit
	}
	return all[q.Offset:end], total, nil
}

func (r *fakeCIRepo) ListAll(_ context.Context) ([]CI, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]CI, 0, len(r.items))
	for _, ci := range r.items {
		out = append(out, *ci)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeCIRepo) Delete(_ context.Context, id uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

// ---------- RelationRepository ----------

type fakeRelationRepo struct {
	mu    sync.Mutex
	seq   uint64
	items map[uint64]*CIRelation
}

func newFakeRelationRepo() *fakeRelationRepo {
	return &fakeRelationRepo{items: map[uint64]*CIRelation{}}
}

func (r *fakeRelationRepo) Create(_ context.Context, rel *CIRelation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.items {
		if e.SourceCIID == rel.SourceCIID && e.TargetCIID == rel.TargetCIID {
			return errFakeDuplicate
		}
	}
	r.seq++
	rel.ID = r.seq
	cp := *rel
	r.items[rel.ID] = &cp
	return nil
}

func (r *fakeRelationRepo) GetByID(_ context.Context, id uint64) (*CIRelation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rel, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *rel
	return &cp, nil
}

func (r *fakeRelationRepo) Find(_ context.Context, sourceID, targetID uint64) (*CIRelation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rel := range r.items {
		if rel.SourceCIID == sourceID && rel.TargetCIID == targetID {
			cp := *rel
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeRelationRepo) ListByCI(_ context.Context, ciID uint64) ([]CIRelation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []CIRelation
	for _, rel := range r.items {
		if rel.SourceCIID == ciID || rel.TargetCIID == ciID {
			out = append(out, *rel)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeRelationRepo) ListAll(_ context.Context) ([]CIRelation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]CIRelation, 0, len(r.items))
	for _, rel := range r.items {
		out = append(out, *rel)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeRelationRepo) Delete(_ context.Context, id uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
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
