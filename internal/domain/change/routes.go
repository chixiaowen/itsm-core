// 本文件注册 change 域路由与权限点。
package change

import (
	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

// Register 将 change 域路由挂载到已鉴权的路由组 rg。
func Register(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/changes", h.List)
	rg.POST("/changes", middleware.RequirePerm(role.PermChangeSubmit), h.Create)
	rg.GET("/changes/:id", h.Get)
	rg.PUT("/changes/:id", middleware.RequirePerm(role.PermChangeSubmit), h.Update)
	rg.DELETE("/changes/:id", middleware.RequireRole(role.Admin), h.Delete)

	rg.POST("/changes/:id/transition", h.Transition)
	rg.POST("/changes/:id/approvals", middleware.RequirePerm(role.PermChangeApprove), h.RecordApproval)
	rg.GET("/changes/:id/approvals", h.ListApprovals)
}
