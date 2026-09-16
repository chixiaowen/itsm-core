// 本文件注册 incident 域路由与权限点。
package incident

import (
	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

// Register 将 incident 域路由挂载到已鉴权的路由组 rg。
//
// 注意：静态路由（priority-matrix/stats）先于参数路由注册，避免路由冲突。
func Register(rg *gin.RouterGroup, h *Handler) {
	rg.GET("/incidents/priority-matrix", h.PriorityMatrix)
	rg.GET("/incidents/stats", h.Stats)

	rg.GET("/incidents", h.List)
	rg.POST("/incidents", middleware.RequirePerm(role.PermIncidentReport), h.Report)
	rg.GET("/incidents/:id", h.Get)
	rg.PUT("/incidents/:id", middleware.RequireRole(role.Agent, role.Resolver, role.ProblemManager, role.Admin), h.Update)
	rg.DELETE("/incidents/:id", middleware.RequireRole(role.Admin), h.Delete)

	rg.POST("/incidents/:id/transition", h.Transition)
	rg.POST("/incidents/:id/priority", middleware.RequireRole(role.Agent, role.Resolver, role.ProblemManager, role.Admin), h.OverridePriority)
	rg.POST("/incidents/:id/escalate", middleware.RequirePerm(role.PermIncidentEscalate), h.Escalate)
	rg.POST("/incidents/:id/convert-to-ticket", middleware.RequirePerm(role.PermIncidentConvert), h.ConvertToTicket)
	rg.POST("/incidents/:id/link-ticket", middleware.RequireRole(role.Agent, role.Resolver, role.Admin), h.LinkTicket)
	rg.POST("/incidents/:id/cis", middleware.RequireRole(role.Agent, role.Resolver, role.Admin), h.AttachCIs)
	rg.DELETE("/incidents/:id/cis/:ciId", middleware.RequireRole(role.Agent, role.Resolver, role.Admin), h.DetachCI)
}
