package platform

import (
	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

// Register 将 platform 域路由挂载到已鉴权的路由组 rg（rg 应已挂 Auth 中间件）。
func Register(rg *gin.RouterGroup, h *Handler) {
	userAdmin := middleware.RequirePerm(role.PermUserManage)
	// 候选用户下拉：任意已登录用户可访问，故不挂 RequirePerm；
	// 必须注册在 /users/:id 之前，避免静态路径被通配路由吞掉。
	rg.GET("/users/options", h.ListUserOptions)
	rg.GET("/users", userAdmin, h.ListUsers)
	rg.POST("/users", userAdmin, h.CreateUser)
	rg.GET("/users/:id", userAdmin, h.GetUser)
	rg.PUT("/users/:id", userAdmin, h.UpdateUser)
	rg.DELETE("/users/:id", userAdmin, h.DeleteUser)
	rg.GET("/roles", userAdmin, h.ListRoles)

	slaAdmin := middleware.RequirePerm(role.PermSlaManage)
	rg.GET("/sla-policies", slaAdmin, h.ListSLAPolicies)
	rg.POST("/sla-policies", slaAdmin, h.CreateSLAPolicy)
	rg.PUT("/sla-policies/:id", slaAdmin, h.UpdateSLAPolicy)
	rg.DELETE("/sla-policies/:id", slaAdmin, h.DeleteSLAPolicy)

	rg.GET("/audit-logs", auditAccess(), h.ListAuditLogs)

	rg.GET("/comments", h.ListComments)
	rg.POST("/comments", h.CreateComment)

	rg.POST("/attachments", h.UploadAttachment)
	rg.GET("/attachments/:id/download", h.DownloadAttachment)
	rg.DELETE("/attachments/:id", h.DeleteAttachment)
}

// auditAccess 是审计读取的访问控制（platform 包内局部中间件，不改动 middleware 包）：
//   - 带实体范围的查询（entity_type/biz_type 与 entity_id/biz_id 必须同时出现）
//     → 任意已登录用户可读：实体自身的状态流转历史属于该实体的可见性范围，与详情接口同级；
//   - 不带实体范围的全局审计浏览 → 仍要求 perm.audit.view（当前仅 admin 拥有）。
//
// 安全红线：实体范围必须「类型 + ID 同时提供」（hasType && hasID），
// 否则任何登录用户仅传 entity_type=ticket 即可枚举全量审计流。
func auditAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		hasType := c.Query("entity_type") != "" || c.Query("biz_type") != ""
		hasID := c.Query("entity_id") != "" || c.Query("biz_id") != ""
		if hasType && hasID {
			c.Next()
			return
		}
		actor, ok := middleware.GetActor(c)
		if !ok || !role.Has(actor.Role, role.PermAuditView) {
			httpx.Fail(c, httpx.ErrForbidden("无权查看全局审计日志"))
			c.Abort()
			return
		}
		c.Next()
	}
}
