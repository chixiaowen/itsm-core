package httpx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// 业务错误码（与 PRD §5、ARCHITECTURE §8.2 对齐）。
const (
	CodeOK              = 0
	CodeInvalidParam    = 10001 // 400 参数校验失败
	CodeInvalidRelation = 10002 // 400 自环/非法关系等业务参数错误
	CodeUnauthorized    = 20001 // 401 未登录 / token 无效或过期
	CodeForbidden       = 20003 // 403 越权
	CodeNotFound        = 30001 // 404 资源不存在
	CodeConflict        = 40001 // 409 状态冲突 / 非法流转 / 重复操作
	CodePrecondition    = 40002 // 422 业务前置条件不满足
	CodeInternal        = 50000 // 500 服务器内部错误
)

// AppError 是携带业务错误码与 HTTP 状态的应用错误。
type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	HTTP    int    `json:"-"`
}

// Error 实现 error 接口。
func (e *AppError) Error() string { return e.Message }

// WithMessage 基于当前错误返回一个替换了 message 的同码错误副本。
func (e *AppError) WithMessage(msg string) *AppError {
	cp := *e
	cp.Message = msg
	return &cp
}

// ErrBadRequest 400 / 10001：参数校验失败。
func ErrBadRequest(msg string) *AppError {
	return &AppError{Code: CodeInvalidParam, Message: msg, HTTP: http.StatusBadRequest}
}

// ErrInvalidRelation 400 / 10002：自环、非法关系等业务参数错误。
func ErrInvalidRelation(msg string) *AppError {
	return &AppError{Code: CodeInvalidRelation, Message: msg, HTTP: http.StatusBadRequest}
}

// ErrUnauthorized 401 / 20001：未登录或 token 无效。
func ErrUnauthorized(msg string) *AppError {
	return &AppError{Code: CodeUnauthorized, Message: msg, HTTP: http.StatusUnauthorized}
}

// ErrForbidden 403 / 20003：越权。
func ErrForbidden(msg string) *AppError {
	return &AppError{Code: CodeForbidden, Message: msg, HTTP: http.StatusForbidden}
}

// ErrNotFound 404 / 30001：资源不存在。
func ErrNotFound(msg string) *AppError {
	return &AppError{Code: CodeNotFound, Message: msg, HTTP: http.StatusNotFound}
}

// ErrConflict 409 / 40001：状态冲突 / 非法流转 / 重复操作。
func ErrConflict(msg string) *AppError {
	return &AppError{Code: CodeConflict, Message: msg, HTTP: http.StatusConflict}
}

// ErrPrecondition 422 / 40002：业务前置条件不满足。
func ErrPrecondition(msg string) *AppError {
	return &AppError{Code: CodePrecondition, Message: msg, HTTP: http.StatusUnprocessableEntity}
}

// ErrInternal 500 / 50000：服务器内部错误。
func ErrInternal(msg string) *AppError {
	return &AppError{Code: CodeInternal, Message: msg, HTTP: http.StatusInternalServerError}
}

// Fail 统一错误渲染：命中 *AppError 按其 HTTP/Code 渲染，否则 500 且只回显通用信息。
func Fail(c *gin.Context, err error) {
	if err == nil {
		OK(c, nil)
		return
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		c.AbortWithStatusJSON(appErr.HTTP, Body{Code: appErr.Code, Message: appErr.Message, Data: nil})
		return
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, Body{Code: CodeInternal, Message: "服务器内部错误", Data: nil})
}
