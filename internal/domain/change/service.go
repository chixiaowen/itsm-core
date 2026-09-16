// 本文件承载 change 域业务规则：申请/风险评估/计划/提交审批/CAB 会签/实施回滚/回顾。
package change

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

// Deps 是 change service 的依赖集合。
type Deps struct {
	Repo          Repository
	Approvals     ApprovalRepository
	Users         UserDirectory
	Auditor       Auditor
	IDGen         *idgen.Generator
	EnforceWindow bool
	Logger        *zap.Logger
	Now           func() time.Time
}

// Service 承载变更业务。
type Service struct {
	repo          Repository
	approvals     ApprovalRepository
	users         UserDirectory
	auditor       Auditor
	gen           *idgen.Generator
	enforceWindow bool
	log           *zap.Logger
	now           func() time.Time
}

// NewService 构造 change service。
func NewService(d Deps) *Service {
	s := &Service{
		repo: d.Repo, approvals: d.Approvals, users: d.Users, auditor: d.Auditor,
		gen: d.IDGen, enforceWindow: d.EnforceWindow, log: d.Logger, now: d.Now,
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

// ---------------- 申请 ----------------

// Create 提交变更申请：编号 + 初始状态 draft。
func (s *Service) Create(ctx context.Context, actor Actor, req CreateRequest) (*Change, error) {
	if s.repo == nil {
		return nil, httpx.ErrInternal("变更仓储未初始化")
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, httpx.ErrBadRequest("标题必填")
	}
	if !validChangeType(req.ChangeType) {
		return nil, httpx.ErrBadRequest("非法变更类型（standard/normal/emergency）")
	}
	if !validRisk(req.RiskLevel) {
		return nil, httpx.ErrBadRequest("非法风险等级（high/medium/low）")
	}
	now := s.now().UTC()
	code, err := s.nextCode(ctx, now)
	if err != nil {
		return nil, err
	}
	ch := &Change{
		Code: code, Title: req.Title, Description: req.Description, ChangeType: req.ChangeType,
		Status: StatusDraft, RiskLevel: req.RiskLevel, RequesterID: actor.UserID,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.repo.Create(ctx, ch); err != nil {
		return nil, httpx.ErrInternal("创建变更失败: " + err.Error())
	}
	s.audit(ctx, actor, "create", ch.ID, "", ch.Status, nil, ch)
	return ch, nil
}

// ---------------- 查询 ----------------

// Get 查询变更。
func (s *Service) Get(ctx context.Context, actor Actor, id uint64) (*Change, error) {
	return s.load(ctx, id)
}

// Detail 变更详情 + 审批记录。
func (s *Service) Detail(ctx context.Context, actor Actor, id uint64) (*Detail, error) {
	ch, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &Detail{Change: ch, Approvals: []ChangeApproval{}}
	if s.approvals != nil {
		as, err := s.approvals.ListByChange(ctx, id)
		if err != nil {
			return nil, httpx.ErrInternal("查询审批记录失败: " + err.Error())
		}
		if as != nil {
			d.Approvals = as
		}
	}
	return d, nil
}

// ListApprovals 返回审批记录。
func (s *Service) ListApprovals(ctx context.Context, id uint64) ([]ChangeApproval, error) {
	if s.approvals == nil {
		return nil, httpx.ErrInternal("审批仓储未初始化")
	}
	if _, err := s.load(ctx, id); err != nil {
		return nil, err
	}
	return s.approvals.ListByChange(ctx, id)
}

// ClosedChangeCount 返回给定变更中状态为 closed 的数量（消费者侧：供 problem 域校验解决约束）。
func (s *Service) ClosedChangeCount(ctx context.Context, changeIDs []uint64) (int, error) {
	if len(changeIDs) == 0 {
		return 0, nil
	}
	if s.repo == nil {
		return 0, httpx.ErrInternal("变更仓储未初始化")
	}
	return s.repo.CountClosedByIDs(ctx, changeIDs)
}

// List 分页查询变更。
func (s *Service) List(ctx context.Context, actor Actor, q ListQuery) ([]Change, int64, error) {
	if s.repo == nil {
		return nil, 0, httpx.ErrInternal("变更仓储未初始化")
	}
	q.SortBy, q.Order = normalizeSort(q.SortBy, q.Order)
	items, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, 0, httpx.ErrInternal("查询变更失败: " + err.Error())
	}
	return items, total, nil
}

// ---------------- 编辑（类型锁定）----------------

// Update 编辑变更：仅 draft/assessment 可改；进入审批后类型不可改（409）。
func (s *Service) Update(ctx context.Context, actor Actor, id uint64, req UpdateRequest) (*Change, error) {
	ch, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if ch.Status != StatusDraft && ch.Status != StatusAssessment {
		return nil, httpx.ErrConflict("仅 draft/assessment 状态可编辑")
	}
	before := *ch
	if req.Title != nil {
		if strings.TrimSpace(*req.Title) == "" {
			return nil, httpx.ErrBadRequest("标题不得为空")
		}
		ch.Title = *req.Title
	}
	if req.Description != nil {
		ch.Description = *req.Description
	}
	if req.ChangeType != nil && *req.ChangeType != ch.ChangeType {
		// 类型变更仅在 draft 阶段允许；进入 assessment 后锁定。
		if ch.Status != StatusDraft {
			return nil, httpx.ErrConflict("变更类型在提交风险评估后不可修改")
		}
		if !validChangeType(*req.ChangeType) {
			return nil, httpx.ErrBadRequest("非法变更类型")
		}
		ch.ChangeType = *req.ChangeType
	}
	if req.RiskLevel != nil {
		if !validRisk(*req.RiskLevel) {
			return nil, httpx.ErrBadRequest("非法风险等级")
		}
		ch.RiskLevel = *req.RiskLevel
	}
	if req.ImpactAnalysis != nil {
		ch.ImpactAnalysis = *req.ImpactAnalysis
	}
	if req.Plan != nil {
		ch.Plan = *req.Plan
	}
	if req.RollbackPlan != nil {
		ch.RollbackPlan = *req.RollbackPlan
	}
	if req.WindowStart != nil {
		ch.WindowStart = req.WindowStart
	}
	if req.WindowEnd != nil {
		ch.WindowEnd = req.WindowEnd
	}
	if ch.WindowStart != nil && ch.WindowEnd != nil && !ch.WindowStart.Before(*ch.WindowEnd) {
		return nil, httpx.ErrPrecondition("window_start 必须早于 window_end")
	}
	ch.UpdatedAt = s.now().UTC()
	if err := s.repo.Update(ctx, ch); err != nil {
		return nil, httpx.ErrInternal("更新变更失败: " + err.Error())
	}
	s.audit(ctx, actor, "update", ch.ID, "", "", before, ch)
	return ch, nil
}

// Delete 软删除变更（admin）。
func (s *Service) Delete(ctx context.Context, actor Actor, id uint64) error {
	if s.repo == nil {
		return httpx.ErrInternal("变更仓储未初始化")
	}
	ch, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return httpx.ErrInternal("删除变更失败: " + err.Error())
	}
	s.audit(ctx, actor, "delete", ch.ID, ch.Status, "", ch, nil)
	return nil
}

// ---------------- 状态流转 ----------------

// Transition 统一状态流转。
func (s *Service) Transition(ctx context.Context, actor Actor, id uint64, action string, req TransitionRequest) (*Change, error) {
	ch, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	tr, ok := changeMachine.Resolve(ch.Status, action)
	if !ok {
		// PRD §5 / docs/API.md：非法流转 409 消息含「当前状态 -> 目标状态」对。
		return nil, httpx.ErrConflict(illegalTransitionMsg(ch.Status, action, "当前状态不允许该动作"))
	}
	if !tr.Allows(actor.Role) {
		return nil, httpx.ErrForbidden("当前角色无权执行该流转")
	}
	params := s.buildParams(action, req)
	if action == ActionApprove {
		apvs, err := s.listApprovals(ctx, id)
		if err != nil {
			return nil, err
		}
		params["approvals_decision"] = EvaluateApprovals(ch.ChangeType, apvs)
	}
	// 排期时先把窗口写入实体，供 guardWindowOrder 校验。
	if action == ActionSchedule {
		if req.WindowStart != nil {
			ch.WindowStart = req.WindowStart
		}
		if req.WindowEnd != nil {
			ch.WindowEnd = req.WindowEnd
		}
	}
	now := s.now().UTC()
	if tr.Guard != nil {
		if err := tr.Guard(statemachine.GuardInput{
			Ctx: ctx, Actor: statemachine.Actor{UserID: actor.UserID, Role: actor.Role},
			Entity: ch, Params: params, Now: now,
		}); err != nil {
			return nil, err
		}
	}
	from := ch.Status
	s.applyTransition(ch, action, tr.To, params, now)
	ch.UpdatedAt = now
	if err := s.repo.Update(ctx, ch); err != nil {
		return nil, httpx.ErrInternal("流转变更失败: " + err.Error())
	}
	// 进入 pending_approval 视为开启新一轮 CAB 会签：清理上一轮审批记录，
	// 既避免旧票计入新轮判定，也保证复合唯一索引 idx_apv_change_approver 不阻断重投。
	if action == ActionSubmitApproval && s.approvals != nil {
		if err := s.approvals.DeleteByChange(ctx, id); err != nil {
			return nil, httpx.ErrInternal("重置审批记录失败: " + err.Error())
		}
	}
	s.audit(ctx, actor, "transition:"+action, ch.ID, from, ch.Status, nil, map[string]any{"action": action, "status": ch.Status})
	return ch, nil
}

// RecordApproval 记录一条 CAB 审批决定；达成就自动流转（通过/驳回）。
func (s *Service) RecordApproval(ctx context.Context, actor Actor, id uint64, req ApprovalRequest) (*Change, error) {
	if s.approvals == nil {
		return nil, httpx.ErrInternal("审批仓储未初始化")
	}
	ch, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if ch.Status != StatusPendingApproval {
		return nil, httpx.ErrConflict("仅 pending_approval 状态可审批")
	}
	if req.Decision != DecisionApprove && req.Decision != DecisionReject {
		return nil, httpx.ErrBadRequest("审批决定须为 approved 或 rejected")
	}
	now := s.now().UTC()
	// 先查后写：同一审批人对同一变更只允许一票（防 CAB 会签被单人绕过；对齐 PRD §8.6）。
	existing, err := s.approvals.FindByChangeAndApprover(ctx, id, actor.UserID)
	if err != nil {
		return nil, httpx.ErrInternal("查询审批记录失败: " + err.Error())
	}
	if existing != nil {
		return nil, httpx.ErrConflict("您已对该变更提交过审批决定，不可重复投票或改票")
	}
	a := &ChangeApproval{
		ChangeID: id, ApproverID: actor.UserID, Decision: req.Decision,
		Comment: req.Comment, DecidedAt: &now, CreatedAt: now,
	}
	if err := s.approvals.Create(ctx, a); err != nil {
		return nil, httpx.ErrInternal("写入审批记录失败: " + err.Error())
	}
	s.audit(ctx, actor, "approval:"+req.Decision, id, "", "", nil, map[string]any{"decision": req.Decision})

	apvs, err := s.approvals.ListByChange(ctx, id)
	if err != nil {
		return nil, httpx.ErrInternal("查询审批记录失败: " + err.Error())
	}
	switch EvaluateApprovals(ch.ChangeType, apvs) {
	case DecisionApprove:
		return s.Transition(ctx, actor, id, ActionApprove, TransitionRequest{Action: ActionApprove})
	case DecisionReject:
		return s.Transition(ctx, actor, id, ActionReject, TransitionRequest{Action: ActionReject, Comment: req.Comment})
	default:
		// 尚未达成，返回当前变更（待审）。
		return s.load(ctx, id)
	}
}

// ---------------- 内部 helpers ----------------

func (s *Service) load(ctx context.Context, id uint64) (*Change, error) {
	if s.repo == nil {
		return nil, httpx.ErrInternal("变更仓储未初始化")
	}
	ch, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "变更不存在")
	}
	return ch, nil
}

func (s *Service) listApprovals(ctx context.Context, id uint64) ([]ChangeApproval, error) {
	if s.approvals == nil {
		return nil, nil
	}
	as, err := s.approvals.ListByChange(ctx, id)
	if err != nil {
		return nil, httpx.ErrInternal(err.Error())
	}
	return as, nil
}

func (s *Service) buildParams(action string, req TransitionRequest) map[string]any {
	p := map[string]any{
		"action":                action,
		"result":                strings.TrimSpace(req.Result),
		"reason":                strings.TrimSpace(req.Reason),
		"comment":               strings.TrimSpace(req.Comment),
		"conclusion":            strings.TrimSpace(req.Conclusion),
		"confirm_out_of_window": req.ConfirmOutOfWindow,
		"enforce_window":        s.enforceWindow,
	}
	if req.WindowStart != nil {
		p["window_start"] = *req.WindowStart
	}
	if req.WindowEnd != nil {
		p["window_end"] = *req.WindowEnd
	}
	return p
}

func (s *Service) applyTransition(ch *Change, action, to string, params map[string]any, now time.Time) {
	switch action {
	case ActionPreAuthorize:
		ch.PreAuthorized = true
	case ActionSchedule:
		if v, ok := params["window_start"].(time.Time); ok {
			ch.WindowStart = &v
		}
		if v, ok := params["window_end"].(time.Time); ok {
			ch.WindowEnd = &v
		}
	case ActionComplete:
		if v, ok := params["result"].(string); ok {
			ch.ImplementResult = v
		}
	case ActionRollback:
		if v, ok := params["reason"].(string); ok {
			ch.RollbackReason = v
		}
	case ActionClose:
		if v, ok := params["conclusion"].(string); ok {
			ch.ReviewConclusion = v
		}
		if ch.ClosedAt == nil {
			ch.ClosedAt = &now
		}
	}
	ch.Status = to
}

func (s *Service) nextCode(ctx context.Context, now time.Time) (string, error) {
	max := 0
	if s.repo != nil {
		m, err := s.repo.MaxDailySeq(ctx, idgen.PrefixChange, now)
		if err != nil {
			return "", httpx.ErrInternal("生成变更编号失败: " + err.Error())
		}
		max = m
	}
	return s.gen.Next(idgen.PrefixChange, now, max), nil
}

func (s *Service) audit(ctx context.Context, actor Actor, action string, bizID uint64, from, to string, before, after any) {
	if s.auditor == nil {
		return
	}
	e := platform.AuditEntry{
		ActorID: actor.UserID, Action: action, BizType: platform.BizTypeChange, BizID: bizID,
		FromStatus: from, ToStatus: to, BeforeValue: toJSON(before), AfterValue: toJSON(after), ClientIP: actor.ClientIP,
	}
	if err := s.auditor.AppendAudit(ctx, e); err != nil {
		s.log.Warn("写入变更审计失败", zap.Uint64("change_id", bizID), zap.Error(err))
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

func validChangeType(t string) bool {
	switch t {
	case TypeStandard, TypeNormal, TypeEmergency:
		return true
	default:
		return false
	}
}

func validRisk(r string) bool {
	switch r {
	case RiskHigh, RiskMedium, RiskLow:
		return true
	default:
		return false
	}
}

func normalizeSort(sortBy, order string) (string, string) {
	allowed := map[string]bool{"created_at": true, "risk_level": true, "status": true, "id": true}
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
