// 本文件承载 problem 域的 HTTP 处理。
package problem

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
)

// Handler 承载 problem 域 HTTP 处理。
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler 构造 problem handler。
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	if log == nil {
		log = zap.NewNop()
	}
	return &Handler{svc: svc, log: log}
}

// List 处理 GET /problems。
func (h *Handler) List(c *gin.Context) {
	page := httpx.ParsePage(c)
	q := ListQuery{
		Status:  c.Query("status"),
		Keyword: c.Query("keyword"),
		Offset:  page.Offset(),
		Limit:   page.PageSize,
		SortBy:  page.SortBy,
		Order:   page.Order,
	}
	if v := strings.TrimSpace(c.Query("assignee_id")); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			q.AssigneeID = &n
		}
	}
	if v := strings.TrimSpace(c.Query("known_error")); v != "" {
		b := v == "true" || v == "1"
		q.KnownError = &b
	}
	items, total, err := h.svc.List(c.Request.Context(), actorOf(c), q)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPageResult(items, total, page.Page, page.PageSize))
}

// Create 处理 POST /problems。
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	p, err := h.svc.Create(c.Request.Context(), actorOf(c), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, p)
}

// Get 处理 GET /problems/:id。
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	d, err := h.svc.Detail(c.Request.Context(), actorOf(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, d)
}

// Update 处理 PUT /problems/:id。
func (h *Handler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req UpdateRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	p, err := h.svc.Update(c.Request.Context(), actorOf(c), id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, p)
}

// Delete 处理 DELETE /problems/:id。
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

// Transition 处理 POST /problems/:id/transition。
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
	p, err := h.svc.Transition(c.Request.Context(), actorOf(c), id, req.Action, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, p)
}

// MarkKnownError 处理 POST /problems/:id/known-error。
func (h *Handler) MarkKnownError(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req KnownErrorRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	p, err := h.svc.MarkKnownError(c.Request.Context(), actorOf(c), id, req.RootCause, req.Workaround)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, p)
}

// AttachChanges 处理 POST /problems/:id/changes。
func (h *Handler) AttachChanges(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req AttachChangesRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.AddChanges(c.Request.Context(), actorOf(c), id, req.ChangeIDs); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// DetachChange 处理 DELETE /problems/:id/changes/:changeId。
func (h *Handler) DetachChange(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	changeID, err := strconv.ParseUint(c.Param("changeId"), 10, 64)
	if err != nil || changeID == 0 {
		httpx.Fail(c, httpx.ErrBadRequest("非法的路径参数 changeId"))
		return
	}
	if err := h.svc.RemoveChange(c.Request.Context(), actorOf(c), id, changeID); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// AggregateSuggestions 处理 GET /problems/aggregate-suggestions。
func (h *Handler) AggregateSuggestions(c *gin.Context) {
	var ciID uint64
	if v := strings.TrimSpace(c.Query("ci_id")); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			ciID = n
		}
	}
	days := 30
	if v := strings.TrimSpace(c.Query("days")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = n
		}
	}
	items, err := h.svc.AggregateSuggestions(c.Request.Context(), ciID, c.Query("keyword"), days)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ---------------- helpers ----------------

func parseID(c *gin.Context) (uint64, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, httpx.ErrBadRequest("非法的路径参数 id")
	}
	return id, nil
}

func actorOf(c *gin.Context) Actor {
	a, _ := middleware.GetActor(c)
	return Actor{UserID: a.UserID, Role: a.Role, ClientIP: c.ClientIP()}
}
