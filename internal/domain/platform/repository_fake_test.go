package platform

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
)

// 本文件提供 platform 各仓储的内存 fake，供 service / handler 单测零依赖运行。

var errFakeDuplicate = errors.New("fake: duplicate")

// ---------- UserRepository ----------

type fakeUserRepo struct {
	mu    sync.Mutex
	seq   uint64
	items map[uint64]*User
}

func newFakeUserRepo() *fakeUserRepo { return &fakeUserRepo{items: map[uint64]*User{}} }

func (r *fakeUserRepo) Create(_ context.Context, u *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.items {
		if e.Username == u.Username {
			return errFakeDuplicate
		}
	}
	r.seq++
	u.ID = r.seq
	cp := *u
	r.items[u.ID] = &cp
	return nil
}

func (r *fakeUserRepo) Update(_ context.Context, u *User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[u.ID]; !ok {
		return ErrNotFound
	}
	cp := *u
	r.items[u.ID] = &cp
	return nil
}

func (r *fakeUserRepo) GetByID(_ context.Context, id uint64) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (r *fakeUserRepo) GetByUsername(_ context.Context, username string) (*User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.items {
		if u.Username == username {
			cp := *u
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeUserRepo) Delete(_ context.Context, id uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *fakeUserRepo) List(_ context.Context, q UserListQuery) ([]User, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kw := strings.ToLower(strings.TrimSpace(q.Keyword))
	var all []User
	for _, u := range r.items {
		if q.Role != "" && u.Role != q.Role {
			continue
		}
		if q.Status != "" && u.Status != q.Status {
			continue
		}
		if kw != "" && !strings.Contains(strings.ToLower(u.Username), kw) && !strings.Contains(strings.ToLower(u.DisplayName), kw) {
			continue
		}
		all = append(all, *u)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	total := int64(len(all))
	if q.Offset >= len(all) {
		return []User{}, total, nil
	}
	end := len(all)
	if q.Limit > 0 && q.Offset+q.Limit < end {
		end = q.Offset + q.Limit
	}
	return all[q.Offset:end], total, nil
}

// ListOptions 与 gorm 实现保持一致：仅 active，role 可选，只输出三字段。
func (r *fakeUserRepo) ListOptions(_ context.Context, role string) ([]UserOption, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var opts []UserOption
	for _, u := range r.items {
		if u.Status != UserStatusActive {
			continue
		}
		if role != "" && u.Role != role {
			continue
		}
		opts = append(opts, UserOption{ID: u.ID, DisplayName: u.DisplayName, Role: u.Role})
	}
	sort.Slice(opts, func(i, j int) bool { return opts[i].ID < opts[j].ID })
	return opts, nil
}

// ---------- SLAPolicyRepository ----------

type fakeSLARepo struct {
	mu    sync.Mutex
	seq   uint64
	items map[uint64]*SLAPolicy
}

func newFakeSLARepo() *fakeSLARepo { return &fakeSLARepo{items: map[uint64]*SLAPolicy{}} }

func (r *fakeSLARepo) Create(_ context.Context, p *SLAPolicy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.items {
		if e.Priority == p.Priority {
			return errFakeDuplicate
		}
	}
	r.seq++
	p.ID = r.seq
	cp := *p
	r.items[p.ID] = &cp
	return nil
}

func (r *fakeSLARepo) Update(_ context.Context, p *SLAPolicy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[p.ID]; !ok {
		return ErrNotFound
	}
	cp := *p
	r.items[p.ID] = &cp
	return nil
}

func (r *fakeSLARepo) GetByID(_ context.Context, id uint64) (*SLAPolicy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *fakeSLARepo) GetByPriority(_ context.Context, priority string) (*SLAPolicy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.items {
		if p.Priority == priority {
			cp := *p
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (r *fakeSLARepo) List(_ context.Context) ([]SLAPolicy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]SLAPolicy, 0, len(r.items))
	for _, p := range r.items {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Priority < out[j].Priority })
	return out, nil
}

func (r *fakeSLARepo) Delete(_ context.Context, id uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

// ---------- AuditRepository ----------

type fakeAuditRepo struct {
	mu    sync.Mutex
	seq   uint64
	items []AuditLog
}

func newFakeAuditRepo() *fakeAuditRepo { return &fakeAuditRepo{} }

func (r *fakeAuditRepo) Append(_ context.Context, entry *AuditLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	entry.ID = r.seq
	r.items = append(r.items, *entry)
	return nil
}

func (r *fakeAuditRepo) List(_ context.Context, q AuditListQuery) ([]AuditLog, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var all []AuditLog
	bizType, bizID := q.auditBizFilter()
	for _, e := range r.items {
		if q.ActorID != 0 && e.ActorID != q.ActorID {
			continue
		}
		if bizType != "" && e.BizType != bizType {
			continue
		}
		if bizID != 0 && e.BizID != bizID {
			continue
		}
		if q.Action != "" && e.Action != q.Action {
			continue
		}
		all = append(all, e)
	}
	total := int64(len(all))
	if q.Offset >= len(all) {
		return []AuditLog{}, total, nil
	}
	end := len(all)
	if q.Limit > 0 && q.Offset+q.Limit < end {
		end = q.Offset + q.Limit
	}
	return all[q.Offset:end], total, nil
}

// ---------- CommentRepository ----------

type fakeCommentRepo struct {
	mu    sync.Mutex
	seq   uint64
	items []Comment
}

func newFakeCommentRepo() *fakeCommentRepo { return &fakeCommentRepo{} }

func (r *fakeCommentRepo) Create(_ context.Context, c *Comment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	c.ID = r.seq
	r.items = append(r.items, *c)
	return nil
}

func (r *fakeCommentRepo) ListByBiz(_ context.Context, bizType string, bizID uint64, includeInternal bool) ([]Comment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Comment
	for _, c := range r.items {
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

// ---------- AttachmentRepository ----------

type fakeAttachmentRepo struct {
	mu    sync.Mutex
	seq   uint64
	items map[uint64]*Attachment
}

func newFakeAttachmentRepo() *fakeAttachmentRepo {
	return &fakeAttachmentRepo{items: map[uint64]*Attachment{}}
}

func (r *fakeAttachmentRepo) Create(_ context.Context, a *Attachment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	a.ID = r.seq
	cp := *a
	r.items[a.ID] = &cp
	return nil
}

func (r *fakeAttachmentRepo) GetByID(_ context.Context, id uint64) (*Attachment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *a
	return &cp, nil
}

func (r *fakeAttachmentRepo) ListByBiz(_ context.Context, bizType string, bizID uint64) ([]Attachment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Attachment
	for _, a := range r.items {
		if a.BizType == bizType && a.BizID == bizID {
			out = append(out, *a)
		}
	}
	return out, nil
}

func (r *fakeAttachmentRepo) Delete(_ context.Context, id uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}
