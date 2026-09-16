package security

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// DefaultBcryptCost 是密码哈希默认成本。
const DefaultBcryptCost = bcrypt.DefaultCost

// ErrEmptyPassword 表示明文密码为空。
var ErrEmptyPassword = errors.New("密码不得为空")

// HashPassword 使用 bcrypt 生成密码哈希。
func HashPassword(plain string) (string, error) {
	if plain == "" {
		return "", ErrEmptyPassword
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), DefaultBcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ComparePassword 校验明文密码与哈希是否匹配。
func ComparePassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// ComparePasswordErr 校验并返回具体错误（bcrypt.ErrMismatchedHashAndPassword 表示不匹配）。
func ComparePasswordErr(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
