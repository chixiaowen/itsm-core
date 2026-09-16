// 本文件承载 incident 域的 HTTP 处理。
package incident

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
)

// Handler 承载 incident 域 HTTP 处理。
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler 构造 incident handler。
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	if log == nil {
		log = zap.NewNop()
	}
	return &Handler{svc: svc, log: log}
}

// List 处理 GET /incidents。
func (h *Handler) List(c *gin.Context) {
	page := httpx.ParsePage(c)
	q := ListQuery{
		Status:   c.Query("status"),
		Priority: c.Query("priority"),
		Impact:   c.Query("impact"),
		Urgency:  c.Query("urgency"),
		Keyword:  c.Query("keyword"),
		Offset:   page.Offset(),
		Limit:    page.PageSize,
		SortBy:   page.SortBy,
		Order:    page.Order,
	}
	if v := strings.TrimSpace(c.Query("escalation_level")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.EscalationLevel = &n
		}
	}
	q.From = timePtr(c.Query("from"))
	q.To = timePtr(c.Query("to"))

	items, total, err := h.svc.List(c.Request.Context(), actorOf(c), q)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPageResult(items, total, page.Page, page.PageSize))
}

// Report 处理 POST /incidents。
func (h *Handler) Report(c *gin.Context) {
	var req ReportRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	it, err := h.svc.Report(c.Request.Context(), actorOf(c), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, it)
}

// Get 处理 GET /incidents/:id。
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

// Update 处理 PUT /incidents/:id。
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
	it, err := h.svc.Update(c.Request.Context(), actorOf(c), id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, it)
}

// Delete 处理 DELETE /incidents/:id。
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

// Transition 处理 POST /incidents/:id/transition。
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
	it, err := h.svc.Transition(c.Request.Context(), actorOf(c), id, req.Action, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, it)
}

// OverridePriority 处理 POST /incidents/:id/priority。
func (h *Handler) OverridePriority(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req PriorityRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	it, err := h.svc.OverridePriority(c.Request.Context(), actorOf(c), id, req.Priority)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, it)
}

// Escalate 处理 POST /incidents/:id/escalate。
func (h *Handler) Escalate(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req EscalateRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	it, err := h.svc.Escalate(c.Request.Context(), actorOf(c), id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, it)
}

// ConvertToTicket 处理 POST /incidents/:id/convert-to-ticket。
func (h *Handler) ConvertToTicket(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req ConvertToTicketRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	tkID, code, err := h.svc.ConvertToTicket(c.Request.Context(), actorOf(c), id, req.Title)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, gin.H{"ticket_id": tkID, "code": code})
}

// LinkTicket 处理 POST /incidents/:id/link-ticket。
func (h *Handler) LinkTicket(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req LinkTicketRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	it, err := h.svc.LinkTicket(c.Request.Context(), actorOf(c), id, req.TicketID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, it)
}

// AttachCIs 处理 POST /incidents/:id/cis。
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

// DetachCI 处理 DELETE /incidents/:id/cis/:ciId。
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

// PriorityMatrix 处理 GET /incidents/priority-matrix。
func (h *Handler) PriorityMatrix(c *gin.Context) {
	httpx.OK(c, gin.H{
		"impacts":   []string{ImpactHigh, ImpactMedium, ImpactLow},
		"urgencies": []string{UrgencyHigh, UrgencyMed, UrgencyLow},
		"matrix":    Matrix(),
	})
}

// Stats 处理 GET /incidents/stats。
func (h *Handler) Stats(c *gin.Context) {
	byStatus, byPriority, err := h.svc.Stats(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"by_status": byStatus, "by_priority": byPriority})
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
