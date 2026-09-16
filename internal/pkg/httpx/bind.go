package httpx

import (
	"github.com/gin-gonic/gin"
)

// ShouldBindJSON 绑定并校验 JSON 请求体；失败统一返回 400 / 10001。
func ShouldBindJSON(c *gin.Context, dst any) error {
	if err := c.ShouldBindJSON(dst); err != nil {
		return ErrBadRequest("参数校验失败: " + err.Error())
	}
	return nil
}

// ShouldBindQuery 绑定并校验 query 参数；失败统一返回 400 / 10001。
func ShouldBindQuery(c *gin.Context, dst any) error {
	if err := c.ShouldBindQuery(dst); err != nil {
		return ErrBadRequest("参数校验失败: " + err.Error())
	}
	return nil
}

// ShouldBindURI 绑定并校验路径参数；失败统一返回 400 / 10001。
func ShouldBindURI(c *gin.Context, dst any) error {
	if err := c.ShouldBindUri(dst); err != nil {
		return ErrBadRequest("路径参数非法: " + err.Error())
	}
	return nil
}
