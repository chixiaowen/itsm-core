package security

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestComparePassword_MalformedHash(t *testing.T) {
	if ComparePassword("not-a-bcrypt-hash", "x") {
		t.Fatalf("非法哈希应返回 false")
	}
	if err := ComparePasswordErr("not-a-hash", "x"); err == nil {
		t.Fatalf("非法哈希应返回错误")
	}
}

func TestJWT_CustomIssuer(t *testing.T) {
	m := NewJWTManager("secret", time.Hour, "itsm-test")
	tok, _, err := m.Issue(1, "u", "agent")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	cl, err := m.Parse(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cl.Issuer != "itsm-test" {
		t.Fatalf("issuer 应自定义，实际 %s", cl.Issuer)
	}
}

func TestJWT_NonHMACRejected(t *testing.T) {
	m := NewJWTManager("secret", time.Hour)
	claims := Claims{
		UserID: 1,
		Role:   "agent",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("构造 none token 失败: %v", err)
	}
	if _, err := m.Parse(tok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("非 HMAC token 应无效，实际 %v", err)
	}
}
