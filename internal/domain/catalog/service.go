package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/domain/platform"
	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/statemachine"
)

// 服务项业务类型常量（审计 biz_type）。
const bizTypeServiceItem = "service_item"

// Operator 是执行操作的当前用户。
type Operator struct {
	ID       uint64
	Role     string
	ClientIP string
}

// AuditWriter 复用 platform 的审计能力（consumer 侧接口，避免直接耦合实现）。
type AuditWriter interface {
	AppendAudit(ctx context.Context, e platform.AuditEntry) error
}

// TicketFromServiceItem 是「服务目录下单」转工单的完整入参。
//
// 由 catalog 组装、ticket 域消费；字段为纯值类型，不含任何 ticket 域类型。
type TicketFromServiceItem struct {
	// Title 工单标题（缺省为服务项名称）。
	Title string
	// Description 工单描述。
	Description string
	// RequesterID 请求人（当前下单用户）。
	RequesterID uint64
	// CategoryID 继承的服务分类 ID（可空）。
	CategoryID *uint64
	// Priority 优先级（来自服务项默认优先级，缺省 P4）。
	Priority string
	// SLAPolicyID 继承的 SLA 策略 ID（可空）。
	SLAPolicyID *uint64
	// ServiceItemID 来源服务项 ID（必填，落库后工单 service_item_id 非空）。
	ServiceItemID uint64
	// FormData 动态表单数据快照（JSON 字符串）。
	FormData string
	// RequiresApproval 服务项是否要求审批。
	RequiresApproval bool
}

// TicketCreator 是 catalog 对 ticket 域的消费侧接口（由 ticket service 实现）。
//
// 返回生成的工单主键 ID；失败返回 error（catalog 原样上抛）。
type TicketCreator interface {
	CreateFromServiceItem(ctx context.Context, req TicketFromServiceItem) (ticketID uint64, err error)
}

// Deps 是 catalog service 的依赖集合。
type Deps struct {
	Categories CategoryRepository
	Items      ItemRepository
	Audit      AuditWriter
	Tickets    TicketCreator
	Logger     *zap.Logger
	Now        func() time.Time
}

// Service 承载服务目录业务。
type Service struct {
	categories CategoryRepository
	items      ItemRepository
	audit      AuditWriter
	tickets    TicketCreator
	log        *zap.Logger
	now        func() time.Time
}

// NewService 构造 catalog service，未提供的可选依赖回退到安全默认值。
func NewService(d Deps) *Service {
	s := &Service{
		categories: d.Categories,
		items:      d.Items,
		audit:      d.Audit,
		tickets:    d.Tickets,
		log:        d.Logger,
		now:        d.Now,
	}
	if s.log == nil {
		s.log = zap.NewNop()
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

// ------------------------- 服务分类 -------------------------

// ListCategoryTree 返回分类树（管理台；ItemCount 统计全部分类下服务项数）。
func (s *Service) ListCategoryTree(ctx context.Context) ([]CategoryNode, error) {
	if s.categories == nil || s.items == nil {
		return nil, httpx.ErrInternal("服务目录仓储未初始化")
	}
	cats, err := s.categories.List(ctx)
	if err != nil {
		return nil, httpx.ErrInternal(err.Error())
	}
	counts := make(map[uint64]int, len(cats))
	for _, c := range cats {
		n, err := s.items.CountByCategory(ctx, c.ID)
		if err != nil {
			return nil, httpx.ErrInternal(err.Error())
		}
		counts[c.ID] = int(n)
	}
	nodes := buildCategoryNodes(cats, nil)
	applyItemCounts(nodes, counts)
	return nodes, nil
}

// CreateCategory 新建分类。
func (s *Service) CreateCategory(ctx context.Context, op Operator, req CategoryRequest) (*ServiceCategory, error) {
	if s.categories == nil {
		return nil, httpx.ErrInternal("分类仓储未初始化")
	}
	if err := s.validateParent(ctx, req.ParentID, 0); err != nil {
		return nil, err
	}
	now := s.now()
	c := &ServiceCategory{
		Name:      strings.TrimSpace(req.Name),
		ParentID:  req.ParentID,
		SortOrder: req.SortOrder,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.categories.Create(ctx, c); err != nil {
		return nil, httpx.ErrInternal("创建分类失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "create", bizTypeCategoryAudit, c.ID, "", "")
	return c, nil
}

// UpdateCategory 编辑分类/排序。
func (s *Service) UpdateCategory(ctx context.Context, op Operator, id uint64, req CategoryRequest) (*ServiceCategory, error) {
	if s.categories == nil {
		return nil, httpx.ErrInternal("分类仓储未初始化")
	}
	c, err := s.categories.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "分类不存在")
	}
	if req.ParentID != nil && *req.ParentID == id {
		return nil, httpx.ErrBadRequest("父分类不能是自身")
	}
	if err := s.validateParent(ctx, req.ParentID, id); err != nil {
		return nil, err
	}
	c.Name = strings.TrimSpace(req.Name)
	c.ParentID = req.ParentID
	c.SortOrder = req.SortOrder
	c.UpdatedAt = s.now()
	if err := s.categories.Update(ctx, c); err != nil {
		return nil, httpx.ErrInternal("更新分类失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "update", bizTypeCategoryAudit, c.ID, "", "")
	return c, nil
}

// DeleteCategory 删除分类；含子分类或含服务项时返回 409。
func (s *Service) DeleteCategory(ctx context.Context, op Operator, id uint64) error {
	if s.categories == nil || s.items == nil {
		return httpx.ErrInternal("服务目录仓储未初始化")
	}
	if _, err := s.categories.GetByID(ctx, id); err != nil {
		return mapRepoErr(err, "分类不存在")
	}
	children, err := s.categories.CountChildren(ctx, id)
	if err != nil {
		return httpx.ErrInternal(err.Error())
	}
	if children > 0 {
		return httpx.ErrConflict(fmt.Sprintf("该分类下仍有 %d 个子分类，无法删除", children))
	}
	items, err := s.items.CountByCategory(ctx, id)
	if err != nil {
		return httpx.ErrInternal(err.Error())
	}
	if items > 0 {
		return httpx.ErrConflict(fmt.Sprintf("该分类下仍有 %d 个服务项，无法删除", items))
	}
	if err := s.categories.Delete(ctx, id); err != nil {
		return httpx.ErrInternal("删除分类失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "delete", bizTypeCategoryAudit, id, "", "")
	return nil
}

// validateParent 校验父分类存在性，并防止把分类挂到自身子树下（简单自环保护）。
func (s *Service) validateParent(ctx context.Context, parentID *uint64, selfID uint64) error {
	if parentID == nil {
		return nil
	}
	if _, err := s.categories.GetByID(ctx, *parentID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return httpx.ErrBadRequest("父分类不存在")
		}
		return httpx.ErrInternal(err.Error())
	}
	if *parentID == selfID {
		return httpx.ErrBadRequest("父分类不能是自身")
	}
	return nil
}

// ------------------------- 服务项（管理台） -------------------------

// ListItems 分页查询服务项（管理台）。
func (s *Service) ListItems(ctx context.Context, q ItemListQuery) ([]ServiceItem, int64, error) {
	if s.items == nil {
		return nil, 0, httpx.ErrInternal("服务项仓储未初始化")
	}
	return s.items.List(ctx, q)
}

// GetItem 查询服务项详情。
func (s *Service) GetItem(ctx context.Context, id uint64) (*ServiceItem, error) {
	if s.items == nil {
		return nil, httpx.ErrInternal("服务项仓储未初始化")
	}
	it, err := s.items.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "服务项不存在")
	}
	return it, nil
}

// CreateItem 新建服务项（初始状态 draft）。
func (s *Service) CreateItem(ctx context.Context, op Operator, req ItemRequest) (*ServiceItem, error) {
	if s.items == nil || s.categories == nil {
		return nil, httpx.ErrInternal("服务目录仓储未初始化")
	}
	if _, err := s.categories.GetByID(ctx, req.CategoryID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, httpx.ErrBadRequest("分类不存在")
		}
		return nil, httpx.ErrInternal(err.Error())
	}
	if _, err := ParseFormSchema(req.FormSchema); err != nil {
		return nil, err
	}
	now := s.now()
	it := &ServiceItem{
		Name:             strings.TrimSpace(req.Name),
		Description:      req.Description,
		Status:           StatusDraft,
		CategoryID:       req.CategoryID,
		SLAPolicyID:      req.SLAPolicyID,
		DefaultPriority:  defaultPriority(req.DefaultPriority),
		RequiresApproval: req.RequiresApproval,
		FormSchema:       strings.TrimSpace(req.FormSchema),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.items.Create(ctx, it); err != nil {
		return nil, httpx.ErrInternal("创建服务项失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "create", bizTypeServiceItem, it.ID, "", it.Status)
	return it, nil
}

// UpdateItem 编辑服务项字段。
//
// 规则：published 状态下编辑自动退回 draft（写审计与状态）；archived 为终态返回 409；
// pending_approval 状态下不可编辑（返回 409）。
func (s *Service) UpdateItem(ctx context.Context, op Operator, id uint64, req ItemRequest) (*ServiceItem, error) {
	if s.items == nil || s.categories == nil {
		return nil, httpx.ErrInternal("服务目录仓储未初始化")
	}
	it, err := s.items.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "服务项不存在")
	}
	if _, err := s.categories.GetByID(ctx, req.CategoryID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, httpx.ErrBadRequest("分类不存在")
		}
		return nil, httpx.ErrInternal(err.Error())
	}
	if _, err := ParseFormSchema(req.FormSchema); err != nil {
		return nil, err
	}

	from := it.Status
	switch it.Status {
	case StatusArchived:
		return nil, httpx.ErrConflict(illegalTransitionMsg(it.Status, resolveItemTarget(ActionEdit), "已归档服务项不可编辑"))
	case StatusPendingApproval:
		return nil, httpx.ErrConflict(illegalTransitionMsg(it.Status, resolveItemTarget(ActionEdit), "待审核服务项不可编辑，请先驳回或发布"))
	case StatusPublished:
		// published 编辑即退回 draft（消费状态机表，保持单一事实来源）。
		tr, ok := itemMachine.Resolve(it.Status, ActionEdit)
		if !ok {
			return nil, httpx.ErrConflict(illegalTransitionMsg(it.Status, resolveItemTarget(ActionEdit), "当前状态不允许编辑"))
		}
		it.Status = tr.To
	case StatusDraft, StatusOffline:
		// 可直接编辑。
	default:
		return nil, httpx.ErrConflict(illegalTransitionMsg(it.Status, resolveItemTarget(ActionEdit), "当前状态不可编辑"))
	}

	it.Name = strings.TrimSpace(req.Name)
	it.Description = req.Description
	it.CategoryID = req.CategoryID
	it.SLAPolicyID = req.SLAPolicyID
	it.DefaultPriority = defaultPriority(req.DefaultPriority)
	it.RequiresApproval = req.RequiresApproval
	it.FormSchema = strings.TrimSpace(req.FormSchema)
	it.UpdatedAt = s.now()

	if err := s.items.Update(ctx, it); err != nil {
		return nil, httpx.ErrInternal("更新服务项失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "update", bizTypeServiceItem, it.ID, from, it.Status)
	return it, nil
}

// PublishItem 发布/重新上架：pending_approval -> published、offline -> published。
func (s *Service) PublishItem(ctx context.Context, op Operator, id uint64) (*ServiceItem, error) {
	if s.items == nil {
		return nil, httpx.ErrInternal("服务项仓储未初始化")
	}
	it, err := s.items.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "服务项不存在")
	}
	action := ""
	switch it.Status {
	case StatusPendingApproval:
		action = ActionPublish
	case StatusOffline:
		action = ActionRepublish
	default:
		return nil, httpx.ErrConflict(illegalTransitionMsg(it.Status, StatusPublished, "仅待审核或已下线服务项可发布/重新上架"))
	}
	return s.transition(ctx, op, it, action, nil)
}

// OfflineItem 下线：published -> offline。
func (s *Service) OfflineItem(ctx context.Context, op Operator, id uint64) (*ServiceItem, error) {
	if s.items == nil {
		return nil, httpx.ErrInternal("服务项仓储未初始化")
	}
	it, err := s.items.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "服务项不存在")
	}
	return s.transition(ctx, op, it, ActionOffline, nil)
}

// DeleteItem 归档服务项（draft/offline -> archived；其他状态 409）。
func (s *Service) DeleteItem(ctx context.Context, op Operator, id uint64) error {
	if s.items == nil {
		return httpx.ErrInternal("服务项仓储未初始化")
	}
	it, err := s.items.GetByID(ctx, id)
	if err != nil {
		return mapRepoErr(err, "服务项不存在")
	}
	if _, err := s.transition(ctx, op, it, ActionArchive, nil); err != nil {
		return err
	}
	return nil
}

// TransitionItem 通用状态流转（如驳回 pending_approval -> draft）。
func (s *Service) TransitionItem(ctx context.Context, op Operator, id uint64, action, reason string) (*ServiceItem, error) {
	if s.items == nil {
		return nil, httpx.ErrInternal("服务项仓储未初始化")
	}
	it, err := s.items.GetByID(ctx, id)
	if err != nil {
		return nil, mapRepoErr(err, "服务项不存在")
	}
	params := map[string]any{"reason": reason}
	return s.transition(ctx, op, it, action, params)
}

// transition 执行一次状态流转：非法流转 409、越权 403、前置不满足 422。
func (s *Service) transition(ctx context.Context, op Operator, it *ServiceItem, action string, params map[string]any) (*ServiceItem, error) {
	tr, ok := itemMachine.Resolve(it.Status, action)
	if !ok {
		return nil, httpx.ErrConflict(illegalTransitionMsg(it.Status, resolveItemTarget(action), "当前状态不允许该动作"))
	}
	if !tr.Allows(op.Role) {
		return nil, httpx.ErrForbidden("当前角色无权执行该操作")
	}
	if tr.Guard != nil {
		if err := tr.Guard(statemachine.GuardInput{
			Ctx:    ctx,
			Actor:  statemachine.Actor{UserID: op.ID, Role: op.Role},
			Entity: it,
			Params: params,
			Now:    s.now(),
		}); err != nil {
			return nil, err
		}
	}
	from := it.Status
	it.Status = tr.To
	it.UpdatedAt = s.now()
	if err := s.items.Update(ctx, it); err != nil {
		return nil, httpx.ErrInternal("服务项流转失败: " + err.Error())
	}
	s.auditWrite(ctx, op, "transition", bizTypeServiceItem, it.ID, from, tr.To)
	return it, nil
}

// ItemActions 返回某状态下允许的动作（前端按钮组渲染）。
func (s *Service) ItemActions(status string) []string { return itemMachine.Actions(status) }

// ------------------------- 用户侧服务目录 -------------------------

// UserCategoryTree 返回用户侧分类树：仅保留含 published 服务项的分类及其祖先。
func (s *Service) UserCategoryTree(ctx context.Context) ([]CategoryNode, error) {
	if s.categories == nil || s.items == nil {
		return nil, httpx.ErrInternal("服务目录仓储未初始化")
	}
	cats, err := s.categories.List(ctx)
	if err != nil {
		return nil, httpx.ErrInternal(err.Error())
	}
	published, err := s.items.ListPublished(ctx, nil, "")
	if err != nil {
		return nil, httpx.ErrInternal(err.Error())
	}
	counts := make(map[uint64]int, len(cats))
	keep := make(map[uint64]bool)
	for _, it := range published {
		counts[it.CategoryID]++
		keep[it.CategoryID] = true
	}
	nodes := buildCategoryNodes(cats, keep)
	applyItemCounts(nodes, counts)
	return nodes, nil
}

// UserItems 返回用户侧服务项（仅 published，支持分类与关键字）。
func (s *Service) UserItems(ctx context.Context, categoryID *uint64, keyword string) ([]ServiceItem, error) {
	if s.items == nil {
		return nil, httpx.ErrInternal("服务项仓储未初始化")
	}
	items, err := s.items.ListPublished(ctx, categoryID, keyword)
	if err != nil {
		return nil, httpx.ErrInternal(err.Error())
	}
	if items == nil {
		items = []ServiceItem{}
	}
	return items, nil
}

// OrderItem 用户下单：校验 published 与动态表单，快照 form_data，转工单。
func (s *Service) OrderItem(ctx context.Context, op Operator, itemID uint64, req OrderRequest) (*OrderResult, error) {
	if s.items == nil {
		return nil, httpx.ErrInternal("服务项仓储未初始化")
	}
	if s.tickets == nil {
		return nil, httpx.ErrInternal("工单服务未装配")
	}
	it, err := s.items.GetByID(ctx, itemID)
	if err != nil {
		return nil, mapRepoErr(err, "服务项不存在")
	}
	if it.Status != StatusPublished {
		return nil, httpx.ErrConflict("服务项未发布，不可下单")
	}
	fs, err := ParseFormSchema(it.FormSchema)
	if err != nil {
		return nil, err
	}
	if err := fs.Validate(req.FormData); err != nil {
		return nil, err
	}
	snapshot, err := json.Marshal(req.FormData)
	if err != nil {
		return nil, httpx.ErrInternal("表单数据序列化失败: " + err.Error())
	}
	if req.FormData == nil {
		snapshot = []byte("{}")
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = it.Name
	}
	categoryID := it.CategoryID
	ticketID, err := s.tickets.CreateFromServiceItem(ctx, TicketFromServiceItem{
		Title:            title,
		Description:      it.Description,
		RequesterID:      op.ID,
		CategoryID:       &categoryID,
		Priority:         defaultPriority(it.DefaultPriority),
		SLAPolicyID:      it.SLAPolicyID,
		ServiceItemID:    it.ID,
		FormData:         string(snapshot),
		RequiresApproval: it.RequiresApproval,
	})
	if err != nil {
		return nil, err
	}
	s.auditWrite(ctx, op, "order", bizTypeServiceItem, it.ID, it.Status, it.Status)
	return &OrderResult{TicketID: ticketID}, nil
}

// ------------------------- helpers -------------------------

// bizTypeCategoryAudit 是分类审计的 biz_type。
const bizTypeCategoryAudit = "service_category"

// defaultPriority 归一化优先级（缺省 P4）。
func defaultPriority(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "P4"
	}
	return p
}

// auditWrite 写入审计（best-effort，失败仅告警）。
func (s *Service) auditWrite(ctx context.Context, op Operator, action, bizType string, bizID uint64, from, to string) {
	if s.audit == nil {
		return
	}
	if err := s.audit.AppendAudit(ctx, platform.AuditEntry{
		ActorID:    op.ID,
		Action:     action,
		BizType:    bizType,
		BizID:      bizID,
		FromStatus: from,
		ToStatus:   to,
		ClientIP:   op.ClientIP,
	}); err != nil {
		s.log.Warn("写入审计日志失败", zap.String("biz_type", bizType), zap.Uint64("biz_id", bizID), zap.Error(err))
	}
}

// mapRepoErr 把仓储哨兵错误映射为 404 或 500。
func mapRepoErr(err error, notFoundMsg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) {
		return httpx.ErrNotFound(notFoundMsg)
	}
	var appErr *httpx.AppError
	if errors.As(err, &appErr) {
		return err
	}
	return httpx.ErrInternal(err.Error())
}

// buildCategoryNodes 依据扁平分类列表构建树；keep 为 nil 时保留全部分类，
// 否则仅保留 keep[id]=true 或其子孙被保留的分类。
func buildCategoryNodes(cats []ServiceCategory, keep map[uint64]bool) []CategoryNode {
	children := make(map[uint64][]ServiceCategory, len(cats))
	for _, c := range cats {
		var pid uint64
		if c.ParentID != nil {
			pid = *c.ParentID
		}
		children[pid] = append(children[pid], c)
	}
	var build func(parentID uint64) []CategoryNode
	build = func(parentID uint64) []CategoryNode {
		out := make([]CategoryNode, 0, len(children[parentID]))
		for _, c := range children[parentID] {
			kids := build(c.ID)
			if keep != nil && !keep[c.ID] && len(kids) == 0 {
				continue
			}
			out = append(out, CategoryNode{
				ID:        c.ID,
				Name:      c.Name,
				ParentID:  c.ParentID,
				SortOrder: c.SortOrder,
				Children:  kids,
			})
		}
		return out
	}
	return build(0)
}

// applyItemCounts 递归填充每个分类节点的 ItemCount。
func applyItemCounts(nodes []CategoryNode, counts map[uint64]int) {
	for i := range nodes {
		nodes[i].ItemCount = counts[nodes[i].ID]
		applyItemCounts(nodes[i].Children, counts)
	}
}
