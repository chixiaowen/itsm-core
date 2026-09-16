// Package middleware 提供与业务无关的 Gin 中间件：
// requestID、访问日志、panic 恢复、CORS、JWT 鉴权、RBAC。
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDHeader 是透传/返回的请求 ID 头。
const RequestIDHeader = "X-Request-ID"

// requestIDKey 是 gin context 中 request id 的键。
const requestIDKey = "itsm.request_id"

// RequestID 读取或生成 X-Request-ID，写入 context 与响应头。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := strings.TrimSpace(c.GetHeader(RequestIDHeader))
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Set(requestIDKey, rid)
		c.Writer.Header().Set(RequestIDHeader, rid)
		c.Next()
	}
}

// RequestIDOf 返回当前请求的 request id（不存在时返回空串）。
func RequestIDOf(c *gin.Context) string {
	if v, ok := c.Get(requestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
