package catalog

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/chixiaowen/itsm-core/internal/domain/platform"
)

// 本文件提供 catalog 各仓储/协作接口的内存 fake，供 service 单测零依赖运行。

// ---------- CategoryRepository ----------

type fakeCategoryRepo struct {
	mu    sync.Mutex
	seq   uint64
	items map[uint64]*ServiceCategory
}

func newFakeCategoryRepo() *fakeCategoryRepo {
	return &fakeCategoryRepo{items: map[uint64]*ServiceCategory{}}
}

func (r *fakeCategoryRepo) Create(_ context.Context, c *ServiceCategory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	c.ID = r.seq
	cp := *c
	r.items[c.ID] = &cp
	return nil
}

func (r *fakeCategoryRepo) Update(_ context.Context, c *ServiceCategory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[c.ID]; !ok {
		return ErrNotFound
	}
	cp := *c
	r.items[c.ID] = &cp
	return nil
}

func (r *fakeCategoryRepo) GetByID(_ context.Context, id uint64) (*ServiceCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *c
	return &cp, nil
}

func (r *fakeCategoryRepo) List(_ context.Context) ([]ServiceCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ServiceCategory, 0, len(r.items))
	for _, c := range r.items {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (r *fakeCategoryRepo) Delete(_ context.Context, id uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *fakeCategoryRepo) CountChildren(_ context.Context, parentID uint64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, c := range r.items {
		if c.ParentID != nil && *c.ParentID == parentID {
			n++
		}
	}
	return n, nil
}

// ---------- ItemRepository ----------

type fakeItemRepo struct {
	mu    sync.Mutex
	seq   uint64
	items map[uint64]*ServiceItem
}

func newFakeItemRepo() *fakeItemRepo { return &fakeItemRepo{items: map[uint64]*ServiceItem{}} }

func (r *fakeItemRepo) Create(_ context.Context, it *ServiceItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	it.ID = r.seq
	cp := *it
	r.items[it.ID] = &cp
	return nil
}

func (r *fakeItemRepo) Update(_ context.Context, it *ServiceItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[it.ID]; !ok {
		return ErrNotFound
	}
	cp := *it
	r.items[it.ID] = &cp
	return nil
}

func (r *fakeItemRepo) GetByID(_ context.Context, id uint64) (*ServiceItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	it, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *it
	return &cp, nil
}

func (r *fakeItemRepo) List(_ context.Context, q ItemListQuery) ([]ServiceItem, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kw := strings.ToLower(strings.TrimSpace(q.Keyword))
	var all []ServiceItem
	for _, it := range r.items {
		if q.Status != "" && it.Status != q.Status {
			continue
		}
		if q.CategoryID != nil && it.CategoryID != *q.CategoryID {
			continue
		}
		if kw != "" && !strings.Contains(strings.ToLower(it.Name), kw) &&
			!strings.Contains(strings.ToLower(it.Description), kw) {
			continue
		}
		all = append(all, *it)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	total := int64(len(all))
	if q.Offset >= len(all) {
		return []ServiceItem{}, total, nil
	}
	end := len(all)
	if q.Limit > 0 && q.Offset+q.Limit < end {
		end = q.Offset + q.Limit
	}
	return all[q.Offset:end], total, nil
}

func (r *fakeItemRepo) ListPublished(_ context.Context, categoryID *uint64, keyword string) ([]ServiceItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kw := strings.ToLower(strings.TrimSpace(keyword))
	var out []ServiceItem
	for _, it := range r.items {
		if it.Status != StatusPublished {
			continue
		}
		if categoryID != nil && it.CategoryID != *categoryID {
			continue
		}
		if kw != "" && !strings.Contains(strings.ToLower(it.Name), kw) {
			continue
		}
		out = append(out, *it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *fakeItemRepo) Delete(_ context.Context, id uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *fakeItemRepo) CountByCategory(_ context.Context, categoryID uint64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, it := range r.items {
		if it.CategoryID == categoryID {
			n++
		}
	}
	return n, nil
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

// ---------- TicketCreator ----------

type fakeTicketCreator struct {
	mu     sync.Mutex
	last   TicketFromServiceItem
	seq    uint64
	failOn error
}

func newFakeTicketCreator() *fakeTicketCreator { return &fakeTicketCreator{} }

func (c *fakeTicketCreator) CreateFromServiceItem(_ context.Context, req TicketFromServiceItem) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failOn != nil {
		return 0, c.failOn
	}
	c.seq++
	c.last = req
	return c.seq, nil
}

func (c *fakeTicketCreator) lastReq() TicketFromServiceItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
}
