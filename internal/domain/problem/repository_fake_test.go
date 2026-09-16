package problem

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

// 本文件提供 problem 域各依赖的内存 fake。

// ---------- Repository ----------

type fakeRepo struct {
	mu    sync.Mutex
	seq   uint64
	items map[uint64]*Problem
	// failWrite 注入写错误（覆盖错误分支）。
	failWrite bool
	// failSeq 注入编号查询错误。
	failSeq bool
}

func newFakeRepo() *fakeRepo { return &fakeRepo{items: map[uint64]*Problem{}} }

func (r *fakeRepo) Create(_ context.Context, p *Problem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failWrite {
		return errors.New("db create failed")
	}
	r.seq++
	p.ID = r.seq
	cp := *p
	r.items[p.ID] = &cp
	return nil
}

func (r *fakeRepo) Update(_ context.Context, p *Problem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failWrite {
		return errors.New("db update failed")
	}
	if _, ok := r.items[p.ID]; !ok {
		return ErrNotFound
	}
	cp := *p
	r.items[p.ID] = &cp
	return nil
}

func (r *fakeRepo) Get(_ context.Context, id uint64) (*Problem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *fakeRepo) Delete(_ context.Context, id uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failWrite {
		return errors.New("db delete failed")
	}
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *fakeRepo) List(_ context.Context, q ListQuery) ([]Problem, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kw := strings.ToLower(strings.TrimSpace(q.Keyword))
	var all []Problem
	for _, p := range r.items {
		if q.Status != "" && p.Status != q.Status {
			continue
		}
		if q.AssigneeID != nil && (p.AssigneeID == nil || *p.AssigneeID != *q.AssigneeID) {
			continue
		}
		if q.KnownError != nil && ((*q.KnownError) != (p.Status == StatusKnownError)) {
			continue
		}
		if kw != "" && !strings.Contains(strings.ToLower(p.Title), kw) && !strings.Contains(strings.ToLower(p.Code), kw) {
			continue
		}
		all = append(all, *p)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	total := int64(len(all))
	if q.Limit <= 0 {
		return all, total, nil
	}
	if q.Offset >= len(all) {
		return []Problem{}, total, nil
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
	if r.failSeq {
		return 0, errors.New("db seq failed")
	}
	max := 0
	for _, p := range r.items {
		if seq, ok := idgen.ParseSeq(p.Code, prefix, day); ok && seq > max {
			max = seq
		}
	}
	return max, nil
}

// ---------- ProblemChangeRepository ----------

type fakeChanges struct {
	mu    sync.Mutex
	seq   uint64
	items []ProblemChange
	fail  bool
}

func newFakeChanges() *fakeChanges { return &fakeChanges{} }

func (r *fakeChanges) Add(_ context.Context, problemID uint64, changeIDs []uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errors.New("db add failed")
	}
	for _, cid := range changeIDs {
		dup := false
		for _, it := range r.items {
			if it.ProblemID == problemID && it.ChangeID == cid {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		r.seq++
		r.items = append(r.items, ProblemChange{ID: r.seq, ProblemID: problemID, ChangeID: cid})
	}
	return nil
}

func (r *fakeChanges) Remove(_ context.Context, problemID, changeID uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errors.New("db remove failed")
	}
	out := r.items[:0]
	for _, it := range r.items {
		if !(it.ProblemID == problemID && it.ChangeID == changeID) {
			out = append(out, it)
		}
	}
	r.items = out
	return nil
}

func (r *fakeChanges) ListChangeIDs(_ context.Context, problemID uint64) ([]uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return nil, errors.New("db list failed")
	}
	var ids []uint64
	for _, it := range r.items {
		if it.ProblemID == problemID {
			ids = append(ids, it.ChangeID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

// ---------- IncidentReader ----------

type fakeIncidents struct {
	mu       sync.Mutex
	status   map[uint64]string // incidentID -> status
	problem  map[uint64]uint64 // incidentID -> problemID
	ciCount  map[uint64]int    // ciID -> 事件数
	kwCount  map[string]int    // keyword -> 事件数
	failRead bool
}

func newFakeIncidents() *fakeIncidents {
	return &fakeIncidents{status: map[uint64]string{}, problem: map[uint64]uint64{}, ciCount: map[uint64]int{}, kwCount: map[string]int{}}
}

// seed 登记一个事件（status 默认 reported）。
func (f *fakeIncidents) seed(id uint64, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[id] = status
}

// attach 将事件挂到问题。
func (f *fakeIncidents) attach(id, problemID uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.problem[id] = problemID
	if _, ok := f.status[id]; !ok {
		f.status[id] = StatusInvestigating
	}
}

func (f *fakeIncidents) GetIncidentStatus(_ context.Context, incidentID uint64) (string, uint64, bool, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failRead {
		return "", 0, false, false, errors.New("incident read failed")
	}
	st, ok := f.status[incidentID]
	if !ok {
		return "", 0, false, false, nil
	}
	if pid, has := f.problem[incidentID]; has {
		return st, pid, true, true, nil
	}
	return st, 0, false, true, nil
}

func (f *fakeIncidents) AttachIncidentsToProblem(_ context.Context, incidentIDs []uint64, problemID uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failRead {
		return errors.New("incident attach failed")
	}
	for _, id := range incidentIDs {
		f.problem[id] = problemID
	}
	return nil
}

func (f *fakeIncidents) ListIncidentIDsByProblem(_ context.Context, problemID uint64) ([]uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failRead {
		return nil, errors.New("incident list failed")
	}
	var ids []uint64
	for id, pid := range f.problem {
		if pid == problemID {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (f *fakeIncidents) CountIncidentsByCI(_ context.Context, ciID uint64, _ time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failRead {
		return 0, errors.New("incident count failed")
	}
	return f.ciCount[ciID], nil
}

func (f *fakeIncidents) CountIncidentsByKeyword(_ context.Context, keyword string, _ time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failRead {
		return 0, errors.New("incident keyword count failed")
	}
	return f.kwCount[keyword], nil
}

// ---------- ChangeReader ----------

type fakeChangeReader struct {
	closed map[uint64]bool
	fail   bool
}

func newFakeChangeReader() *fakeChangeReader { return &fakeChangeReader{closed: map[uint64]bool{}} }

func (f *fakeChangeReader) ClosedChangeCount(_ context.Context, changeIDs []uint64) (int, error) {
	if f.fail {
		return 0, errors.New("change read failed")
	}
	n := 0
	for _, id := range changeIDs {
		if f.closed[id] {
			n++
		}
	}
	return n, nil
}

// ---------- Auditor / UserDirectory ----------

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
	fail  bool
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{items: map[uint64]*platform.User{
		7: {ID: 7, Username: "pm", Role: "problem_manager"},
	}}
}

func (f *fakeUsers) GetUser(_ context.Context, id uint64) (*platform.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return nil, errors.New("user read failed")
	}
	u, ok := f.items[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *u
	return &cp, nil
}
