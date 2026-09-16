package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

// RequirePerm 校验当前操作者是否拥有指定权限点；越权返回 403。
//
// 需在 Auth 中间件之后使用。
func RequirePerm(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := GetActor(c)
		if !ok {
			httpx.Fail(c, httpx.ErrUnauthorized("未登录"))
			return
		}
		if !role.Has(a.Role, perm) {
			httpx.Fail(c, httpx.ErrForbidden("无权执行该操作"))
			return
		}
		c.Next()
	}
}

// RequireRole 校验当前操作者角色是否在允许集合内；越权返回 403。
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *gin.Context) {
		a, ok := GetActor(c)
		if !ok {
			httpx.Fail(c, httpx.ErrUnauthorized("未登录"))
			return
		}
		if !allowed[a.Role] {
			httpx.Fail(c, httpx.ErrForbidden("无权执行该操作"))
			return
		}
		c.Next()
	}
}
