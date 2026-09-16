// 本文件承载 incident 域业务规则：上报/矩阵/覆盖/升级/转单/解决/复盘/看板。
package incident

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/domain/platform"
	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/idgen"
	"github.com/chixiaowen/itsm-core/internal/pkg/sla"
	"github.com/chixiaowen/itsm-core/internal/pkg/statemachine"
)

// Actor 是执行操作的当前用户。
type Actor struct {
	UserID   uint64
	Role     string
	ClientIP string
}

// Deps 是 incident service 的依赖集合。
type Deps struct {
	Repo        Repository
	Escalations EscalationRepository
	Users       UserDirectory
	SLAs        SLAPolicyReader
	Auditor     Auditor
	Tickets     TicketCreator
	IDGen       *idgen.Generator
	Logger      *zap.Logger
	Now         func() time.Time
}

// Service 承载事件业务。
type Service struct {
	repo        Repository
	escalations EscalationRepository
	users       UserDirectory
	slas        SLAPolicyReader
	auditor     Auditor
	tickets     TicketCreator
	gen         *idgen.Generator
	log         *zap.Logger
	now         func() time.Time
}

// NewService 构造 incident service。
func NewService(d Deps) *Service {
	s := &Service{
		repo: d.Repo, escalations: d.Escalations, users: d.Users, slas: d.SLAs,
		auditor: d.Auditor, tickets: d.Tickets, gen: d.IDGen, log: d.Logger, now: d.Now,
	}
	if s.gen == nil {
		s.gen = idgen.NewGenerator()
	}
	if s.log == nil {
		s.log = zap.NewNop()
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

// ---------------- 上报 ----------------

// Report 上报事件：矩阵计算优先级 + SLA 截止时间 + 初始状态 reported。
func (s *Service) Report(ctx context.Context, actor Actor, req ReportRequest) (*Incident, error) {
	if s.repo == nil {
		return nil, httpx.ErrInternal("事件仓储未初始化")
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, httpx.ErrBadRequest("标题必填")
	}
	if !ValidImpact(req.Impact) {
		return nil, httpx.ErrBadRequest("影响度必填且合法（high/medium/low）")
	}
	if !ValidUrgency(req.Urgency) {
		return nil, httpx.ErrBadRequest("紧急度必填且合法（high/medium/low）")
	}
	priority, err := PriorityFromImpactUrgency(req.Impact, req.Urgency)
	if err != nil {
		return nil, err
	}
	reporter := actor.UserID
	if req.ReporterID != nil && *req.ReporterID != 0 {
		reporter = *req.ReporterID
	}
	now := s.now().UTC()
	policy := s.policyFor(ctx, priority)
	code, err := s.nextCode(ctx, now)
	if err != nil {
		return nil, err
	}
	respDue := sla.Due(now, policy.ResponseMinutes)
	resolveDue := sla.Due(now, policy.ResolveMinutes)
	it := &Incident{
		Code: code, Title: req.Title, Description: req.Description, Status: StatusReported,
		Impact: req.Impact, Urgency: req.Urgency, Priority: priority,
		ReporterID: reporter, OccurredAt: req.OccurredAt,
		ResponseDueAt: &respDue, ResolveDueAt: &resolveDue, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.Create(ctx, it); err != nil {
		return nil, httpx.ErrInternal("上报事件失败: " + err.Error())
	}
	if len(req.CIIDs) > 0 {
		_ = s.repo.AddCIs(ctx, it.ID, req.CIIDs)
	}
	s.audit(ctx, actor, "report", it.ID, "", it.Status, nil, it)
	return s.decorate(ctx, it), nil
}

// ---------------- 查询 ----------------

// Get 查询事件详情。
func (s *Service) Get(ctx context.Context, actor Actor, id uint64) (*Incident, error) {
	it, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.decorate(ctx, it), nil
}

// Detail 事件详情 + 升级历史 + CI。
func (s *Service) Detail(ctx context.Context, actor Actor, id uint64) (*Detail, error) {
	it, err := s.Get(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	d := &Detail{Incident: it, Escalations: []IncidentEscalation{}, CIs: []uint64{}}
	if s.escalations != nil {
		es, err := s.escalations.ListByIncident(ctx, id)
		if err != nil {
			return nil, httpx.ErrInternal("查询升级历史失败: " + err.Error())
		}
		if es != nil {
			d.Escalations = es
		}
	}
	if s.repo != nil {
		cis, err := s.repo.ListCIs(ctx, id)
		if err != nil {
			return nil, httpx.ErrInternal("查询关联 CI 失败: " + err.Error())
		}
		if cis != nil {
			d.CIs = cis
		}
	}
	return d, nil
}

// List 分页查询事件。
func (s *Service) List(ctx context.Context, actor Actor, q ListQuery) ([]Incident, int64, error) {
	if s.repo == nil {
		return nil, 0, httpx.ErrInternal("事件仓储未初始化")
	}
	q.SortBy, q.Order = normalizeSort(q.SortBy, q.Order)
	items, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, 0, httpx.ErrInternal("查询事件失败: " + err.Error())
	}
	for i := range items {
		s.decorate(ctx, &items[i])
	}
	return items, total, nil
}

// Stats 看板统计（按状态/优先级）。
func (s *Service) Stats(ctx context.Context) (map[string]int64, map[string]int64, error) {
	if s.repo == nil {
		return nil, nil, httpx.ErrInternal("事件仓储未初始化")
	}
	byStatus, err := s.repo.CountByStatus(ctx)
	if err != nil {
		return nil, nil, httpx.ErrInternal("统计失败: " + err.Error())
	}
	byPriority, err := s.repo.CountByPriority(ctx)
	if err != nil {
		return nil, nil, httpx.ErrInternal("统计失败: " + err.Error())
	}
	return byStatus, byPriority, nil
}

// ---------------- 编辑 ----------------

// Update 编辑事件（agent/admin，重算优先级若影响度/紧急度变化）。
func (s *Service) Update(ctx context.Context, actor Actor, id uint64, req UpdateRequest) (*Incident, error) {
	it, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	before := *it
	recalc := false
	if req.Title != nil {
		if strings.TrimSpace(*req.Title) == "" {
			return nil, httpx.ErrBadRequest("标题不得为空")
		}
		it.Title = *req.Title
	}
	if req.Description != nil {
		it.Description = *req.Description
	}
	if req.Impact != nil {
		if !ValidImpact(*req.Impact) {
			return nil, httpx.ErrBadRequest("非法影响度")
		}
		it.Impact = *req.Impact
		recalc = true
	}
	if req.Urgency != nil {
		if !ValidUrgency(*req.Urgency) {
			return nil, httpx.ErrBadRequest("非法紧急度")
		}
		it.Urgency = *req.Urgency
		recalc = true
	}
	// 人工覆盖优先级后不再自动重算，保留覆盖结果。
	if recalc && !it.PriorityOverridden {
		if p, err := PriorityFromImpactUrgency(it.Impact, it.Urgency); err == nil {
			it.Priority = p
			policy := s.policyFor(ctx, p)
			rd := sla.Due(it.CreatedAt, policy.ResponseMinutes)
			vd := sla.Due(it.CreatedAt, policy.ResolveMinutes)
			it.ResponseDueAt = &rd
			it.ResolveDueAt = &vd
		}
	}
	it.UpdatedAt = s.now().UTC()
	if err := s.repo.Update(ctx, it); err != nil {
		return nil, httpx.ErrInternal("更新事件失败: " + err.Error())
	}
	s.audit(ctx, actor, "update", it.ID, "", "", before, it)
	return s.decorate(ctx, it), nil
}

// Delete 软删除事件（admin）。
func (s *Service) Delete(ctx context.Context, actor Actor, id uint64) error {
	if s.repo == nil {
		return httpx.ErrInternal("事件仓储未初始化")
	}
	it, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return httpx.ErrInternal("删除事件失败: " + err.Error())
	}
	s.audit(ctx, actor, "delete", it.ID, it.Status, "", it, nil)
	return nil
}

// ---------------- 优先级覆盖 ----------------

// OverridePriority 人工覆盖优先级（写 priority_overridden=true + 审计）。
func (s *Service) OverridePriority(ctx context.Context, actor Actor, id uint64, priority string) (*Incident, error) {
	it, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if !validPriority(priority) {
		return nil, httpx.ErrBadRequest("非法优先级: " + priority)
	}
	before := *it
	it.Priority = priority
	it.PriorityOverridden = true
	it.UpdatedAt = s.now().UTC()
	if err := s.repo.Update(ctx, it); err != nil {
		return nil, httpx.ErrInternal("覆盖优先级失败: " + err.Error())
	}
	s.audit(ctx, actor, "override_priority", it.ID, "", "", before, map[string]any{"priority": priority})
	return s.decorate(ctx, it), nil
}

// ---------------- 状态流转 ----------------

// Transition 统一状态流转（升级动作会额外写升级历史）。
func (s *Service) Transition(ctx context.Context, actor Actor, id uint64, action string, req TransitionRequest) (*Incident, error) {
	it, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	tr, ok := incidentMachine.Resolve(it.Status, action)
	if !ok {
		// PRD §5 / docs/API.md：非法流转 409 消息含「当前状态 -> 目标状态」对。
		return nil, httpx.ErrConflict(illegalTransitionMsg(it.Status, action, "当前状态不允许该动作"))
	}
	if !tr.Allows(actor.Role) {
		return nil, httpx.ErrForbidden("当前角色无权执行该流转")
	}
	params := s.buildParams(action, req)
	now := s.now().UTC()
	if tr.Guard != nil {
		if err := tr.Guard(statemachine.GuardInput{
			Ctx: ctx, Actor: statemachine.Actor{UserID: actor.UserID, Role: actor.Role},
			Entity: it, Params: params, Now: now,
		}); err != nil {
			return nil, err
		}
	}
	if action == ActionConfirm {
		if err := s.validateAssignee(ctx, params); err != nil {
			return nil, err
		}
	}
	from := it.Status
	if action == ActionEscalate {
		if err := s.recordEscalation(ctx, actor, it, params); err != nil {
			return nil, err
		}
	}
	s.applyTransition(it, action, tr.To, params, now)
	it.UpdatedAt = now
	if err := s.repo.Update(ctx, it); err != nil {
		return nil, httpx.ErrInternal("流转事件失败: " + err.Error())
	}
	s.audit(ctx, actor, "transition:"+action, it.ID, from, it.Status, nil, map[string]any{"action": action, "status": it.Status})
	return s.decorate(ctx, it), nil
}

// Escalate 便捷升级（等价 transition escalate）。
func (s *Service) Escalate(ctx context.Context, actor Actor, id uint64, req EscalateRequest) (*Incident, error) {
	return s.Transition(ctx, actor, id, ActionEscalate, TransitionRequest{
		Action: ActionEscalate, Type: req.Type, Reason: req.Reason,
		ToAssigneeID: req.ToAssigneeID, Level: req.Level,
	})
}

// recordEscalation 写入升级历史并提升级别。
func (s *Service) recordEscalation(ctx context.Context, actor Actor, it *Incident, params map[string]any) error {
	target := it.EscalationLevel + 1
	et := EscFunc
	if v, ok := params["escalation_type"].(string); ok && v != "" {
		et = v
	}
	var to *uint64
	if v, ok := params["assignee_id"].(uint64); ok && v != 0 {
		to = &v
	}
	rec := &IncidentEscalation{
		IncidentID: it.ID, Level: target, Type: et,
		Reason:       reasonOf(statemachine.GuardInput{Params: params}),
		FromAssignee: it.AssigneeID, ToAssignee: to,
		ActorID: actor.UserID, CreatedAt: s.now().UTC(),
	}
	if s.escalations != nil {
		if err := s.escalations.Create(ctx, rec); err != nil {
			return httpx.ErrInternal("写入升级历史失败: " + err.Error())
		}
	}
	it.EscalationLevel = target
	if to != nil {
		it.AssigneeID = to
	}
	s.audit(ctx, actor, "escalate", it.ID, "", "", nil, map[string]any{"level": target, "type": et})
	return nil
}

// ---------------- 转工单 ----------------

// ConvertToTicket 一键转工单（写双向引用；重复转单 409）。
func (s *Service) ConvertToTicket(ctx context.Context, actor Actor, id uint64, title string) (uint64, string, error) {
	it, err := s.load(ctx, id)
	if err != nil {
		return 0, "", err
	}
	if it.Status != StatusInProgress && it.Status != StatusTriage {
		return 0, "", httpx.ErrConflict("仅 triage/in_progress 状态的事件可转工单")
	}
	if it.TicketID != nil && *it.TicketID != 0 {
		return 0, "", httpx.ErrConflict("该事件已转工单，不可重复转单")
	}
	if s.tickets == nil {
		return 0, "", httpx.ErrInternal("工单创建组件未初始化")
	}
	if strings.TrimSpace(title) == "" {
		title = it.Title
	}
	tkID, code, err := s.tickets.CreateFromIncident(ctx, title, it.Description, it.ReporterID, it.ID, it.Priority)
	if err != nil {
		return 0, "", err
	}
	it.TicketID = &tkID
	it.UpdatedAt = s.now().UTC()
	if err := s.repo.Update(ctx, it); err != nil {
		return 0, "", httpx.ErrInternal("回写事件工单号失败: " + err.Error())
	}
	s.audit(ctx, actor, "convert_to_ticket", it.ID, "", "", nil, map[string]any{"ticket_id": tkID})
	return tkID, code, nil
}

// LinkTicket 关联已有工单。
func (s *Service) LinkTicket(ctx context.Context, actor Actor, id, ticketID uint64) (*Incident, error) {
	if ticketID == 0 {
		return nil, httpx.ErrBadRequest("ticket_id 必填")
	}
	it, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	it.TicketID = &ticketID
	it.UpdatedAt = s.now().UTC()
	if err := s.repo.Update(ctx, it); err != nil {
		return nil, httpx.ErrInternal("关联工单失败: " + err.Error())
	}
	s.audit(ctx, actor, "link_ticket", it.ID, "", "", nil, map[string]any{"ticket_id": ticketID})
	return s.decorate(ctx, it), nil
}

// ---------------- 关联 CI ----------------

// AddCIs 关联 CI。
func (s *Service) AddCIs(ctx context.Context, actor Actor, id uint64, ciIDs []uint64) error {
	if s.repo == nil {
		return httpx.ErrInternal("事件仓储未初始化")
	}
	if _, err := s.load(ctx, id); err != nil {
		return err
	}
	if len(ciIDs) == 0 {
		return httpx.ErrBadRequest("ci_ids 不得为空")
	}
	if err := s.repo.AddCIs(ctx, id, ciIDs); err != nil {
		return httpx.ErrInternal("关联 CI 失败: " + err.Error())
	}
	s.audit(ctx, actor, "attach_cis", id, "", "", nil, map[string]any{"ci_ids": ciIDs})
	return nil
}

// RemoveCI 解除 CI 关联。
func (s *Service) RemoveCI(ctx context.Context, actor Actor, id, ciID uint64) error {
	if s.repo == nil {
		return httpx.ErrInternal("事件仓储未初始化")
	}
	if _, err := s.load(ctx, id); err != nil {
		return err
	}
	if err := s.repo.RemoveCI(ctx, id, ciID); err != nil {
		return httpx.ErrInternal("解除 CI 失败: " + err.Error())
	}
	s.audit(ctx, actor, "detach_ci", id, "", "", nil, map[string]any{"ci_id": ciID})
	return nil
}

// ---------------- 消费者侧实现（供 problem 域消费）----------------

// GetIncidentStatus 返回事件状态与已挂载的问题 id（供 problem 聚合校验）。
func (s *Service) GetIncidentStatus(ctx context.Context, incidentID uint64) (status string, problemID uint64, hasProblem bool, found bool, err error) {
	if s.repo == nil {
		return "", 0, false, false, httpx.ErrInternal("事件仓储未初始化")
	}
	it, e := s.repo.Get(ctx, incidentID)
	if e != nil {
		if errors.Is(e, ErrNotFound) {
			return "", 0, false, false, nil
		}
		return "", 0, false, false, e
	}
	if it.ProblemID != nil && *it.ProblemID != 0 {
		return it.Status, *it.ProblemID, true, true, nil
	}
	return it.Status, 0, false, true, nil
}

// AttachIncidentsToProblem 批量写入 incidents.problem_id（跨域写）。
func (s *Service) AttachIncidentsToProblem(ctx context.Context, incidentIDs []uint64, problemID uint64) error {
	if s.repo == nil {
		return httpx.ErrInternal("事件仓储未初始化")
	}
	return s.repo.AttachToProblem(ctx, incidentIDs, problemID)
}

// CountIncidentsByCI 统计某 CI 在 since 之后的事件数（供 problem 建议聚合）。
func (s *Service) CountIncidentsByCI(ctx context.Context, ciID uint64, since time.Time) (int, error) {
	if s.repo == nil {
		return 0, httpx.ErrInternal("事件仓储未初始化")
	}
	return s.repo.CountByCISince(ctx, ciID, since)
}

// CountIncidentsByKeyword 统计标题/描述含关键词且在此之后的事件数（供 problem 建议聚合）。
func (s *Service) CountIncidentsByKeyword(ctx context.Context, keyword string, since time.Time) (int, error) {
	if s.repo == nil {
		return 0, httpx.ErrInternal("事件仓储未初始化")
	}
	return s.repo.CountByKeywordSince(ctx, keyword, since)
}

// ListIncidentIDsByProblem 返回挂载到某问题的事件 id 列表（供 problem 详情展示）。
func (s *Service) ListIncidentIDsByProblem(ctx context.Context, problemID uint64) ([]uint64, error) {
	if s.repo == nil {
		return nil, httpx.ErrInternal("事件仓储未初始化")
	}
	return s.repo.ListIDsByProblem(ctx, problemID)
}

// ---------------- 内部 helpers ----------------

func (s *Service) load(ctx context.Context, id uint64) (*Incident, error) {
	if s.repo == nil {
		return nil, httpx.ErrInternal("事件仓储未初始化")
	}
	it, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "事件不存在")
	}
	return it, nil
}

func (s *Service) buildParams(action string, req TransitionRequest) map[string]any {
	p := map[string]any{
		"action":            action,
		"solution":          strings.TrimSpace(req.Solution),
		"reason":            strings.TrimSpace(req.Reason),
		"review_conclusion": strings.TrimSpace(req.ReviewConclusion),
		"escalation_type":   strings.TrimSpace(req.Type),
	}
	if req.AssigneeID != nil {
		p["assignee_id"] = *req.AssigneeID
	}
	if req.ToAssigneeID != nil {
		p["assignee_id"] = *req.ToAssigneeID
	}
	if req.Level != nil {
		p["level"] = *req.Level
	}
	return p
}

func (s *Service) validateAssignee(ctx context.Context, params map[string]any) error {
	id, _ := params["assignee_id"].(uint64)
	if id == 0 {
		return httpx.ErrPrecondition("指派必须指定有效的 assignee_id")
	}
	if s.users == nil {
		return nil
	}
	if _, err := s.users.GetUser(ctx, id); err != nil {
		return httpx.ErrPrecondition("指派人不存在或不可用")
	}
	return nil
}

func (s *Service) applyTransition(it *Incident, action, to string, params map[string]any, now time.Time) {
	switch action {
	case ActionConfirm:
		if id, ok := params["assignee_id"].(uint64); ok && id != 0 {
			a := id
			it.AssigneeID = &a
		}
	case ActionTakeOver:
		if id, ok := params["assignee_id"].(uint64); ok && id != 0 {
			a := id
			it.AssigneeID = &a
		}
	case ActionFalsePos, ActionResolve:
		if sol, ok := params["solution"].(string); ok {
			it.Solution = sol
		}
		if it.ResolvedAt == nil {
			it.ResolvedAt = &now
		}
	case ActionClose:
		if v, ok := params["review_conclusion"].(string); ok && v != "" {
			it.ReviewConclusion = v
		}
		if it.ClosedAt == nil {
			it.ClosedAt = &now
		}
	case ActionRevert:
		if it.ResolvedAt != nil {
			it.ResolvedAt = nil // 回退后清空解决时间，允许再次解决
		}
	}
	it.Status = to
}

func (s *Service) decorate(ctx context.Context, it *Incident) *Incident {
	policy := s.policyFor(ctx, it.Priority)
	view := sla.Evaluate(policy, it.CreatedAt, s.now().UTC(), nil, it.ResolvedAt, 0)
	it.SLAStatus = string(view.SLAStatus)
	return it
}

func (s *Service) policyFor(ctx context.Context, priority string) sla.Policy {
	if s.slas != nil {
		if list, err := s.slas.ListSLAPolicies(ctx); err == nil {
			for _, p := range list {
				if p.Priority == priority {
					return sla.Policy{Priority: p.Priority, ResponseMinutes: p.ResponseMinutes, ResolveMinutes: p.ResolveMinutes, PauseOnPending: p.PauseOnPending}
				}
			}
		}
	}
	for _, p := range sla.DefaultPolicies() {
		if p.Priority == priority {
			return p
		}
	}
	return sla.Policy{Priority: priority, ResponseMinutes: 480, ResolveMinutes: 4320, PauseOnPending: true}
}

func (s *Service) nextCode(ctx context.Context, now time.Time) (string, error) {
	max := 0
	if s.repo != nil {
		m, err := s.repo.MaxDailySeq(ctx, idgen.PrefixIncident, now)
		if err != nil {
			return "", httpx.ErrInternal("生成事件编号失败: " + err.Error())
		}
		max = m
	}
	return s.gen.Next(idgen.PrefixIncident, now, max), nil
}

func (s *Service) audit(ctx context.Context, actor Actor, action string, bizID uint64, from, to string, before, after any) {
	if s.auditor == nil {
		return
	}
	e := platform.AuditEntry{
		ActorID: actor.UserID, Action: action, BizType: platform.BizTypeIncident, BizID: bizID,
		FromStatus: from, ToStatus: to, BeforeValue: toJSON(before), AfterValue: toJSON(after), ClientIP: actor.ClientIP,
	}
	if err := s.auditor.AppendAudit(ctx, e); err != nil {
		s.log.Warn("写入事件审计失败", zap.Uint64("incident_id", bizID), zap.Error(err))
	}
}

func toJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func normalizeSort(sortBy, order string) (string, string) {
	allowed := map[string]bool{"created_at": true, "priority": true, "status": true, "id": true, "escalation_level": true}
	if !allowed[sortBy] {
		sortBy = "created_at"
	}
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	return sortBy, order
}

func mapRepoErr(err error, notFoundMsg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return httpx.ErrNotFound(notFoundMsg)
	}
	return httpx.ErrInternal(err.Error())
}

// canView 资源级鉴权（事件对全体登录用户可见；预留给未来细分）。
func canView(_ Actor, _ *Incident) bool { return true }
