package middleware

import (
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
)

// Recovery 捕获 panic，记录堆栈并返回 500 统一响应。
func Recovery(z *zap.Logger) gin.HandlerFunc {
	if z == nil {
		z = zap.NewNop()
	}
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				z.Error("panic 已恢复",
					zap.Any("panic", r),
					zap.String("path", c.Request.URL.Path),
					zap.String("request_id", RequestIDOf(c)),
					zap.ByteString("stack", debug.Stack()),
				)
				if !c.Writer.Written() {
					httpx.Fail(c, httpx.ErrInternal("服务器内部错误"))
					return
				}
				c.Abort()
			}
		}()
		c.Next()
	}
}
