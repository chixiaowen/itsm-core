// 本文件注册 problem 域路由与权限点。
package problem

import (
	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

// Register 将 problem 域路由挂载到已鉴权的路由组 rg。
//
// 注意：静态路由（aggregate-suggestions）先于参数路由注册，避免路由冲突。
// transition 不做路由级角色限制，由状态机（machine.go）按流转逐条校验角色。
func Register(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/problems/aggregate-suggestions", middleware.RequireRole(role.ProblemManager, role.Admin), h.AggregateSuggestions)

	rg.GET("/problems", middleware.RequireRole(role.ProblemManager, role.Resolver, role.Admin), h.List)
	rg.POST("/problems", middleware.RequirePerm(role.PermProblemCreate), h.Create)
	rg.GET("/problems/:id", h.Get)
	rg.PUT("/problems/:id", middleware.RequirePerm(role.PermProblemRCA), h.Update)
	rg.DELETE("/problems/:id", middleware.RequireRole(role.Admin), h.Delete)

	rg.POST("/problems/:id/transition", h.Transition)
	rg.POST("/problems/:id/known-error", middleware.RequirePerm(role.PermProblemRCA), h.MarkKnownError)
	rg.POST("/problems/:id/changes", middleware.RequirePerm(role.PermProblemRCA), h.AttachChanges)
	rg.DELETE("/problems/:id/changes/:changeId", middleware.RequirePerm(role.PermProblemRCA), h.DetachChange)
}
