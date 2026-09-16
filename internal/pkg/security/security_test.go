package security

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// signWithSecret 用指定密钥签发一份 claims（仅测试用）。
func signWithSecret(c Claims, secret string) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(secret))
}

func TestPassword_HashCompareRoundTrip(t *testing.T) {
	hash, err := HashPassword("s3cret!")
	if err != nil {
		t.Fatalf("HashPassword 失败: %v", err)
	}
	if hash == "s3cret!" {
		t.Fatalf("哈希不应等于明文")
	}
	if !ComparePassword(hash, "s3cret!") {
		t.Fatalf("正确密码应校验通过")
	}
	if ComparePassword(hash, "wrong") {
		t.Fatalf("错误密码不应通过")
	}
	if err := ComparePasswordErr(hash, "wrong"); err == nil {
		t.Fatalf("错误密码应返回错误")
	}
}

func TestPassword_Empty(t *testing.T) {
	if _, err := HashPassword(""); !errors.Is(err, ErrEmptyPassword) {
		t.Fatalf("空密码应返回 ErrEmptyPassword，实际 %v", err)
	}
}

func TestJWT_IssueParseRoundTrip(t *testing.T) {
	m := NewJWTManager("test-secret", time.Hour)
	token, exp, err := m.Issue(42, "alice", "agent")
	if err != nil {
		t.Fatalf("Issue 失败: %v", err)
	}
	if token == "" {
		t.Fatalf("token 不应为空")
	}
	if time.Until(exp) <= 0 {
		t.Fatalf("过期时间应在未来")
	}

	claims, err := m.Parse(token)
	if err != nil {
		t.Fatalf("Parse 失败: %v", err)
	}
	if claims.UserID != 42 || claims.Username != "alice" || claims.Role != "agent" {
		t.Fatalf("claims 不符: %+v", claims)
	}
	if claims.Issuer != "itsm-core" {
		t.Fatalf("issuer 不符: %s", claims.Issuer)
	}
}

func TestJWT_TTLDefault(t *testing.T) {
	m := NewJWTManager("s", 0)
	if m.TTL() != 24*time.Hour {
		t.Fatalf("ttl==0 应回退 24h，实际 %v", m.TTL())
	}
}

func TestJWT_EmptySecret(t *testing.T) {
	m := NewJWTManager("", time.Hour)
	if _, _, err := m.Issue(1, "u", "agent"); !errors.Is(err, ErrEmptySecret) {
		t.Fatalf("空密钥应返回 ErrEmptySecret，实际 %v", err)
	}
}

func TestJWT_InvalidTokens(t *testing.T) {
	m := NewJWTManager("secret-a", time.Hour)

	// 空 token
	if _, err := m.Parse(""); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("空 token 应无效，实际 %v", err)
	}
	// 乱码 token
	if _, err := m.Parse("not-a-jwt"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("乱码 token 应无效，实际 %v", err)
	}

	// 不同密钥签发 -> 签名不符
	other := NewJWTManager("secret-b", time.Hour)
	tokenB, _, _ := other.Issue(7, "bob", "admin")
	if _, err := m.Parse(tokenB); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("异密钥 token 应无效，实际 %v", err)
	}
}

func TestJWT_Expired(t *testing.T) {
	// ttl 为负 -> 立即过期
	m := NewJWTManager("secret", -time.Minute)
	token, _, err := m.Issue(1, "u", "agent")
	if err != nil {
		t.Fatalf("Issue 失败: %v", err)
	}
	if _, err := m.Parse(token); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("过期 token 应返回 ErrExpiredToken，实际 %v", err)
	}
}

func TestJWT_MissingClaimsRejected(t *testing.T) {
	// 直接构造一个缺少 uid/role 的合法签名 token
	m := NewJWTManager("secret", time.Hour)
	empty := Claims{}
	signed, err := signWithSecret(empty, "secret")
	if err != nil {
		t.Fatalf("构造测试 token 失败: %v", err)
	}
	if _, err := m.Parse(signed); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("缺少 uid/role 应无效，实际 %v", err)
	}
	if !strings.Contains(ErrInvalidToken.Error(), "无效") {
		t.Fatalf("错误信息应可读")
	}
}
