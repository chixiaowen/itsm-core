// 本文件注册 ticket 域路由与权限点（rg 应已挂 Auth 中间件）。
package ticket

import (
	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

// Register 将 ticket 域路由挂载到已鉴权的路由组 rg。
func Register(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/tickets", h.List)
	rg.POST("/tickets", middleware.RequirePerm(role.PermTicketCreate), h.Create)
	rg.GET("/tickets/:id", h.Get)
	rg.PUT("/tickets/:id", h.Update)
	rg.DELETE("/tickets/:id", middleware.RequireRole(role.Admin), h.Delete)

	rg.POST("/tickets/:id/transition", h.Transition)
	rg.POST("/tickets/:id/assign", middleware.RequirePerm(role.PermTicketAssign), h.Assign)
	rg.POST("/tickets/:id/rating", middleware.RequirePerm(role.PermTicketRate), h.Rating)
	rg.POST("/tickets/:id/comment", h.Comment)
	rg.POST("/tickets/:id/cis", middleware.RequirePerm(role.PermTicketHandle), h.AttachCIs)
	rg.DELETE("/tickets/:id/cis/:ciId", middleware.RequirePerm(role.PermTicketHandle), h.DetachCI)

	rg.GET("/ticket-categories", h.ListCategories)
	rg.POST("/ticket-categories", middleware.RequireRole(role.Admin), h.CreateCategory)
	rg.PUT("/ticket-categories/:id", middleware.RequireRole(role.Admin), h.UpdateCategory)
	rg.DELETE("/ticket-categories/:id", middleware.RequireRole(role.Admin), h.DeleteCategory)
}
