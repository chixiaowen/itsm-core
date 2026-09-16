// Package auth 负责登录校验、签发 JWT 与当前用户查询。
package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/chixiaowen/itsm-core/internal/domain/platform"
	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/security"
)

// Service 是认证服务。
type Service struct {
	users platform.UserRepository
	jwt   *security.JWTManager
}

// NewService 构造认证服务。
func NewService(users platform.UserRepository, jwt *security.JWTManager) *Service {
	return &Service{users: users, jwt: jwt}
}

// LoginResult 是登录结果。
type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	User      *platform.User
}

// Login 校验用户名密码并签发 JWT。
//
// 用户名不存在 / 密码错误 / 账号禁用 一律 401（不区分，避免账号枚举）。
func (s *Service) Login(ctx context.Context, req LoginRequest) (*LoginResult, error) {
	if s.users == nil || s.jwt == nil {
		return nil, httpx.ErrInternal("认证服务未正确初始化")
	}
	username := strings.TrimSpace(req.Username)
	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, platform.ErrNotFound) {
			return nil, httpx.ErrUnauthorized("用户名或密码错误")
		}
		return nil, httpx.ErrInternal(err.Error())
	}
	if u.Status != platform.UserStatusActive {
		return nil, httpx.ErrUnauthorized("账号已被禁用")
	}
	if !security.ComparePassword(u.PasswordHash, req.Password) {
		return nil, httpx.ErrUnauthorized("用户名或密码错误")
	}

	token, expiresAt, err := s.jwt.Issue(u.ID, u.Username, u.Role)
	if err != nil {
		return nil, httpx.ErrInternal("签发 token 失败: " + err.Error())
	}
	return &LoginResult{Token: token, ExpiresAt: expiresAt, User: u}, nil
}

// Me 返回当前登录用户。
func (s *Service) Me(ctx context.Context, userID uint64) (*platform.User, error) {
	if s.users == nil {
		return nil, httpx.ErrInternal("认证服务未正确初始化")
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, platform.ErrNotFound) {
			return nil, httpx.ErrUnauthorized("用户不存在或已被删除")
		}
		return nil, httpx.ErrInternal(err.Error())
	}
	if u.Status != platform.UserStatusActive {
		return nil, httpx.ErrUnauthorized("账号已被禁用")
	}
	return u, nil
}
