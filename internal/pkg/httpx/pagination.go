package httpx

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// 分页常量。
const (
	// DefaultPageSize 默认每页条数。
	DefaultPageSize = 20
	// MaxPageSize 每页条数上限（超过则截断）。
	MaxPageSize = 100
)

// PageQuery 是归一化后的分页查询参数。
type PageQuery struct {
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	SortBy   string `json:"sort_by"`
	Order    string `json:"order"`
}

// ParsePage 从 query 解析分页参数并归一化：
// page 默认 1（<1 归 1）；page_size 默认 20，>100 截断为 100；
// order 仅接受 asc/desc，默认 desc。
func ParsePage(c *gin.Context) PageQuery {
	page := atoiDefault(c.Query("page"), 1)
	if page < 1 {
		page = 1
	}

	size := atoiDefault(c.Query("page_size"), DefaultPageSize)
	if size < 1 {
		size = DefaultPageSize
	}
	if size > MaxPageSize {
		size = MaxPageSize
	}

	order := strings.ToLower(strings.TrimSpace(c.Query("order")))
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	return PageQuery{
		Page:     page,
		PageSize: size,
		SortBy:   strings.TrimSpace(c.Query("sort_by")),
		Order:    order,
	}
}

// Offset 返回 SQL LIMIT/OFFSET 的偏移量。
func (q PageQuery) Offset() int {
	if q.Page < 1 {
		return 0
	}
	if q.PageSize < 1 {
		return (q.Page - 1) * DefaultPageSize
	}
	return (q.Page - 1) * q.PageSize
}

// NormalizeSort 校验排序字段是否在白名单内；非法则回退到 fallback，order 缺省为 desc。
func (q PageQuery) NormalizeSort(allowed map[string]bool, fallback string) PageQuery {
	cp := q
	if cp.SortBy == "" || (allowed != nil && !allowed[cp.SortBy]) {
		cp.SortBy = fallback
	}
	if cp.Order != "asc" && cp.Order != "desc" {
		cp.Order = "desc"
	}
	return cp
}

// PageResult 是统一分页响应体 data。
type PageResult[T any] struct {
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Items    []T   `json:"items"`
}

// NewPageResult 构造分页结果；items 为 nil 时返回空切片以保证 JSON 为 []。
func NewPageResult[T any](items []T, total int64, page, size int) PageResult[T] {
	if items == nil {
		items = make([]T, 0)
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = DefaultPageSize
	}
	return PageResult[T]{Total: total, Page: page, PageSize: size, Items: items}
}

func atoiDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
