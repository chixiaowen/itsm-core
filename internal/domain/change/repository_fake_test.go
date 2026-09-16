package change

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chixiaowen/itsm-core/internal/domain/platform"
	"github.com/chixiaowen/itsm-core/internal/pkg/idgen"
)

// 本文件提供 change 域各依赖的内存 fake。

type fakeRepo struct {
	mu    sync.Mutex
	seq   uint64
	items map[uint64]*Change
}

func newFakeRepo() *fakeRepo { return &fakeRepo{items: map[uint64]*Change{}} }

func (r *fakeRepo) Create(_ context.Context, c *Change) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	c.ID = r.seq
	cp := *c
	r.items[c.ID] = &cp
	return nil
}

func (r *fakeRepo) Update(_ context.Context, c *Change) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[c.ID]; !ok {
		return ErrNotFound
	}
	cp := *c
	r.items[c.ID] = &cp
	return nil
}

func (r *fakeRepo) Get(_ context.Context, id uint64) (*Change, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *c
	return &cp, nil
}

func (r *fakeRepo) Delete(_ context.Context, id uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *fakeRepo) List(_ context.Context, q ListQuery) ([]Change, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kw := strings.ToLower(strings.TrimSpace(q.Keyword))
	var all []Change
	for _, c := range r.items {
		if q.Status != "" && c.Status != q.Status {
			continue
		}
		if q.ChangeType != "" && c.ChangeType != q.ChangeType {
			continue
		}
		if q.RiskLevel != "" && c.RiskLevel != q.RiskLevel {
			continue
		}
		if q.ManagerID != nil && (c.ManagerID == nil || *c.ManagerID != *q.ManagerID) {
			continue
		}
		if kw != "" && !strings.Contains(strings.ToLower(c.Title), kw) && !strings.Contains(strings.ToLower(c.Code), kw) {
			continue
		}
		all = append(all, *c)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	total := int64(len(all))
	if q.Limit <= 0 {
		return all, total, nil
	}
	if q.Offset >= len(all) {
		return []Change{}, total, nil
	}
	end := len(all)
	if q.Offset+q.Limit < end {
		end = q.Offset + q.Limit
	}
	return all[q.Offset:end], total, nil
}

func (r *fakeRepo) MaxDailySeq(_ context.Context, prefix string, day time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	max := 0
	for _, c := range r.items {
		if seq, ok := idgen.ParseSeq(c.Code, prefix, day); ok && seq > max {
			max = seq
		}
	}
	return max, nil
}

func (r *fakeRepo) CountClosedByIDs(_ context.Context, ids []uint64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, id := range ids {
		if c, ok := r.items[id]; ok && c.Status == StatusClosed {
			n++
		}
	}
	return n, nil
}

type fakeApprovals struct {
	mu    sync.Mutex
	seq   uint64
	items []ChangeApproval
}

func newFakeApprovals() *fakeApprovals { return &fakeApprovals{} }

func (r *fakeApprovals) Create(_ context.Context, a *ChangeApproval) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	a.ID = r.seq
	r.items = append(r.items, *a)
	return nil
}

func (r *fakeApprovals) ListByChange(_ context.Context, changeID uint64) ([]ChangeApproval, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []ChangeApproval
	for _, a := range r.items {
		if a.ChangeID == changeID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (r *fakeApprovals) DeleteByChange(_ context.Context, changeID uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.items[:0]
	for _, a := range r.items {
		if a.ChangeID != changeID {
			out = append(out, a)
		}
	}
	r.items = out
	return nil
}

func (r *fakeApprovals) FindByChangeAndApprover(_ context.Context, changeID, approverID uint64) (*ChangeApproval, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.items {
		if a.ChangeID == changeID && a.ApproverID == approverID {
			cp := a
			return &cp, nil
		}
	}
	return nil, nil
}

type fakeAuditor struct {
	mu     sync.Mutex
	audits []platform.AuditEntry
}

func newFakeAuditor() *fakeAuditor { return &fakeAuditor{} }

func (f *fakeAuditor) AppendAudit(_ context.Context, e platform.AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audits = append(f.audits, e)
	return nil
}

type fakeUsers struct {
	mu    sync.Mutex
	items map[uint64]*platform.User
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{items: map[uint64]*platform.User{
		7: {ID: 7, Username: "cm", Role: "change_manager"},
	}}
}

func (f *fakeUsers) GetUser(_ context.Context, id uint64) (*platform.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.items[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *u
	return &cp, nil
}
