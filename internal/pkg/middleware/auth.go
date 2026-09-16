package middleware

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/security"
)

// actorKey 是 gin context 中操作者的键。
const actorKey = "itsm.actor"

// Actor 是当前操作者（鉴权后注入 context）。
type Actor struct {
	UserID   uint64
	Username string
	Role     string
	ClientIP string
}

// SetActor 将操作者写入 context。
func SetActor(c *gin.Context, a Actor) { c.Set(actorKey, a) }

// GetActor 从 context 读取操作者。
func GetActor(c *gin.Context) (Actor, bool) {
	v, ok := c.Get(actorKey)
	if !ok {
		return Actor{}, false
	}
	a, ok := v.(Actor)
	return a, ok
}

// Auth 解析 Bearer JWT 并注入 Actor；解析失败返回 401。
//
// skipPaths 为免鉴权路径白名单（如 /auth/login、/healthz）。
func Auth(v *security.JWTManager, skipPaths ...string) gin.HandlerFunc {
	skip := make(map[string]bool, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = true
	}

	return func(c *gin.Context) {
		if skip[c.Request.URL.Path] {
			c.Next()
			return
		}

		token := extractBearer(c.GetHeader("Authorization"))
		if token == "" {
			httpx.Fail(c, httpx.ErrUnauthorized("未登录或缺少 Authorization 头"))
			return
		}
		if v == nil {
			httpx.Fail(c, httpx.ErrInternal("鉴权组件未初始化"))
			return
		}

		claims, err := v.Parse(token)
		if err != nil {
			msg := "token 无效"
			if errors.Is(err, security.ErrExpiredToken) {
				msg = "登录已过期，请重新登录"
			}
			httpx.Fail(c, httpx.ErrUnauthorized(msg))
			return
		}

		SetActor(c, Actor{
			UserID:   claims.UserID,
			Username: claims.Username,
			Role:     claims.Role,
			ClientIP: c.ClientIP(),
		})
		c.Next()
	}
}

// extractBearer 从 Authorization 头提取 Bearer token。
func extractBearer(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}
