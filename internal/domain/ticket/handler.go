// 本文件承载 ticket 域的 HTTP 处理：参数绑定、调用 service、渲染统一响应。
//
// 铁律：handler 不得出现 *gorm.DB，不做业务判断。
package ticket

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
)

// Handler 承载 ticket 域 HTTP 处理。
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler 构造 ticket handler。
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	if log == nil {
		log = zap.NewNop()
	}
	return &Handler{svc: svc, log: log}
}

// List 处理 GET /tickets。
func (h *Handler) List(c *gin.Context) {
	page := httpx.ParsePage(c)
	q := ListQuery{
		Status:    c.Query("status"),
		Priority:  c.Query("priority"),
		Type:      c.Query("type"),
		SLAStatus: c.Query("sla_status"),
		Keyword:   c.Query("keyword"),
		Offset:    page.Offset(),
		Limit:     page.PageSize,
		SortBy:    page.SortBy,
		Order:     page.Order,
	}
	q.CategoryID = uintPtr(c.Query("category_id"))
	q.AssigneeID = uintPtr(c.Query("assignee_id"))
	q.RequesterID = uintPtr(c.Query("requester_id"))
	q.From = timePtr(c.Query("from"))
	q.To = timePtr(c.Query("to"))

	items, total, err := h.svc.List(c.Request.Context(), actorOf(c), q)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPageResult(items, total, page.Page, page.PageSize))
}

// Create 处理 POST /tickets。
func (h *Handler) Create(c *gin.Context) {
	var req CreateTicketRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	tk, err := h.svc.Create(c.Request.Context(), actorOf(c), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, tk)
}

// Get 处理 GET /tickets/:id。
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	includeInternal := c.Query("include_internal") == "true"
	d, err := h.svc.Detail(c.Request.Context(), actorOf(c), id, includeInternal)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, d)
}

// Update 处理 PUT /tickets/:id。
func (h *Handler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req UpdateTicketRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	tk, err := h.svc.Update(c.Request.Context(), actorOf(c), id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, tk)
}

// Delete 处理 DELETE /tickets/:id。
func (h *Handler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.Delete(c.Request.Context(), actorOf(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// Transition 处理 POST /tickets/:id/transition。
func (h *Handler) Transition(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req TransitionRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	tk, err := h.svc.Transition(c.Request.Context(), actorOf(c), id, req.Action, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, tk)
}

// Assign 处理 POST /tickets/:id/assign。
func (h *Handler) Assign(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req AssignRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	tk, err := h.svc.Assign(c.Request.Context(), actorOf(c), id, req.AssigneeID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, tk)
}

// Rating 处理 POST /tickets/:id/rating。
func (h *Handler) Rating(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req RatingRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	tk, err := h.svc.Rate(c.Request.Context(), actorOf(c), id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, tk)
}

// Comment 处理 POST /tickets/:id/comment。
func (h *Handler) Comment(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req TicketCommentRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	tk, err := h.svc.AddComment(c.Request.Context(), actorOf(c), id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, tk)
}

// AttachCIs 处理 POST /tickets/:id/cis。
func (h *Handler) AttachCIs(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req AttachCIsRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.AddCIs(c.Request.Context(), actorOf(c), id, req.CIIDs); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// DetachCI 处理 DELETE /tickets/:id/cis/:ciId。
func (h *Handler) DetachCI(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ciID, err := strconv.ParseUint(c.Param("ciId"), 10, 64)
	if err != nil || ciID == 0 {
		httpx.Fail(c, httpx.ErrBadRequest("非法的路径参数 ciId"))
		return
	}
	if err := h.svc.RemoveCI(c.Request.Context(), actorOf(c), id, ciID); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// ListCategories 处理 GET /ticket-categories。
func (h *Handler) ListCategories(c *gin.Context) {
	items, err := h.svc.ListCategories(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

// CreateCategory 处理 POST /ticket-categories。
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

// UpdateCategory 处理 PUT /ticket-categories/:id。
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

// DeleteCategory 处理 DELETE /ticket-categories/:id。
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

// ---------------- helpers ----------------

// parseID 解析路径参数 id。
func parseID(c *gin.Context) (uint64, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, httpx.ErrBadRequest("非法的路径参数 id")
	}
	return id, nil
}

// actorOf 从 context 提取操作者。
func actorOf(c *gin.Context) Actor {
	a, _ := middleware.GetActor(c)
	return Actor{UserID: a.UserID, Role: a.Role, ClientIP: c.ClientIP()}
}

// uintPtr 将 query 字符串解析为 *uint64（空或非法返回 nil）。
func uintPtr(s string) *uint64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

// timePtr 将 RFC3339 query 字符串解析为 *time.Time（空或非法返回 nil）。
func timePtr(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}
