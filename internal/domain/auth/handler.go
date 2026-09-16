package auth

import (
	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
)

// Handler 承载 auth 域的 HTTP 处理。
type Handler struct {
	svc *Service
}

// NewHandler 构造 auth handler。
func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

// Login 处理 POST /auth/login。
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := httpx.ShouldBindJSON(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	res, err := h.svc.Login(c.Request.Context(), req)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, LoginResponse{Token: res.Token, ExpiresAt: res.ExpiresAt, User: res.User})
}

// Me 处理 GET /auth/me。
func (h *Handler) Me(c *gin.Context) {
	a, ok := middleware.GetActor(c)
	if !ok {
		httpx.Fail(c, httpx.ErrUnauthorized("未登录"))
		return
	}
	u, err := h.svc.Me(c.Request.Context(), a.UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, u)
}

// Logout 处理 POST /auth/logout（无状态，前端清除 token 即可）。
func (h *Handler) Logout(c *gin.Context) {
	httpx.OK(c, nil)
}
