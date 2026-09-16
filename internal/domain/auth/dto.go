package auth

import (
	"time"

	"github.com/chixiaowen/itsm-core/internal/domain/platform"
)

// LoginRequest 是登录请求体。
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 是登录响应 data。
type LoginResponse struct {
	Token     string         `json:"token"`
	ExpiresAt time.Time      `json:"expires_at"`
	User      *platform.User `json:"user"`
}
