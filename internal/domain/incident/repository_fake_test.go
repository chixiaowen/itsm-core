package incident

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

// 本文件提供 incident 域各依赖的内存 fake。

// ---------- Repository ----------

type fakeRepo struct {
	mu     sync.Mutex
	seq    uint64
	items  map[uint64]*Incident
	cis    map[uint64][]uint64
	nowRef *time.Time
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[uint64]*Incident{}, cis: map[uint64][]uint64{}}
}

func (r *fakeRepo) Create(_ context.Context, i *Incident) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	i.ID = r.seq
	cp := *i
	r.items[i.ID] = &cp
	return nil
}

func (r *fakeRepo) Update(_ context.Context, i *Incident) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[i.ID]; !ok {
		return ErrNotFound
	}
	cp := *i
	r.items[i.ID] = &cp
	return nil
}

func (r *fakeRepo) Get(_ context.Context, id uint64) (*Incident, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	i, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *i
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

func (r *fakeRepo) List(_ context.Context, q ListQuery) ([]Incident, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kw := strings.ToLower(strings.TrimSpace(q.Keyword))
	var all []Incident
	for _, i := range r.items {
		if q.Status != "" && i.Status != q.Status {
			continue
		}
		if q.Priority != "" && i.Priority != q.Priority {
			continue
		}
		if q.Impact != "" && i.Impact != q.Impact {
			continue
		}
		if q.Urgency != "" && i.Urgency != q.Urgency {
			continue
		}
		if q.EscalationLevel != nil && i.EscalationLevel != *q.EscalationLevel {
			continue
		}
		if q.From != nil && i.CreatedAt.Before(*q.From) {
			continue
		}
		if q.To != nil && i.CreatedAt.After(*q.To) {
			continue
		}
		if kw != "" && !strings.Contains(strings.ToLower(i.Title), kw) && !strings.Contains(strings.ToLower(i.Code), kw) {
			continue
		}
		all = append(all, *i)
	}
	sort.Slice(all, func(a, b int) bool { return all[a].ID > all[b].ID })
	total := int64(len(all))
	if q.Limit <= 0 {
		return all, total, nil
	}
	if q.Offset >= len(all) {
		return []Incident{}, total, nil
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
	for _, i := range r.items {
		if seq, ok := idgen.ParseSeq(i.Code, prefix, day); ok && seq > max {
			max = seq
		}
	}
	return max, nil
}

func (r *fakeRepo) ListCIs(_ context.Context, incidentID uint64) ([]uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]uint64{}, r.cis[incidentID]...), nil
}

func (r *fakeRepo) AddCIs(_ context.Context, incidentID uint64, ciIDs []uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ci := range ciIDs {
		exists := false
		for _, e := range r.cis[incidentID] {
			if e == ci {
				exists = true
				break
			}
		}
		if !exists {
			r.cis[incidentID] = append(r.cis[incidentID], ci)
		}
	}
	return nil
}

func (r *fakeRepo) RemoveCI(_ context.Context, incidentID, ciID uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.cis[incidentID][:0]
	for _, e := range r.cis[incidentID] {
		if e != ciID {
			out = append(out, e)
		}
	}
	r.cis[incidentID] = out
	return nil
}

func (r *fakeRepo) AttachToProblem(_ context.Context, incidentIDs []uint64, problemID uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range incidentIDs {
		if it, ok := r.items[id]; ok {
			pid := problemID
			it.ProblemID = &pid
		}
	}
	return nil
}

func (r *fakeRepo) ListIDsByProblem(_ context.Context, problemID uint64) ([]uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var ids []uint64
	for id, it := range r.items {
		if it.ProblemID != nil && *it.ProblemID == problemID {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (r *fakeRepo) CountByCISince(_ context.Context, ciID uint64, since time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for id, cis := range r.cis {
		has := false
		for _, c := range cis {
			if c == ciID {
				has = true
				break
			}
		}
		if has {
			if it, ok := r.items[id]; ok && !it.CreatedAt.Before(since) {
				n++
			}
		}
	}
	return n, nil
}

func (r *fakeRepo) CountByKeywordSince(_ context.Context, keyword string, since time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kw := strings.ToLower(keyword)
	n := 0
	for _, it := range r.items {
		if it.CreatedAt.Before(since) {
			continue
		}
		if strings.Contains(strings.ToLower(it.Title), kw) || strings.Contains(strings.ToLower(it.Description), kw) {
			n++
		}
	}
	return n, nil
}

func (r *fakeRepo) CountByStatus(_ context.Context) (map[string]int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]int64{}
	for _, it := range r.items {
		out[it.Status]++
	}
	return out, nil
}

func (r *fakeRepo) CountByPriority(_ context.Context) (map[string]int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]int64{}
	for _, it := range r.items {
		out[it.Priority]++
	}
	return out, nil
}

// ---------- EscalationRepository ----------

type fakeEscalations struct {
	mu    sync.Mutex
	seq   uint64
	items []IncidentEscalation
}

func newFakeEscalations() *fakeEscalations { return &fakeEscalations{} }

func (r *fakeEscalations) Create(_ context.Context, e *IncidentEscalation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	e.ID = r.seq
	r.items = append(r.items, *e)
	return nil
}

func (r *fakeEscalations) ListByIncident(_ context.Context, incidentID uint64) ([]IncidentEscalation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []IncidentEscalation
	for _, e := range r.items {
		if e.IncidentID == incidentID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (r *fakeEscalations) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.items)
}

// ---------- 其它消费者 ----------

type fakeUsers struct {
	mu    sync.Mutex
	items map[uint64]*platform.User
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{items: map[uint64]*platform.User{
		7:  {ID: 7, Username: "agent7", Role: "agent"},
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

func (f *fakeAuditor) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.audits)
}

// fakeTicketCreator 实现 TicketCreator。
type fakeTicketCreator struct {
	mu       sync.Mutex
	seq      uint64
	calls    int
	failWith error
}

func newFakeTicketCreator() *fakeTicketCreator { return &fakeTicketCreator{} }

func (f *fakeTicketCreator) CreateFromIncident(_ context.Context, _ string, _ string, _ uint64, _ uint64, _ string) (uint64, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return 0, "", f.failWith
	}
	f.calls++
	f.seq++
	return f.seq, "TKT-20260916-0001", nil
}
