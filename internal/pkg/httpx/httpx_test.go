package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newCtx(method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestOKCreatedNoContent(t *testing.T) {
	c, w := newCtx(http.MethodGet, "/x", "")
	OK(c, map[string]int{"n": 1})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatalf("OK 响应不符: %d %s", w.Code, w.Body.String())
	}

	c, w = newCtx(http.MethodPost, "/x", "")
	Created(c, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("Created 应 201，实际 %d", w.Code)
	}

	c, w = newCtx(http.MethodDelete, "/x", "")
	NoContent(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"data":null`) {
		t.Fatalf("NoContent 响应不符: %d %s", w.Code, w.Body.String())
	}
}

func TestErrorConstructorsAndCodes(t *testing.T) {
	cases := []struct {
		err    *AppError
		code   int
		status int
	}{
		{ErrBadRequest("x"), CodeInvalidParam, 400},
		{ErrInvalidRelation("x"), CodeInvalidRelation, 400},
		{ErrUnauthorized("x"), CodeUnauthorized, 401},
		{ErrForbidden("x"), CodeForbidden, 403},
		{ErrNotFound("x"), CodeNotFound, 404},
		{ErrConflict("x"), CodeConflict, 409},
		{ErrPrecondition("x"), CodePrecondition, 422},
		{ErrInternal("x"), CodeInternal, 500},
	}
	for _, c := range cases {
		if c.err.Code != c.code || c.err.HTTP != c.status {
			t.Fatalf("%v 映射不符: %+v", c.err, c.err)
		}
		if c.err.Error() != "x" {
			t.Fatalf("Error() 不符")
		}
	}

	// WithMessage 返回副本
	base := ErrConflict("a")
	cp := base.WithMessage("b")
	if base.Message != "a" || cp.Message != "b" || cp.Code != base.Code {
		t.Fatalf("WithMessage 不符: %+v %+v", base, cp)
	}
}

func TestFail(t *testing.T) {
	// nil -> OK
	c, w := newCtx(http.MethodGet, "/x", "")
	Fail(c, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("nil 应 200，实际 %d", w.Code)
	}

	// AppError
	c, w = newCtx(http.MethodGet, "/x", "")
	Fail(c, ErrNotFound("未找到"))
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), `"code":30001`) {
		t.Fatalf("AppError 渲染不符: %d %s", w.Code, w.Body.String())
	}

	// wrapped AppError
	c, w = newCtx(http.MethodGet, "/x", "")
	Fail(c, errors.New("boom")) // 普通错误 -> 500
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), `"code":50000`) {
		t.Fatalf("普通错误应 500，实际 %d", w.Code)
	}
}

func TestParsePage_Normalization(t *testing.T) {
	cases := []struct {
		query      string
		wantPage   int
		wantSize   int
		wantOrder  string
		wantOffset int
	}{
		{"", 1, DefaultPageSize, "desc", 0},
		{"?page=0&page_size=0", 1, DefaultPageSize, "desc", 0},
		{"?page=abc&page_size=xyz", 1, DefaultPageSize, "desc", 0},
		{"?page=3&page_size=25", 3, 25, "desc", 50},
		{"?page=2&page_size=999", 2, MaxPageSize, "desc", 100},
		{"?page=1&page_size=10&order=asc", 1, 10, "asc", 0},
		{"?order=weird", 1, DefaultPageSize, "desc", 0},
		{"?sort_by=created_at&order=ASC", 1, DefaultPageSize, "asc", 0},
	}
	for _, c := range cases {
		ctx, _ := newCtx(http.MethodGet, "/x"+c.query, "")
		q := ParsePage(ctx)
		if q.Page != c.wantPage || q.PageSize != c.wantSize || q.Order != c.wantOrder {
			t.Fatalf("ParsePage(%q)=%+v", c.query, q)
		}
		if q.Offset() != c.wantOffset {
			t.Fatalf("Offset(%q)=%d，期望 %d", c.query, q.Offset(), c.wantOffset)
		}
	}
}

func TestOffsetEdge(t *testing.T) {
	if got := (PageQuery{Page: 0, PageSize: 0}).Offset(); got != 0 {
		t.Fatalf("非法分页 Offset 应 0，实际 %d", got)
	}
	if got := (PageQuery{Page: 2, PageSize: 0}).Offset(); got != DefaultPageSize {
		t.Fatalf("PageSize=0 应用默认，实际 %d", got)
	}
}

func TestNormalizeSort(t *testing.T) {
	allowed := map[string]bool{"created_at": true, "priority": true}
	q := PageQuery{SortBy: "bogus", Order: "x"}
	got := q.NormalizeSort(allowed, "created_at")
	if got.SortBy != "created_at" || got.Order != "desc" {
		t.Fatalf("回退不符: %+v", got)
	}
	q2 := PageQuery{SortBy: "priority", Order: "asc"}
	got2 := q2.NormalizeSort(allowed, "created_at")
	if got2.SortBy != "priority" || got2.Order != "asc" {
		t.Fatalf("保留合法排序失败: %+v", got2)
	}
	// allowed=nil 时仅校正 order
	got3 := PageQuery{SortBy: "keep", Order: "bad"}.NormalizeSort(nil, "fallback")
	if got3.SortBy != "keep" || got3.Order != "desc" {
		t.Fatalf("nil allowed 处理不符: %+v", got3)
	}
}

func TestNewPageResult(t *testing.T) {
	r := NewPageResult[string](nil, 5, 0, 0)
	if r.Items == nil || len(r.Items) != 0 || r.Page != 1 || r.PageSize != DefaultPageSize || r.Total != 5 {
		t.Fatalf("NewPageResult 归一化不符: %+v", r)
	}
	r2 := NewPageResult([]int{1, 2}, 2, 2, 10)
	if len(r2.Items) != 2 || r2.Page != 2 {
		t.Fatalf("NewPageResult 不符: %+v", r2)
	}
}

func TestBindHelpers(t *testing.T) {
	type body struct {
		Name string `json:"name" binding:"required"`
	}

	// 合法
	c, _ := newCtx(http.MethodPost, "/x", `{"name":"a"}`)
	var b body
	if err := ShouldBindJSON(c, &b); err != nil {
		t.Fatalf("合法 JSON 应通过: %v", err)
	}
	// 非法（缺字段）
	c, _ = newCtx(http.MethodPost, "/x", `{}`)
	if err := ShouldBindJSON(c, &body{}); err == nil {
		t.Fatalf("缺字段应报错")
	}
	// 非法 JSON
	c, _ = newCtx(http.MethodPost, "/x", `{`)
	if err := ShouldBindJSON(c, &body{}); err == nil {
		t.Fatalf("非法 JSON 应报错")
	}

	// Query
	type q struct {
		Page int `form:"page" binding:"required,min=1"`
	}
	c, _ = newCtx(http.MethodGet, "/x?page=2", "")
	if err := ShouldBindQuery(c, &q{}); err != nil {
		t.Fatalf("合法 query 应通过: %v", err)
	}
	c, _ = newCtx(http.MethodGet, "/x", "")
	if err := ShouldBindQuery(c, &q{}); err == nil {
		t.Fatalf("缺 query 应报错")
	}

	// URI
	type uri struct {
		ID uint64 `uri:"id" binding:"required"`
	}
	c, _ = newCtx(http.MethodGet, "/x/1", "")
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	if err := ShouldBindURI(c, &uri{}); err != nil {
		t.Fatalf("合法 uri 应通过: %v", err)
	}
	c, _ = newCtx(http.MethodGet, "/x/abc", "")
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	if err := ShouldBindURI(c, &uri{}); err == nil {
		t.Fatalf("非法 uri 应报错")
	}
}
