package security

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWT 相关错误。
var (
	// ErrEmptySecret 表示 JWT 密钥为空。
	ErrEmptySecret = errors.New("JWT 密钥不得为空")
	// ErrInvalidToken 表示 token 无效（签名不符/格式错误/字段缺失）。
	ErrInvalidToken = errors.New("token 无效")
	// ErrExpiredToken 表示 token 已过期。
	ErrExpiredToken = errors.New("token 已过期")
)

// Claims 是 JWT 载荷。
type Claims struct {
	UserID   uint64 `json:"uid"`
	Username string `json:"usr"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// JWTManager 负责签发与解析 HS256 JWT。
type JWTManager struct {
	secret []byte
	ttl    time.Duration
	issuer string
	now    func() time.Time
}

// NewJWTManager 构造签发器。ttl == 0 时回退为 24h；issuer 缺省为 "itsm-core"。
// 传入负值 ttl 表示立即过期（用于测试）。
func NewJWTManager(secret string, ttl time.Duration, issuer ...string) *JWTManager {
	iss := "itsm-core"
	if len(issuer) > 0 && issuer[0] != "" {
		iss = issuer[0]
	}
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	return &JWTManager{secret: []byte(secret), ttl: ttl, issuer: iss, now: time.Now}
}

// TTL 返回 token 有效期。
func (m *JWTManager) TTL() time.Duration { return m.ttl }

// Issue 签发 JWT，返回 token 与过期时间。
func (m *JWTManager) Issue(userID uint64, username, role string) (string, time.Time, error) {
	if len(m.secret) == 0 {
		return "", time.Time{}, ErrEmptySecret
	}
	now := m.now()
	expiresAt := now.Add(m.ttl)

	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   strconv.FormatUint(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

// Parse 解析并校验 JWT。
func (m *JWTManager) Parse(tokenString string) (*Claims, error) {
	if tokenString == "" {
		return nil, ErrInvalidToken
	}
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}
	if claims.UserID == 0 || claims.Role == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
