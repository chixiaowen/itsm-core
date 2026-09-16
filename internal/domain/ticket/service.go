// 本文件承载 ticket 域业务规则：创建/指派/流转/SLA/挂起恢复/评价/重开/时间线。
//
// 铁律：service 只依赖本包 repository 接口与 pkg/*，禁止 import gorm。
package ticket

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
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
	"github.com/chixiaowen/itsm-core/internal/pkg/sla"
	"github.com/chixiaowen/itsm-core/internal/pkg/statemachine"
)

// Actor 是执行操作的当前用户。
type Actor struct {
	UserID   uint64
	Role     string
	ClientIP string
}

// Deps 是 ticket service 的依赖集合。
type Deps struct {
	Repo       Repository
	Categories CategoryRepository
	Users      UserDirectory
	SLAs       SLAPolicyReader
	Comments   CommentStore
	IDGen      *idgen.Generator
	Logger     *zap.Logger
	Now        func() time.Time
}

// Service 承载工单业务。
type Service struct {
	repo       Repository
	categories CategoryRepository
	users      UserDirectory
	slas       SLAPolicyReader
	comments   CommentStore
	gen        *idgen.Generator
	log        *zap.Logger
	now        func() time.Time
}

// NewService 构造 ticket service，未提供的可选依赖回退到安全默认值。
func NewService(d Deps) *Service {
	s := &Service{
		repo:       d.Repo,
		categories: d.Categories,
		users:      d.Users,
		slas:       d.SLAs,
		comments:   d.Comments,
		gen:        d.IDGen,
		log:        d.Logger,
		now:        d.Now,
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

// ---------------- 创建 ----------------

// Create 创建工单：编号 + SLA 截止时间 + 初始状态 new。
func (s *Service) Create(ctx context.Context, actor Actor, req CreateTicketRequest) (*Ticket, error) {
	if s.repo == nil {
		return nil, httpx.ErrInternal("工单仓储未初始化")
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, httpx.ErrBadRequest("标题必填")
	}
	requester := actor.UserID
	if req.RequesterID != nil && *req.RequesterID != 0 {
		requester = *req.RequesterID
	}
	if requester == 0 {
		return nil, httpx.ErrBadRequest("请求人必填")
	}
	priority := req.Priority
	if priority == "" {
		priority = PriorityP4
	}
	if !validPriority(priority) {
		return nil, httpx.ErrBadRequest("非法优先级: " + priority)
	}
	typ := req.Type
	if typ == "" {
		typ = TypeManual
	}

	now := s.now().UTC()
	policy := s.policyFor(ctx, priority)
	code, err := s.nextCode(ctx, now)
	if err != nil {
		return nil, err
	}

	responseDue := sla.Due(now, policy.ResponseMinutes)
	resolveDue := sla.Due(now, policy.ResolveMinutes)
	tk := &Ticket{
		Code:             code,
		Title:            req.Title,
		Description:      req.Description,
		Status:           StatusNew,
		Priority:         priority,
		Type:             typ,
		CategoryID:       req.CategoryID,
		RequesterID:      requester,
		ServiceItemID:    req.ServiceItemID,
		SourceIncidentID: req.SourceIncidentID,
		FormData:         req.FormData,
		ResponseDueAt:    &responseDue,
		ResolveDueAt:     &resolveDue,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.repo.Create(ctx, tk); err != nil {
		return nil, httpx.ErrInternal("创建工单失败: " + err.Error())
	}
	s.audit(ctx, actor, "create", tk.ID, "", tk.Status, nil, tk)
	return s.decorate(ctx, tk), nil
}

// CreateFromIncident 由事件域转单调用（消费者侧接口实现，仅用基础类型）。
func (s *Service) CreateFromIncident(ctx context.Context, title, description string, requesterID, incidentID uint64, priority string) (uint64, string, error) {
	inc := incidentID
	tk, err := s.Create(ctx, Actor{UserID: requesterID, Role: role.Agent}, CreateTicketRequest{
		Title:            title,
		Description:      description,
		Priority:         priority,
		RequesterID:      &requesterID,
		Type:             TypeIncident,
		SourceIncidentID: &inc,
	})
	if err != nil {
		return 0, "", err
	}
	return tk.ID, tk.Code, nil
}

// CreateFromServiceItem 由服务目录域下单调用（消费者侧接口实现，仅用基础类型）。
func (s *Service) CreateFromServiceItem(ctx context.Context, title, description string, requesterID uint64, categoryID, serviceItemID uint64, priority, formData string) (uint64, string, error) {
	cat := categoryID
	item := serviceItemID
	tk, err := s.Create(ctx, Actor{UserID: requesterID, Role: role.Requestor}, CreateTicketRequest{
		Title:         title,
		Description:   description,
		Priority:      priority,
		RequesterID:   &requesterID,
		Type:          TypeService,
		CategoryID:    &cat,
		ServiceItemID: &item,
		FormData:      formData,
	})
	if err != nil {
		return 0, "", err
	}
	return tk.ID, tk.Code, nil
}

// ---------------- 查询 ----------------

// Get 查询工单详情（含资源级鉴权）。
func (s *Service) Get(ctx context.Context, actor Actor, id uint64) (*Ticket, error) {
	tk, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canView(actor, tk) {
		return nil, httpx.ErrForbidden("无权查看该工单")
	}
	return s.decorate(ctx, tk), nil
}

// Detail 查询工单详情 + 关联 CI + 评论时间线。
func (s *Service) Detail(ctx context.Context, actor Actor, id uint64, includeInternal bool) (*Detail, error) {
	tk, err := s.Get(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	d := &Detail{Ticket: tk, CIs: []uint64{}}
	if s.repo != nil {
		cis, err := s.repo.ListCIs(ctx, id)
		if err != nil {
			return nil, httpx.ErrInternal("查询关联 CI 失败: " + err.Error())
		}
		if cis != nil {
			d.CIs = cis
		}
	}
	if s.comments != nil {
		if includeInternal {
			includeInternal = role.Has(actor.Role, role.PermTicketHandle)
		}
		cs, err := s.comments.ListComments(ctx, platform.BizTypeTicket, id, includeInternal)
		if err != nil {
			return nil, httpx.ErrInternal("查询评论失败: " + err.Error())
		}
		d.Comments = cs
	}
	return d, nil
}

// List 分页查询工单（requestor 仅本人；支持 SLA 状态应用层过滤）。
func (s *Service) List(ctx context.Context, actor Actor, q ListQuery) ([]Ticket, int64, error) {
	if s.repo == nil {
		return nil, 0, httpx.ErrInternal("工单仓储未初始化")
	}
	// requestor 仅本人数据（资源级鉴权）。
	if !role.Has(actor.Role, role.PermTicketViewAll) {
		own := actor.UserID
		q.RequesterID = &own
	}
	q.SortBy, q.Order = normalizeSort(q.SortBy, q.Order)

	needFilter := q.SLAStatus != ""
	offset, limit := q.Offset, q.Limit
	if needFilter {
		q.Offset, q.Limit = 0, 0 // 取全集，应用层过滤后分页
	}
	items, total, err := s.repo.List(ctx, q)
	if err != nil {
		return nil, 0, httpx.ErrInternal("查询工单失败: " + err.Error())
	}
	if needFilter {
		filtered := make([]Ticket, 0, len(items))
		for i := range items {
			s.decorate(ctx, &items[i])
			if items[i].SLAStatus == q.SLAStatus {
				filtered = append(filtered, items[i])
			}
		}
		total = int64(len(filtered))
		items = pageSlice(filtered, offset, limit)
	}
	for i := range items {
		s.decorate(ctx, &items[i])
	}
	return items, total, nil
}

// ---------------- 编辑 ----------------

// Update 编辑工单（draft/new/assigned 可改；改优先级触发 SLA 重算）。
func (s *Service) Update(ctx context.Context, actor Actor, id uint64, req UpdateTicketRequest) (*Ticket, error) {
	tk, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canEdit(actor, tk) {
		return nil, httpx.ErrForbidden("无权编辑该工单")
	}
	before := *tk

	if req.Title != nil {
		if strings.TrimSpace(*req.Title) == "" {
			return nil, httpx.ErrBadRequest("标题不得为空")
		}
		tk.Title = *req.Title
	}
	if req.Description != nil {
		tk.Description = *req.Description
	}
	if req.CategoryID != nil {
		tk.CategoryID = req.CategoryID
	}
	if req.Priority != nil && *req.Priority != tk.Priority {
		if !validPriority(*req.Priority) {
			return nil, httpx.ErrBadRequest("非法优先级: " + *req.Priority)
		}
		tk.Priority = *req.Priority
		// 修改优先级后按新策略重算 SLA 目标时间（REQ-TKT-003）。
		policy := s.policyFor(ctx, tk.Priority)
		rd := sla.Due(tk.CreatedAt, policy.ResponseMinutes)
		vd := sla.Due(tk.CreatedAt.Add(time.Duration(tk.PausedMinutes)*time.Minute), policy.ResolveMinutes)
		tk.ResponseDueAt = &rd
		tk.ResolveDueAt = &vd
	}
	tk.UpdatedAt = s.now().UTC()
	if err := s.repo.Update(ctx, tk); err != nil {
		return nil, httpx.ErrInternal("更新工单失败: " + err.Error())
	}
	s.audit(ctx, actor, "update", tk.ID, "", "", before, tk)
	return s.decorate(ctx, tk), nil
}

// Delete 软删除工单（admin）。
func (s *Service) Delete(ctx context.Context, actor Actor, id uint64) error {
	if s.repo == nil {
		return httpx.ErrInternal("工单仓储未初始化")
	}
	tk, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return httpx.ErrInternal("删除工单失败: " + err.Error())
	}
	s.audit(ctx, actor, "delete", tk.ID, tk.Status, "", tk, nil)
	return nil
}

// ---------------- 状态流转 ----------------

// Transition 统一状态流转（ARCHITECTURE §6.2 骨架）。
func (s *Service) Transition(ctx context.Context, actor Actor, id uint64, action string, req TransitionRequest) (*Ticket, error) {
	tk, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canView(actor, tk) {
		return nil, httpx.ErrForbidden("无权操作该工单")
	}
	tr, ok := ticketMachine.Resolve(tk.Status, action)
	if !ok {
		// PRD §5 / docs/API.md：非法流转 409 消息含「当前状态 -> 目标状态」对。
		return nil, httpx.ErrConflict(illegalTransitionMsg(tk.Status, action, "当前状态不允许该动作"))
	}
	if !tr.Allows(actor.Role) {
		return nil, httpx.ErrForbidden("当前角色无权执行该流转")
	}

	params := s.buildParams(ctx, tk, action, req)
	if tr.Guard != nil {
		if err := tr.Guard(statemachine.GuardInput{
			Ctx: ctx, Actor: statemachine.Actor{UserID: actor.UserID, Role: actor.Role},
			Entity: tk, Params: params, Now: s.now().UTC(),
		}); err != nil {
			return nil, err
		}
	}
	// 指派有效性由 service 结合用户目录二次校验（guard 只做结构性校验）。
	if action == ActionAssign {
		if err := s.validateAssignee(ctx, params); err != nil {
			return nil, err
		}
	}

	from := tk.Status
	s.applyTransition(tk, action, tr.To, params)
	tk.UpdatedAt = s.now().UTC()
	if err := s.repo.Update(ctx, tk); err != nil {
		return nil, httpx.ErrInternal("流转工单失败: " + err.Error())
	}
	s.audit(ctx, actor, "transition:"+action, tk.ID, from, tk.Status, nil, map[string]any{
		"action": action, "status": tk.Status,
	})
	return s.decorate(ctx, tk), nil
}

// Assign 快捷指派（等价于 transition assign）。
func (s *Service) Assign(ctx context.Context, actor Actor, id uint64, assigneeID uint64) (*Ticket, error) {
	return s.Transition(ctx, actor, id, ActionAssign, TransitionRequest{Action: ActionAssign, AssigneeID: &assigneeID})
}

// Rate 满意度评价（仅工单请求人本人，且仅一次；admin 亦不得评他人单，对齐 PRD §2.2）。
func (s *Service) Rate(ctx context.Context, actor Actor, id uint64, req RatingRequest) (*Ticket, error) {
	tk, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	// 必须是该工单的请求人本人（不因 admin 等特权角色放行）。
	if tk.RequesterID != actor.UserID {
		return nil, httpx.ErrForbidden("仅工单请求人本人可评价")
	}
	if tk.Status != StatusResolved && tk.Status != StatusClosed {
		return nil, httpx.ErrPrecondition("仅已解决/已关闭工单可评价")
	}
	if tk.Rating != nil {
		return nil, httpx.ErrConflict("工单已评价，不可重复评分")
	}
	if req.Rating < 1 || req.Rating > 5 {
		return nil, httpx.ErrBadRequest("评分须在 1~5 之间")
	}
	now := s.now().UTC()
	rating := req.Rating
	tk.Rating = &rating
	tk.RatingComment = req.Comment
	tk.RatedAt = &now
	tk.UpdatedAt = now
	if err := s.repo.Update(ctx, tk); err != nil {
		return nil, httpx.ErrInternal("评价失败: " + err.Error())
	}
	s.audit(ctx, actor, "rate", tk.ID, "", "", nil, map[string]any{"rating": rating})
	return s.decorate(ctx, tk), nil
}

// AddComment 新增公开回复/内部备注；坐席首条公开回复触发首次响应时间。
func (s *Service) AddComment(ctx context.Context, actor Actor, id uint64, req TicketCommentRequest) (*Ticket, error) {
	tk, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canView(actor, tk) {
		return nil, httpx.ErrForbidden("无权评论该工单")
	}
	if strings.TrimSpace(req.Content) == "" {
		return nil, httpx.ErrBadRequest("评论内容不得为空")
	}
	if s.comments == nil {
		return nil, httpx.ErrInternal("评论组件未初始化")
	}
	if _, err := s.comments.CreateComment(ctx, platform.Operator{ID: actor.UserID, Role: actor.Role, ClientIP: actor.ClientIP},
		platform.CommentRequest{BizType: platform.BizTypeTicket, BizID: id, Content: req.Content, IsInternal: req.IsInternal}); err != nil {
		return nil, err
	}
	// 首次响应：坐席（处理权限）的公开回复，且此前未响应。
	if !req.IsInternal && role.Has(actor.Role, role.PermTicketHandle) && tk.FirstRespondedAt == nil {
		now := s.now().UTC()
		tk.FirstRespondedAt = &now
		tk.UpdatedAt = now
		if err := s.repo.Update(ctx, tk); err != nil {
			return nil, httpx.ErrInternal("更新首次响应时间失败: " + err.Error())
		}
	}
	return s.decorate(ctx, tk), nil
}

// ---------------- 关联 CI ----------------

// AddCIs 关联 CI。
func (s *Service) AddCIs(ctx context.Context, actor Actor, id uint64, ciIDs []uint64) error {
	if s.repo == nil {
		return httpx.ErrInternal("工单仓储未初始化")
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
		return httpx.ErrInternal("工单仓储未初始化")
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

// ---------------- 分类 ----------------

// ListCategories 返回全部分类（flat，前端按 parent_id 建树）。
func (s *Service) ListCategories(ctx context.Context) ([]TicketCategory, error) {
	if s.categories == nil {
		return nil, httpx.ErrInternal("分类仓储未初始化")
	}
	return s.categories.List(ctx)
}

// CreateCategory 新建分类。
func (s *Service) CreateCategory(ctx context.Context, actor Actor, req CategoryRequest) (*TicketCategory, error) {
	if s.categories == nil {
		return nil, httpx.ErrInternal("分类仓储未初始化")
	}
	now := s.now().UTC()
	c := &TicketCategory{Name: req.Name, ParentID: req.ParentID, SortOrder: req.SortOrder, CreatedAt: now, UpdatedAt: now}
	if err := s.categories.Create(ctx, c); err != nil {
		return nil, httpx.ErrConflict("创建分类失败: " + err.Error())
	}
	s.audit(ctx, actor, "create", c.ID, "", "", nil, c)
	return c, nil
}

// UpdateCategory 编辑分类。
func (s *Service) UpdateCategory(ctx context.Context, actor Actor, id uint64, req CategoryRequest) (*TicketCategory, error) {
	if s.categories == nil {
		return nil, httpx.ErrInternal("分类仓储未初始化")
	}
	c, err := s.categories.Get(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "分类不存在")
	}
	before := *c
	c.Name = req.Name
	c.ParentID = req.ParentID
	c.SortOrder = req.SortOrder
	c.UpdatedAt = s.now().UTC()
	if err := s.categories.Update(ctx, c); err != nil {
		return nil, httpx.ErrInternal("更新分类失败: " + err.Error())
	}
	s.audit(ctx, actor, "update", c.ID, "", "", before, c)
	return c, nil
}

// DeleteCategory 删除分类（含子分类或含工单 → 409）。
func (s *Service) DeleteCategory(ctx context.Context, actor Actor, id uint64) error {
	if s.categories == nil {
		return httpx.ErrInternal("分类仓储未初始化")
	}
	c, err := s.categories.Get(ctx, id)
	if err != nil {
		return mapRepoErr(err, "分类不存在")
	}
	if n, err := s.categories.CountChildren(ctx, id); err != nil {
		return httpx.ErrInternal(err.Error())
	} else if n > 0 {
		return httpx.ErrConflict("分类含子分类，不可删除")
	}
	if n, err := s.categories.CountByCategory(ctx, id); err != nil {
		return httpx.ErrInternal(err.Error())
	} else if n > 0 {
		return httpx.ErrConflict("分类下存在工单，不可删除")
	}
	if err := s.categories.Delete(ctx, id); err != nil {
		return httpx.ErrInternal("删除分类失败: " + err.Error())
	}
	s.audit(ctx, actor, "delete", c.ID, "", "", c, nil)
	return nil
}

// ---------------- 内部 helpers ----------------

// load 读取工单，归一化 not-found。
func (s *Service) load(ctx context.Context, id uint64) (*Ticket, error) {
	if s.repo == nil {
		return nil, httpx.ErrInternal("工单仓储未初始化")
	}
	tk, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "工单不存在")
	}
	return tk, nil
}

// buildParams 构造 guard 所需参数，并做形态化预校验。
func (s *Service) buildParams(ctx context.Context, tk *Ticket, action string, req TransitionRequest) map[string]any {
	p := map[string]any{
		"action":   action,
		"solution": strings.TrimSpace(req.Solution),
		"reason":   strings.TrimSpace(req.Reason),
	}
	if req.AssigneeID != nil {
		p["assignee_id"] = *req.AssigneeID
	}
	return p
}

// validateAssignee 通过用户目录校验指派人存在（不存在 → 422）。
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

// applyTransition 落地状态变更副作用（时间戳/paused_minutes/solution/assignee）。
func (s *Service) applyTransition(tk *Ticket, action, to string, params map[string]any) {
	now := s.now().UTC()
	switch action {
	case ActionAssign:
		if id, ok := params["assignee_id"].(uint64); ok && id != 0 {
			a := id
			tk.AssigneeID = &a
		}
		// 首次进入 assigned 视为首次响应。
		if tk.FirstRespondedAt == nil {
			tk.FirstRespondedAt = &now
		}
	case ActionReturn:
		tk.AssigneeID = nil
	case ActionPending:
		if tk.PausedAt == nil {
			tk.PausedAt = &now
		}
	case ActionResume:
		if tk.PausedAt != nil {
			delta := now.Sub(*tk.PausedAt)
			if delta > 0 {
				tk.PausedMinutes += int(delta.Minutes())
				if tk.ResolveDueAt != nil {
					shifted := tk.ResolveDueAt.Add(delta)
					tk.ResolveDueAt = &shifted
				}
			}
			tk.PausedAt = nil
		}
	case ActionResolve:
		if sol, ok := params["solution"].(string); ok {
			tk.Solution = sol
		}
		if tk.ResolvedAt == nil { // 只写一次，不可覆盖
			tk.ResolvedAt = &now
		}
	case ActionClose:
		if tk.ClosedAt == nil { // 只写一次，不可覆盖
			tk.ClosedAt = &now
		}
	case ActionReopen:
		tk.ReopenedAt = &now
	}
	tk.Status = to
}

// decorate 注入 SLA 计算字段（不落库）。
func (s *Service) decorate(ctx context.Context, tk *Ticket) *Ticket {
	policy := s.policyFor(ctx, tk.Priority)
	view := sla.Evaluate(policy, tk.CreatedAt, s.now().UTC(), tk.FirstRespondedAt, tk.ResolvedAt, tk.PausedMinutes)
	tk.SLAStatus = string(view.SLAStatus)
	if view.ResponseDueAt != nil {
		v := *view.ResponseDueAt
		tk.ResponseDueAt = &v
	}
	if view.ResolveDueAt != nil {
		v := *view.ResolveDueAt
		tk.ResolveDueAt = &v
	}
	return tk
}

// policyFor 按优先级解析 SLA 策略；缺省回退到默认策略。
func (s *Service) policyFor(ctx context.Context, priority string) sla.Policy {
	if s.slas != nil {
		if list, err := s.slas.ListSLAPolicies(ctx); err == nil {
			for _, p := range list {
				if p.Priority == priority {
					return sla.Policy{
						Priority: p.Priority, ResponseMinutes: p.ResponseMinutes,
						ResolveMinutes: p.ResolveMinutes, PauseOnPending: p.PauseOnPending,
					}
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

// nextCode 生成编号（先查库内当日最大序号 +1）。
func (s *Service) nextCode(ctx context.Context, now time.Time) (string, error) {
	max := 0
	if s.repo != nil {
		m, err := s.repo.MaxDailySeq(ctx, idgen.PrefixTicket, now)
		if err != nil {
			return "", httpx.ErrInternal("生成工单编号失败: " + err.Error())
		}
		max = m
	}
	return s.gen.Next(idgen.PrefixTicket, now, max), nil
}

// audit 写审计；失败仅告警。
func (s *Service) audit(ctx context.Context, actor Actor, action string, bizID uint64, from, to string, before, after any) {
	if s.comments == nil {
		return
	}
	e := platform.AuditEntry{
		ActorID: actor.UserID, Action: action, BizType: platform.BizTypeTicket, BizID: bizID,
		FromStatus: from, ToStatus: to, BeforeValue: toJSON(before), AfterValue: toJSON(after),
		ClientIP: actor.ClientIP,
	}
	if err := s.comments.AppendAudit(ctx, e); err != nil {
		s.log.Warn("写入工单审计失败", zap.Uint64("ticket_id", bizID), zap.Error(err))
	}
}

// ---------------- 纯函数 helpers ----------------

// validPriority 校验优先级取值。
func validPriority(p string) bool {
	switch p {
	case PriorityP1, PriorityP2, PriorityP3, PriorityP4:
		return true
	default:
		return false
	}
}

// canView 资源级鉴权：请求人本人或拥有查看全部权限者。
func canView(a Actor, tk *Ticket) bool {
	if tk.RequesterID == a.UserID {
		return true
	}
	return role.Has(a.Role, role.PermTicketViewAll)
}

// canEdit 可编辑判定：draft/new/assigned 且（本人或坐席/admin）。
func canEdit(a Actor, tk *Ticket) bool {
	switch tk.Status {
	case StatusDraft, StatusNew, StatusAssigned:
	default:
		return false
	}
	if role.Has(a.Role, role.PermTicketViewAll) {
		return true
	}
	return tk.RequesterID == a.UserID
}

// normalizeSort 校验排序字段并回退。
func normalizeSort(sortBy, order string) (string, string) {
	allowed := map[string]bool{"created_at": true, "priority": true, "status": true, "id": true}
	if !allowed[sortBy] {
		sortBy = "created_at"
	}
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	return sortBy, order
}

// pageSlice 对已过滤结果分页。
func pageSlice(items []Ticket, offset, limit int) []Ticket {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return []Ticket{}
	}
	end := len(items)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return items[offset:end]
}

// toJSON 将任意值序列化为审计用的 JSON 字符串；失败或空值返回空串。
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

// mapRepoErr 归一化仓储错误为 AppError。
func mapRepoErr(err error, notFoundMsg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return httpx.ErrNotFound(notFoundMsg)
	}
	return httpx.ErrInternal(err.Error())
}
