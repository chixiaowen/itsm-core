package asset

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
)

// Handler 承载 asset 域的 HTTP 处理。
type Handler struct {
	svc *Service
	log *zap.Logger
}

// NewHandler 构造 asset handler。
func NewHandler(svc *Service, log *zap.Logger) *Handler {
	if log == nil {
		log = zap.NewNop()
	}
	return &Handler{svc: svc, log: log}
}

// ListAssets 处理 GET /assets。
func (h *Handler) ListAssets(c *gin.Context) {
	page := httpx.ParsePage(c)
	q := AssetListQuery{
		Category: c.Query("category"),
		Status:   c.Query("status"),
		Offset:   page.Offset(),
		Limit:    page.PageSize,
	}
	if v := c.Query("user_id"); v != "" {
		if id, err := strconv.ParseUint(v, 10, 64); err == nil && id > 0 {
			q.UserID = &id
		}
	}
	if v := c.Query("warranty_before"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q.WarrantyBefore = &t
		}
	}
	items, total, err := h.svc.ListAssets(c.Request.Context(), q)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.NewPageResult(items, total, page.Page, page.PageSize))
}

// CreateAsset 处理 POST /assets。
func (h *Handler) CreateAsset(c *gin.Context) {
	var req AssetRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	a, err := h.svc.CreateAsset(c.Request.Context(), actorOf(c), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, a)
}

// GetAsset 处理 GET /assets/:id。
func (h *Handler) GetAsset(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	detail, err := h.svc.GetAssetDetail(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, detail)
}

// UpdateAsset 处理 PUT /assets/:id。
func (h *Handler) UpdateAsset(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req AssetRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	a, err := h.svc.UpdateAsset(c.Request.Context(), actorOf(c), id, req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, a)
}

// DeleteAsset 处理 DELETE /assets/:id。
func (h *Handler) DeleteAsset(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteAsset(c.Request.Context(), actorOf(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// Transition 处理 POST /assets/:id/transition。
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
	a, err := h.svc.TransitionAsset(c.Request.Context(), actorOf(c), id, req.Action, TransitionParams{
		Remark:       req.Remark,
		Reason:       req.Reason,
		UserID:       req.UserID,
		Location:     req.Location,
		CIID:         req.CIID,
		PurchaseDate: req.PurchaseDate,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, a)
}

// ListHistory 处理 GET /assets/:id/history。
func (h *Handler) ListHistory(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, err := h.svc.ListHistory(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, items)
}

// BindCI 处理 POST /assets/:id/bind-ci。
func (h *Handler) BindCI(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req BindCIRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	a, err := h.svc.BindCI(c.Request.Context(), actorOf(c), id, req.CIID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, a)
}

// UnbindCI 处理 DELETE /assets/:id/bind-ci。
func (h *Handler) UnbindCI(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	a, err := h.svc.UnbindCI(c.Request.Context(), actorOf(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, a)
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
