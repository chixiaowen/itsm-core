package catalog

import (
	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

// Register 将 catalog 域路由挂载到已鉴权的路由组 rg（rg 应已挂 Auth 中间件）。
func Register(rg *gin.RouterGroup, h *Handler) {
	manage := middleware.RequirePerm(role.PermCatalogManage)

	// 服务分类树（管理台）。
	rg.GET("/service-categories", manage, h.ListCategoryTree)
	rg.POST("/service-categories", manage, h.CreateCategory)
	rg.PUT("/service-categories/:id", manage, h.UpdateCategory)
	rg.DELETE("/service-categories/:id", manage, h.DeleteCategory)

	// 服务项（管理台）。
	rg.GET("/service-items", manage, h.ListItems)
	rg.POST("/service-items", manage, h.CreateItem)
	rg.PUT("/service-items/:id", manage, h.UpdateItem)
	rg.DELETE("/service-items/:id", manage, h.DeleteItem)
	rg.POST("/service-items/:id/publish", manage, h.PublishItem)
	rg.POST("/service-items/:id/offline", manage, h.OfflineItem)
	rg.POST("/service-items/:id/transition", manage, h.TransitionItem)
	// 详情登录可读。
	rg.GET("/service-items/:id", h.GetItem)

	// 用户侧服务目录（含下单）。
	order := middleware.RequirePerm(role.PermCatalogOrder)
	rg.GET("/catalog/categories", order, h.UserCategories)
	rg.GET("/catalog/items", order, h.UserItems)
	rg.POST("/catalog/items/:id/order", order, h.OrderItem)
}
