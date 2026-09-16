package asset

import (
	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

// Register 将 asset 域路由挂载到已鉴权的路由组 rg（rg 应已挂 Auth 中间件）。
//
// 全部接口使用权限点 perm.asset.manage（矩阵授予 cmdb_manager/admin）。
func Register(rg *gin.RouterGroup, h *Handler) {
	manage := middleware.RequirePerm(role.PermAssetManage)

	rg.GET("/assets", manage, h.ListAssets)
	rg.POST("/assets", manage, h.CreateAsset)
	rg.GET("/assets/:id", manage, h.GetAsset)
	rg.PUT("/assets/:id", manage, h.UpdateAsset)
	rg.DELETE("/assets/:id", manage, h.DeleteAsset)
	rg.POST("/assets/:id/transition", manage, h.Transition)
	rg.GET("/assets/:id/history", manage, h.ListHistory)
	rg.POST("/assets/:id/bind-ci", manage, h.BindCI)
	rg.DELETE("/assets/:id/bind-ci", manage, h.UnbindCI)
}
