// 本文件承载 change 域的 HTTP 处理。
package change

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
)

// Handler 承载 change 域 HTTP 处理。
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler 构造 change handler。
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	if log == nil {
		log = zap.NewNop()
	}
	return &Handler{svc: svc, log: log}
}

// List 处理 GET /changes。
func (h *Handler) List(c *gin.Context) {
	page := httpx.ParsePage(c)
	q := ListQuery{
		Status:     c.Query("status"),
		ChangeType: c.Query("change_type"),
		RiskLevel:  c.Query("risk_level"),
		Keyword:    c.Query("keyword"),
		Offset:     page.Offset(),
		Limit:      page.PageSize,
		SortBy:     page.SortBy,
		Order:      page.Order,
	}
	if v := strings.TrimSpace(c.Query("manager_id")); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			q.ManagerID = &n
		}
	}
	q.WindowFrom = timePtr(c.Query("window_from"))
	q.WindowTo = timePtr(c.Query("window_to"))

	items, total, err := h.svc.List(c.Request.Context(), actorOf(c), q)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPageResult(items, total, page.Page, page.PageSize))
}

// Create 处理 POST /changes。
func (h *Handler) Create(c *gin.Context) {
	var req CreateRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	ch, err := h.svc.Create(c.Request.Context(), actorOf(c), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, ch)
}

// Get 处理 GET /changes/:id。
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

// Update 处理 PUT /changes/:id。
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
	ch, err := h.svc.Update(c.Request.Context(), actorOf(c), id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, ch)
}

// Delete 处理 DELETE /changes/:id。
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

// Transition 处理 POST /changes/:id/transition。
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
	ch, err := h.svc.Transition(c.Request.Context(), actorOf(c), id, req.Action, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, ch)
}

// RecordApproval 处理 POST /changes/:id/approvals。
func (h *Handler) RecordApproval(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req ApprovalRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	ch, err := h.svc.RecordApproval(c.Request.Context(), actorOf(c), id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, ch)
}

// ListApprovals 处理 GET /changes/:id/approvals。
func (h *Handler) ListApprovals(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, err := h.svc.ListApprovals(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
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
