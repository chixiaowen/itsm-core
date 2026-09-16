package cmdb

import (
	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

// Register 将 cmdb 域路由挂载到已鉴权的路由组 rg（rg 应已挂 Auth 中间件）。
//
// 管理类接口使用权限点 perm.cmdb.manage（矩阵授予 cmdb_manager/admin，以及
// resolver/problem_manager 只读以外的管理能力，见 role.go）；查询类登录可读。
func Register(rg *gin.RouterGroup, h *Handler) {
	manage := middleware.RequirePerm(role.PermCmdbManage)

	rg.GET("/cis", manage, h.ListCIs)
	rg.POST("/cis", manage, h.CreateCI)
	rg.PUT("/cis/:id", manage, h.UpdateCI)
	rg.DELETE("/cis/:id", manage, h.DeleteCI)
	rg.POST("/cis/:id/relations", manage, h.AddRelation)
	rg.DELETE("/cis/:id/relations/:relId", manage, h.DeleteRelation)

	// 只读接口：登录即可。
	rg.GET("/cis/:id", h.GetCIDetail)
	rg.GET("/cis/:id/topology", h.Topology)
	rg.GET("/ci-types", h.ListCITypes)
}
