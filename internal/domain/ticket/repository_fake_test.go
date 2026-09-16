package ticket

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

// 本文件提供 ticket 域各依赖的内存 fake，供 service / handler 单测零依赖运行。

var errFakeDuplicate = errors.New("fake: duplicate")

// ---------- Repository ----------

type fakeRepo struct {
	mu    sync.Mutex
	seq   uint64
	items map[uint64]*Ticket
	cis   map[uint64][]uint64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[uint64]*Ticket{}, cis: map[uint64][]uint64{}}
}

func (r *fakeRepo) Create(_ context.Context, t *Ticket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	t.ID = r.seq
	cp := *t
	r.items[t.ID] = &cp
	return nil
}

func (r *fakeRepo) Update(_ context.Context, t *Ticket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[t.ID]; !ok {
		return ErrNotFound
	}
	cp := *t
	r.items[t.ID] = &cp
	return nil
}

func (r *fakeRepo) Get(_ context.Context, id uint64) (*Ticket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *t
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

func (r *fakeRepo) List(_ context.Context, q ListQuery) ([]Ticket, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kw := strings.ToLower(strings.TrimSpace(q.Keyword))
	var all []Ticket
	for _, t := range r.items {
		if q.Status != "" && t.Status != q.Status {
			continue
		}
		if q.Priority != "" && t.Priority != q.Priority {
			continue
		}
		if q.Type != "" && t.Type != q.Type {
			continue
		}
		if q.CategoryID != nil && (t.CategoryID == nil || *t.CategoryID != *q.CategoryID) {
			continue
		}
		if q.AssigneeID != nil && (t.AssigneeID == nil || *t.AssigneeID != *q.AssigneeID) {
			continue
		}
		if q.RequesterID != nil && t.RequesterID != *q.RequesterID {
			continue
		}
		if q.From != nil && t.CreatedAt.Before(*q.From) {
			continue
		}
		if q.To != nil && t.CreatedAt.After(*q.To) {
			continue
		}
		if kw != "" && !strings.Contains(strings.ToLower(t.Title), kw) && !strings.Contains(strings.ToLower(t.Code), kw) {
			continue
		}
		all = append(all, *t)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	total := int64(len(all))
	if q.Limit <= 0 {
		return all, total, nil
	}
	if q.Offset >= len(all) {
		return []Ticket{}, total, nil
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
	for _, t := range r.items {
		if seq, ok := idgen.ParseSeq(t.Code, prefix, day); ok && seq > max {
			max = seq
		}
	}
	return max, nil
}

func (r *fakeRepo) ListCIs(_ context.Context, ticketID uint64) ([]uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]uint64{}, r.cis[ticketID]...)
	return out, nil
}

func (r *fakeRepo) AddCIs(_ context.Context, ticketID uint64, ciIDs []uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ci := range ciIDs {
		exists := false
		for _, e := range r.cis[ticketID] {
			if e == ci {
				exists = true
				break
			}
		}
		if !exists {
			r.cis[ticketID] = append(r.cis[ticketID], ci)
		}
	}
	return nil
}

func (r *fakeRepo) RemoveCI(_ context.Context, ticketID, ciID uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.cis[ticketID]
	out := list[:0]
	for _, e := range list {
		if e != ciID {
			out = append(out, e)
		}
	}
	r.cis[ticketID] = out
	return nil
}

// ---------- CategoryRepository ----------

type fakeCategoryRepo struct {
	mu          sync.Mutex
	seq         uint64
	items       map[uint64]*TicketCategory
	ticketCount map[uint64]int64 // 模拟分类下工单数（用于删除校验）
}

func newFakeCategoryRepo() *fakeCategoryRepo {
	return &fakeCategoryRepo{items: map[uint64]*TicketCategory{}, ticketCount: map[uint64]int64{}}
}

func (r *fakeCategoryRepo) Create(_ context.Context, c *TicketCategory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	c.ID = r.seq
	cp := *c
	r.items[c.ID] = &cp
	return nil
}

func (r *fakeCategoryRepo) Update(_ context.Context, c *TicketCategory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[c.ID]; !ok {
		return ErrNotFound
	}
	cp := *c
	r.items[c.ID] = &cp
	return nil
}

func (r *fakeCategoryRepo) Get(_ context.Context, id uint64) (*TicketCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *c
	return &cp, nil
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

func (r *fakeCategoryRepo) List(_ context.Context) ([]TicketCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []TicketCategory
	for _, c := range r.items {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
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

func (r *fakeCategoryRepo) CountByCategory(_ context.Context, categoryID uint64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ticketCount[categoryID], nil
}

// ---------- UserDirectory ----------

type fakeUsers struct {
	mu    sync.Mutex
	items map[uint64]*platform.User
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{items: map[uint64]*platform.User{
		7:  {ID: 7, Username: "agent7", Role: "agent"},
		9:  {ID: 9, Username: "admin", Role: "admin"},
		20: {ID: 20, Username: "req", Role: "requestor"},
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

// ---------- SLAPolicyReader ----------

type fakeSLAs struct{ policies []platform.SLAPolicy }

func newFakeSLAs() *fakeSLAs {
	return &fakeSLAs{policies: []platform.SLAPolicy{
		{Priority: "P1", ResponseMinutes: 15, ResolveMinutes: 240, PauseOnPending: true},
		{Priority: "P2", ResponseMinutes: 30, ResolveMinutes: 480, PauseOnPending: true},
		{Priority: "P3", ResponseMinutes: 120, ResolveMinutes: 1440, PauseOnPending: true},
		{Priority: "P4", ResponseMinutes: 480, ResolveMinutes: 4320, PauseOnPending: true},
	}}
}

func (f *fakeSLAs) ListSLAPolicies(_ context.Context) ([]platform.SLAPolicy, error) {
	return f.policies, nil
}

// ---------- CommentStore ----------

type fakeCommentStore struct {
	mu       sync.Mutex
	seq      uint64
	comments []platform.Comment
	audits   []platform.AuditEntry
}

func newFakeCommentStore() *fakeCommentStore { return &fakeCommentStore{} }

func (f *fakeCommentStore) AppendAudit(_ context.Context, e platform.AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.audits = append(f.audits, e)
	return nil
}

func (f *fakeCommentStore) ListComments(_ context.Context, bizType string, bizID uint64, includeInternal bool) ([]platform.Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []platform.Comment
	for _, c := range f.comments {
		if c.BizType != bizType || c.BizID != bizID {
			continue
		}
		if c.IsInternal && !includeInternal {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeCommentStore) CreateComment(_ context.Context, op platform.Operator, req platform.CommentRequest) (*platform.Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	c := platform.Comment{ID: f.seq, BizType: req.BizType, BizID: req.BizID, AuthorID: op.ID, Content: req.Content, IsInternal: req.IsInternal}
	f.comments = append(f.comments, c)
	return &c, nil
}

func (f *fakeCommentStore) auditCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.audits)
}
