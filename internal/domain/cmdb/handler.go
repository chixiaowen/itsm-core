package cmdb

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
)

// Handler 承载 cmdb 域的 HTTP 处理。
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler 构造 cmdb handler。
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	if log == nil {
		log = zap.NewNop()
	}
	return &Handler{svc: svc, log: log}
}

// ListCIs 处理 GET /cis。
func (h *Handler) ListCIs(c *gin.Context) {
	page := httpx.ParsePage(c)
	q := CIListQuery{
		CIType:  c.Query("ci_type"),
		Status:  c.Query("status"),
		Keyword: c.Query("keyword"),
		Offset:  page.Offset(),
		Limit:   page.PageSize,
	}
	if v := c.Query("owner_id"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 64); err == nil && id > 0 {
			q.OwnerID = &id
		}
	}
	items, total, err := h.svc.ListCIs(c.Request.Context(), q)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPageResult(items, total, page.Page, page.PageSize))
}

// CreateCI 处理 POST /cis。
func (h *Handler) CreateCI(c *gin.Context) {
	var req CIRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	ci, err := h.svc.CreateCI(c.Request.Context(), actorOf(c), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, ci)
}

// GetCIDetail 处理 GET /cis/:id。
func (h *Handler) GetCIDetail(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	detail, err := h.svc.GetCIDetail(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, detail)
}

// UpdateCI 处理 PUT /cis/:id。
func (h *Handler) UpdateCI(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req CIRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	ci, err := h.svc.UpdateCI(c.Request.Context(), actorOf(c), id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, ci)
}

// DeleteCI 处理 DELETE /cis/:id。
func (h *Handler) DeleteCI(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteCI(c.Request.Context(), actorOf(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// Topology 处理 GET /cis/:id/topology。
func (h *Handler) Topology(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	depth := 0
	if v := c.Query("depth"); v != "" {
		if n, perr := strconv.Atoi(v); perr == nil {
			depth = n
		}
	}
	direction := c.Query("direction")
	g, err := h.svc.Topology(c.Request.Context(), id, depth, direction)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, g)
}

// AddRelation 处理 POST /cis/:id/relations。
func (h *Handler) AddRelation(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req RelationRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	rel, err := h.svc.AddRelation(c.Request.Context(), actorOf(c), id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, rel)
}

// DeleteRelation 处理 DELETE /cis/:id/relations/:relId。
func (h *Handler) DeleteRelation(c *gin.Context) {
	ciID, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	relID, err := strconv.ParseUint(c.Param("relId"), 10, 64)
	if err != nil || relID == 0 {
		httpx.Fail(c, httpx.ErrBadRequest("非法的路径参数 relId"))
		return
	}
	if err := h.svc.DeleteRelation(c.Request.Context(), actorOf(c), ciID, relID); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// ListCITypes 处理 GET /ci-types。
func (h *Handler) ListCITypes(c *gin.Context) {
	httpx.OK(c, gin.H{"ci_types": CITypes(), "relation_types": RelationTypes()})
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
