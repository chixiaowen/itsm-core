// 本文件承载 problem 域业务规则：聚合创建/RCA/已知错误/关联变更/解决约束/建议聚合。
package problem

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
	"github.com/chixiaowen/itsm-core/internal/pkg/statemachine"
)

// Actor 是执行操作的当前用户。
type Actor struct {
	UserID   uint64
	Role     string
	ClientIP string
}

// Deps 是 problem service 的依赖集合。
type Deps struct {
	Repo       Repository
	Changes    ProblemChangeRepository
	Incidents  IncidentReader
	ChangeRead ChangeReader
	Users      UserDirectory
	Auditor    Auditor
	IDGen      *idgen.Generator
	Logger     *zap.Logger
	Now        func() time.Time
}

// Service 承载问题业务。
type Service struct {
	repo       Repository
	changes    ProblemChangeRepository
	incidents  IncidentReader
	changeRead ChangeReader
	users      UserDirectory
	auditor    Auditor
	gen        *idgen.Generator
	log        *zap.Logger
	now        func() time.Time
}

// NewService 构造 problem service。
func NewService(d Deps) *Service {
	s := &Service{
		repo: d.Repo, changes: d.Changes, incidents: d.Incidents, changeRead: d.ChangeRead,
		users: d.Users, auditor: d.Auditor, gen: d.IDGen, log: d.Logger, now: d.Now,
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

// ---------------- 创建（聚合 / 手动）----------------

// Create 创建问题：source=aggregate 时选择 ≥1 个事件聚合。
func (s *Service) Create(ctx context.Context, actor Actor, req CreateRequest) (*Problem, error) {
	if s.repo == nil {
		return nil, httpx.ErrInternal("问题仓储未初始化")
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, httpx.ErrBadRequest("标题必填")
	}
	source := req.Source
	if source == "" {
		source = SourceManual
	}
	if source != SourceAggregate && source != SourceManual {
		return nil, httpx.ErrBadRequest("非法来源（aggregate/manual）")
	}
	if source == SourceAggregate && len(req.IncidentIDs) == 0 {
		return nil, httpx.ErrBadRequest("聚合创建须选择至少 1 个事件")
	}
	// 校验事件可聚合（存在且未挂到其它未关闭问题）。
	if source == SourceAggregate {
		if s.incidents == nil {
			return nil, httpx.ErrInternal("事件读取组件未初始化")
		}
		for _, iid := range req.IncidentIDs {
			_, pid, has, found, err := s.incidents.GetIncidentStatus(ctx, iid)
			if err != nil {
				return nil, httpx.ErrInternal("查询事件失败: " + err.Error())
			}
			if !found {
				return nil, httpx.ErrBadRequest("事件不存在: " + itoa(iid))
			}
			if has {
				existing, err := s.repo.Get(ctx, pid)
				if err == nil && existing.Status != StatusClosed && existing.Status != StatusCancelled {
					return nil, httpx.ErrConflict("事件已被其它未关闭问题聚合")
				}
			}
		}
	}

	now := s.now().UTC()
	code, err := s.nextCode(ctx, now)
	if err != nil {
		return nil, err
	}
	p := &Problem{
		Code: code, Title: req.Title, Description: req.Description, Status: StatusNew,
		Source: source, CreatorID: actor.UserID, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, httpx.ErrInternal("创建问题失败: " + err.Error())
	}
	if source == SourceAggregate {
		if err := s.incidents.AttachIncidentsToProblem(ctx, req.IncidentIDs, p.ID); err != nil {
			return nil, httpx.ErrInternal("聚合事件失败: " + err.Error())
		}
	}
	s.audit(ctx, actor, "create", p.ID, "", p.Status, nil, p)
	return p, nil
}

// ---------------- 查询 ----------------

// Get 查询问题。
func (s *Service) Get(ctx context.Context, actor Actor, id uint64) (*Problem, error) {
	return s.load(ctx, id)
}

// Detail 问题详情 + 关联事件 id + 关联变更 id。
func (s *Service) Detail(ctx context.Context, actor Actor, id uint64) (*Detail, error) {
	p, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &Detail{Problem: p, IncidentIDs: []uint64{}, ChangeIDs: []uint64{}}
	if s.incidents != nil {
		ids, err := s.incidents.ListIncidentIDsByProblem(ctx, id)
		if err != nil {
			return nil, httpx.ErrInternal("查询关联事件失败: " + err.Error())
		}
		if ids != nil {
			d.IncidentIDs = ids
		}
	}
	if s.changes != nil {
		ids, err := s.changes.ListChangeIDs(ctx, id)
		if err != nil {
			return nil, httpx.ErrInternal("查询关联变更失败: " + err.Error())
		}
		if ids != nil {
			d.ChangeIDs = ids
		}
	}
	return d, nil
}

// List 分页查询问题。
func (s *Service) List(ctx context.Context, actor Actor, q ListQuery) ([]Problem, int64, error) {
	if s.repo == nil {
		return nil, 0, httpx.ErrInternal("问题仓储未初始化")
	}
	q.SortBy, q.Order = normalizeSort(q.SortBy, q.Order)
	items, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, 0, httpx.ErrInternal("查询问题失败: " + err.Error())
	}
	return items, total, nil
}

// ---------------- 编辑 / RCA ----------------

// Update 编辑问题与 RCA 字段。
func (s *Service) Update(ctx context.Context, actor Actor, id uint64, req UpdateRequest) (*Problem, error) {
	p, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	before := *p
	if req.Title != nil {
		if strings.TrimSpace(*req.Title) == "" {
			return nil, httpx.ErrBadRequest("标题不得为空")
		}
		p.Title = *req.Title
	}
	if req.Description != nil {
		p.Description = *req.Description
	}
	if req.Symptom != nil {
		p.Symptom = *req.Symptom
	}
	if req.Analysis != nil {
		p.Analysis = *req.Analysis
	}
	if req.RootCause != nil {
		p.RootCause = *req.RootCause
	}
	if req.Workaround != nil {
		p.Workaround = *req.Workaround
	}
	p.UpdatedAt = s.now().UTC()
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, httpx.ErrInternal("更新问题失败: " + err.Error())
	}
	s.audit(ctx, actor, "update", p.ID, "", "", before, p)
	return p, nil
}

// Delete 软删除问题（admin）。
func (s *Service) Delete(ctx context.Context, actor Actor, id uint64) error {
	if s.repo == nil {
		return httpx.ErrInternal("问题仓储未初始化")
	}
	p, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return httpx.ErrInternal("删除问题失败: " + err.Error())
	}
	s.audit(ctx, actor, "delete", p.ID, p.Status, "", p, nil)
	return nil
}

// ---------------- 状态流转 ----------------

// Transition 统一状态流转。
func (s *Service) Transition(ctx context.Context, actor Actor, id uint64, action string, req TransitionRequest) (*Problem, error) {
	p, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	tr, ok := problemMachine.Resolve(p.Status, action)
	if !ok {
		// PRD §5 / docs/API.md：非法流转 409 消息含「当前状态 -> 目标状态」对。
		return nil, httpx.ErrConflict(illegalTransitionMsg(p.Status, action, "当前状态不允许该动作"))
	}
	if !tr.Allows(actor.Role) {
		return nil, httpx.ErrForbidden("当前角色无权执行该流转")
	}
	params := s.buildParams(action, req)
	// 预写入实体字段，供 guard 校验。
	if action == ActionMarkKnownError || action == ActionUpdateWorkaround {
		if req.RootCause != "" {
			p.RootCause = req.RootCause
		}
		if req.Workaround != "" {
			p.Workaround = req.Workaround
		}
	}
	if action == ActionResolve && req.NoChangeReason != "" {
		p.NoChangeReason = req.NoChangeReason
	}
	// 解决约束：查询关联变更中 closed 的数量。
	if action == ActionResolve && s.changeRead != nil && s.changes != nil {
		ids, err := s.changes.ListChangeIDs(ctx, id)
		if err != nil {
			return nil, httpx.ErrInternal("查询关联变更失败: " + err.Error())
		}
		n, err := s.changeRead.ClosedChangeCount(ctx, ids)
		if err != nil {
			return nil, httpx.ErrInternal("查询变更状态失败: " + err.Error())
		}
		params["closed_changes"] = n
	}
	now := s.now().UTC()
	if tr.Guard != nil {
		if err := tr.Guard(statemachine.GuardInput{
			Ctx: ctx, Actor: statemachine.Actor{UserID: actor.UserID, Role: actor.Role},
			Entity: p, Params: params, Now: now,
		}); err != nil {
			return nil, err
		}
	}
	if action == ActionInvestigate {
		if err := s.validateAssignee(ctx, params); err != nil {
			return nil, err
		}
	}
	from := p.Status
	s.applyTransition(p, action, tr.To, params, now)
	p.UpdatedAt = now
	if err := s.repo.Update(ctx, p); err != nil {
		return nil, httpx.ErrInternal("流转问题失败: " + err.Error())
	}
	s.audit(ctx, actor, "transition:"+action, p.ID, from, p.Status, nil, map[string]any{"action": action, "status": p.Status})
	return p, nil
}

// MarkKnownError 标记/更新已知错误（root_cause & workaround 非空，否则 422）。
func (s *Service) MarkKnownError(ctx context.Context, actor Actor, id uint64, rootCause, workaround string) (*Problem, error) {
	if strings.TrimSpace(rootCause) == "" || strings.TrimSpace(workaround) == "" {
		return nil, httpx.ErrPrecondition("标记已知错误须填写 root_cause 与 workaround")
	}
	p, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	switch p.Status {
	case StatusInvestigating:
		return s.Transition(ctx, actor, id, ActionMarkKnownError, TransitionRequest{Action: ActionMarkKnownError, RootCause: rootCause, Workaround: workaround})
	case StatusKnownError:
		return s.Transition(ctx, actor, id, ActionUpdateWorkaround, TransitionRequest{Action: ActionUpdateWorkaround, RootCause: rootCause, Workaround: workaround})
	default:
		return nil, httpx.ErrConflict("仅 investigating/known_error 状态可标记已知错误")
	}
}

// ---------------- 关联变更 ----------------

// AddChanges 关联变更。
func (s *Service) AddChanges(ctx context.Context, actor Actor, id uint64, changeIDs []uint64) error {
	if s.changes == nil {
		return httpx.ErrInternal("关联仓储未初始化")
	}
	if _, err := s.load(ctx, id); err != nil {
		return err
	}
	if len(changeIDs) == 0 {
		return httpx.ErrBadRequest("change_ids 不得为空")
	}
	if err := s.changes.Add(ctx, id, changeIDs); err != nil {
		return httpx.ErrInternal("关联变更失败: " + err.Error())
	}
	s.audit(ctx, actor, "attach_changes", id, "", "", nil, map[string]any{"change_ids": changeIDs})
	return nil
}

// RemoveChange 解除变更关联。
func (s *Service) RemoveChange(ctx context.Context, actor Actor, id, changeID uint64) error {
	if s.changes == nil {
		return httpx.ErrInternal("关联仓储未初始化")
	}
	if _, err := s.load(ctx, id); err != nil {
		return err
	}
	if err := s.changes.Remove(ctx, id, changeID); err != nil {
		return httpx.ErrInternal("解除变更失败: " + err.Error())
	}
	s.audit(ctx, actor, "detach_change", id, "", "", nil, map[string]any{"change_id": changeID})
	return nil
}

// ---------------- 建议聚合 ----------------

// Suggestion 是「建议聚合」提示项。
type Suggestion struct {
	Type    string `json:"type"` // ci / keyword
	Key     string `json:"key"`
	Count   int    `json:"count"`
	Message string `json:"message"`
}

// AggregateSuggestions 返回建议聚合提示（同 CI ≥3 次 / 同关键词 ≥2 次）。
func (s *Service) AggregateSuggestions(ctx context.Context, ciID uint64, keyword string, days int) ([]Suggestion, error) {
	if s.incidents == nil {
		return nil, httpx.ErrInternal("事件读取组件未初始化")
	}
	if days <= 0 {
		days = 30
	}
	since := s.now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	out := make([]Suggestion, 0, 2)
	if ciID != 0 {
		n, err := s.incidents.CountIncidentsByCI(ctx, ciID, since)
		if err != nil {
			return nil, httpx.ErrInternal("统计事件失败: " + err.Error())
		}
		if n >= 3 {
			out = append(out, Suggestion{Type: "ci", Key: itoa(ciID), Count: n, Message: "同一 CI 近期事件频发，建议聚合为问题"})
		}
	}
	if strings.TrimSpace(keyword) != "" {
		n, err := s.incidents.CountIncidentsByKeyword(ctx, keyword, since)
		if err != nil {
			return nil, httpx.ErrInternal("统计事件失败: " + err.Error())
		}
		if n >= 2 {
			out = append(out, Suggestion{Type: "keyword", Key: keyword, Count: n, Message: "同根因关键词命中多个事件，建议聚合为问题"})
		}
	}
	return out, nil
}

// ---------------- 内部 helpers ----------------

func (s *Service) load(ctx context.Context, id uint64) (*Problem, error) {
	if s.repo == nil {
		return nil, httpx.ErrInternal("问题仓储未初始化")
	}
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "问题不存在")
	}
	return p, nil
}

func (s *Service) buildParams(action string, req TransitionRequest) map[string]any {
	p := map[string]any{
		"action":           action,
		"reason":           strings.TrimSpace(req.Reason),
		"no_change_reason": strings.TrimSpace(req.NoChangeReason),
	}
	if req.AssigneeID != nil {
		p["assignee_id"] = *req.AssigneeID
	}
	return p
}

func (s *Service) validateAssignee(ctx context.Context, params map[string]any) error {
	id, _ := params["assignee_id"].(uint64)
	if id == 0 {
		return httpx.ErrPrecondition("须指定有效的 assignee_id")
	}
	if s.users == nil {
		return nil
	}
	if _, err := s.users.GetUser(ctx, id); err != nil {
		return httpx.ErrPrecondition("指派人不存在或不可用")
	}
	return nil
}

func (s *Service) applyTransition(p *Problem, action, to string, params map[string]any, now time.Time) {
	switch action {
	case ActionInvestigate:
		if v, ok := params["assignee_id"].(uint64); ok && v != 0 {
			a := v
			p.AssigneeID = &a
		}
	case ActionMarkKnownError, ActionUpdateWorkaround:
		// 实体字段已在 Transition 预写入。
	case ActionResolve:
		if v, ok := params["no_change_reason"].(string); ok && v != "" {
			p.NoChangeReason = v
		}
		if p.ResolvedAt == nil {
			p.ResolvedAt = &now
		}
	case ActionClose:
		if p.ClosedAt == nil {
			p.ClosedAt = &now
		}
	case ActionRecur:
		p.ResolvedAt = nil
	}
	p.Status = to
}

func (s *Service) nextCode(ctx context.Context, now time.Time) (string, error) {
	max := 0
	if s.repo != nil {
		m, err := s.repo.MaxDailySeq(ctx, idgen.PrefixProblem, now)
		if err != nil {
			return "", httpx.ErrInternal("生成问题编号失败: " + err.Error())
		}
		max = m
	}
	return s.gen.Next(idgen.PrefixProblem, now, max), nil
}

func (s *Service) audit(ctx context.Context, actor Actor, action string, bizID uint64, from, to string, before, after any) {
	if s.auditor == nil {
		return
	}
	e := platform.AuditEntry{
		ActorID: actor.UserID, Action: action, BizType: platform.BizTypeProblem, BizID: bizID,
		FromStatus: from, ToStatus: to, BeforeValue: toJSON(before), AfterValue: toJSON(after), ClientIP: actor.ClientIP,
	}
	if err := s.auditor.AppendAudit(ctx, e); err != nil {
		s.log.Warn("写入问题审计失败", zap.Uint64("problem_id", bizID), zap.Error(err))
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
	allowed := map[string]bool{"created_at": true, "status": true, "id": true}
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

// itoa 无符号整数转字符串（避免引入 strconv 到纯逻辑函数）。
func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
