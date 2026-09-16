package asset

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

func newTestEngine(t *testing.T, e *testEnv, roleName string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		middleware.SetActor(c, middleware.Actor{UserID: 1, Role: roleName})
		c.Next()
	})
	Register(r.Group("/api/v1"), NewHandler(e.svc, nil))
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("编码请求体失败: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_AssetLifecycle(t *testing.T) {
	e := newTestEnv(t)
	r := newTestEngine(t, e, role.CmdbManager)
	pd := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// 新建
	w := doJSON(t, r, http.MethodPost, "/api/v1/assets", AssetRequest{AssetNo: "AST-0001", Name: "笔记本", Category: "laptop"})
	if w.Code != http.StatusCreated {
		t.Fatalf("新建应 201，实际 %d (%s)", w.Code, w.Body.String())
	}
	// 非法（缺 name）
	w = doJSON(t, r, http.MethodPost, "/api/v1/assets", map[string]any{"asset_no": "x"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("缺字段应 400，实际 %d", w.Code)
	}
	// 列表
	w = doJSON(t, r, http.MethodGet, "/api/v1/assets?category=laptop&status=planned&user_id=1&warranty_before=2027-01-01T00:00:00Z&page=1&page_size=10", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("列表应 200，实际 %d", w.Code)
	}
	// 详情
	w = doJSON(t, r, http.MethodGet, "/api/v1/assets/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("详情应 200，实际 %d", w.Code)
	}
	// 非法 id
	w = doJSON(t, r, http.MethodGet, "/api/v1/assets/abc", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，实际 %d", w.Code)
	}
	// 编辑
	w = doJSON(t, r, http.MethodPut, "/api/v1/assets/1", AssetRequest{AssetNo: "AST-0001", Name: "笔记本改", Category: "laptop"})
	if w.Code != http.StatusOK {
		t.Fatalf("编辑应 200，实际 %d", w.Code)
	}
	// 流转 stock_in（带采购日期）
	w = doJSON(t, r, http.MethodPost, "/api/v1/assets/1/transition", TransitionRequest{Action: ActionStockIn, PurchaseDate: &pd})
	if w.Code != http.StatusOK {
		t.Fatalf("入库流转应 200，实际 %d (%s)", w.Code, w.Body.String())
	}
	// 非法流转
	w = doJSON(t, r, http.MethodPost, "/api/v1/assets/1/transition", TransitionRequest{Action: ActionFinishMaintain})
	if w.Code != http.StatusConflict {
		t.Fatalf("非法流转应 409，实际 %d", w.Code)
	}
	// 绑定 CI
	w = doJSON(t, r, http.MethodPost, "/api/v1/assets/1/bind-ci", BindCIRequest{CIID: 100})
	if w.Code != http.StatusOK {
		t.Fatalf("绑定 CI 应 200，实际 %d (%s)", w.Code, w.Body.String())
	}
	// 绑定非法 ci_id
	w = doJSON(t, r, http.MethodPost, "/api/v1/assets/1/bind-ci", map[string]any{"ci_id": 0})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("ci_id=0 应 400，实际 %d", w.Code)
	}
	// 历史
	w = doJSON(t, r, http.MethodGet, "/api/v1/assets/1/history", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("历史应 200，实际 %d", w.Code)
	}
	// 解绑
	w = doJSON(t, r, http.MethodDelete, "/api/v1/assets/1/bind-ci", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("解绑应 200，实际 %d", w.Code)
	}
	// 删除
	w = doJSON(t, r, http.MethodDelete, "/api/v1/assets/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("删除应 200，实际 %d", w.Code)
	}
}

func TestHandler_BadJSON(t *testing.T) {
	e := newTestEnv(t)
	r := newTestEngine(t, e, role.Admin)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法 JSON 应 400，实际 %d", w.Code)
	}
}

func TestHandler_RoleForbidden(t *testing.T) {
	e := newTestEnv(t)
	// requestor 无 perm.asset.manage → 403
	r := newTestEngine(t, e, role.Requestor)
	w := doJSON(t, r, http.MethodGet, "/api/v1/assets", nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("越权应 403，实际 %d", w.Code)
	}
}
