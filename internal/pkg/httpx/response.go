// Package httpx 提供与业务无关的 HTTP 通用能力：
// 统一响应体、错误码与 HTTP 映射、分页、参数绑定。
package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Body 是统一响应体：{"code":0,"message":"ok","data":...}。
type Body struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// OK 渲染 200 成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: CodeOK, Message: "ok", Data: data})
}

// Created 渲染 201 成功响应（资源创建）。
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Body{Code: CodeOK, Message: "ok", Data: data})
}

// NoContent 渲染删除成功响应。
//
// 为保持前端统一响应体（{code,message,data}）可解析，使用 200 + data:null，
// 而非 HTTP 204（204 无响应体，前端拦截器无法读取 code）。
func NoContent(c *gin.Context) {
	c.JSON(http.StatusOK, Body{Code: CodeOK, Message: "ok", Data: nil})
}
