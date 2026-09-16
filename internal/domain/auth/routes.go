package auth

import "github.com/gin-gonic/gin"

// Register 在公开路由组上注册免鉴权路由（登录）。
func Register(public *gin.RouterGroup, h *Handler) {
	public.POST("/auth/login", h.Login)
}

// RegisterProtected 在已鉴权路由组上注册需登录的路由。
func RegisterProtected(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/auth/me", h.Me)
	rg.POST("/auth/logout", h.Logout)
}
