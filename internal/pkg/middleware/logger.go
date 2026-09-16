package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Logger 记录结构化访问日志：method/path/status/latency/request_id/actor_id/client_ip。
func Logger(z *zap.Logger) gin.HandlerFunc {
	if z == nil {
		z = zap.NewNop()
	}
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("request_id", RequestIDOf(c)),
			zap.String("client_ip", c.ClientIP()),
		}
		if a, ok := GetActor(c); ok {
			fields = append(fields, zap.Uint64("actor_id", a.UserID), zap.String("role", a.Role))
		}

		switch {
		case c.Writer.Status() >= 500:
			z.Error("http 请求", fields...)
		case c.Writer.Status() >= 400:
			z.Warn("http 请求", fields...)
		default:
			z.Info("http 请求", fields...)
		}
	}
}
