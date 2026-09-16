package catalog

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
)

// Handler 承载 catalog 域的 HTTP 处理。
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler 构造 catalog handler。
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	if log == nil {
		log = zap.NewNop()
	}
	return &Handler{svc: svc, log: log}
}

// ------------------------- 服务分类 -------------------------

// ListCategoryTree 处理 GET /service-categories。
func (h *Handler) ListCategoryTree(c *gin.Context) {
	nodes, err := h.svc.ListCategoryTree(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nodes)
}

// CreateCategory 处理 POST /service-categories。
func (h *Handler) CreateCategory(c *gin.Context) {
	var req CategoryRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	cat, err := h.svc.CreateCategory(c.Request.Context(), actorOf(c), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, cat)
}

// UpdateCategory 处理 PUT /service-categories/:id。
func (h *Handler) UpdateCategory(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req CategoryRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	cat, err := h.svc.UpdateCategory(c.Request.Context(), actorOf(c), id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, cat)
}

// DeleteCategory 处理 DELETE /service-categories/:id。
func (h *Handler) DeleteCategory(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteCategory(c.Request.Context(), actorOf(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// ------------------------- 服务项（管理台） -------------------------

// ListItems 处理 GET /service-items。
func (h *Handler) ListItems(c *gin.Context) {
	page := httpx.ParsePage(c)
	q := ItemListQuery{
		Status:  c.Query("status"),
		Keyword: c.Query("keyword"),
		Offset:  page.Offset(),
		Limit:   page.PageSize,
	}
	if v := c.Query("category_id"); v != "" {
		if id, perr := strconv.ParseUint(v, 10, 64); perr == nil && id > 0 {
			q.CategoryID = &id
		}
	}
	items, total, err := h.svc.ListItems(c.Request.Context(), q)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPageResult(items, total, page.Page, page.PageSize))
}

// CreateItem 处理 POST /service-items。
func (h *Handler) CreateItem(c *gin.Context) {
	var req ItemRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	it, err := h.svc.CreateItem(c.Request.Context(), actorOf(c), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, it)
}

// GetItem 处理 GET /service-items/:id。
func (h *Handler) GetItem(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	it, err := h.svc.GetItem(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, it)
}

// UpdateItem 处理 PUT /service-items/:id。
func (h *Handler) UpdateItem(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req ItemRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	it, err := h.svc.UpdateItem(c.Request.Context(), actorOf(c), id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, it)
}

// DeleteItem 处理 DELETE /service-items/:id（归档）。
func (h *Handler) DeleteItem(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteItem(c.Request.Context(), actorOf(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// PublishItem 处理 POST /service-items/:id/publish。
func (h *Handler) PublishItem(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	it, err := h.svc.PublishItem(c.Request.Context(), actorOf(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, it)
}

// OfflineItem 处理 POST /service-items/:id/offline。
func (h *Handler) OfflineItem(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	it, err := h.svc.OfflineItem(c.Request.Context(), actorOf(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, it)
}

// TransitionItem 处理 POST /service-items/:id/transition（通用流转，如驳回）。
func (h *Handler) TransitionItem(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req ItemTransitionRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	it, err := h.svc.TransitionItem(c.Request.Context(), actorOf(c), id, req.Action, req.Reason)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, it)
}

// ------------------------- 用户侧服务目录 -------------------------

// UserCategories 处理 GET /catalog/categories（仅含 published）。
func (h *Handler) UserCategories(c *gin.Context) {
	nodes, err := h.svc.UserCategoryTree(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, nodes)
}

// UserItems 处理 GET /catalog/items（仅 published）。
func (h *Handler) UserItems(c *gin.Context) {
	var categoryID *uint64
	if v := c.Query("category_id"); v != "" {
		if id, perr := strconv.ParseUint(v, 10, 64); perr == nil && id > 0 {
			categoryID = &id
		}
	}
	items, err := h.svc.UserItems(c.Request.Context(), categoryID, c.Query("keyword"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

// OrderItem 处理 POST /catalog/items/:id/order。
func (h *Handler) OrderItem(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req OrderRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	res, err := h.svc.OrderItem(c.Request.Context(), actorOf(c), id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, res)
}

// ------------------------- helpers -------------------------

func parseID(c *gin.Context) (uint64, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, httpx.ErrBadRequest("非法的路径参数 id")
	}
	return id, nil
}

// actorOf 从 context 提取当前操作者。
func actorOf(c *gin.Context) Operator {
	a, _ := middleware.GetActor(c)
	return Operator{ID: a.UserID, Role: a.Role, ClientIP: c.ClientIP()}
}
