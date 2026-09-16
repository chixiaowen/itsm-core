package catalog

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

// newTestEngine 构建挂载 catalog 路由的测试 engine，并注入固定角色操作者。
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

func TestHandler_CategoryCRUD(t *testing.T) {
	e := newTestEnv(t)
	r := newTestEngine(t, e, role.Admin)

	// 新建分类
	w := doJSON(t, r, http.MethodPost, "/api/v1/service-categories", CategoryRequest{Name: "办公支持", SortOrder: 1})
	if w.Code != http.StatusCreated {
		t.Fatalf("新建分类应 201，实际 %d (%s)", w.Code, w.Body.String())
	}
	// 参数非法（缺 name）
	w = doJSON(t, r, http.MethodPost, "/api/v1/service-categories", map[string]any{"sort_order": 1})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("缺 name 应 400，实际 %d", w.Code)
	}
	// 列表
	w = doJSON(t, r, http.MethodGet, "/api/v1/service-categories", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("分类树应 200，实际 %d", w.Code)
	}
	// 编辑
	w = doJSON(t, r, http.MethodPut, "/api/v1/service-categories/1", CategoryRequest{Name: "办公支持2", SortOrder: 2})
	if w.Code != http.StatusOK {
		t.Fatalf("编辑分类应 200，实际 %d", w.Code)
	}
	// 非法路径参数
	w = doJSON(t, r, http.MethodPut, "/api/v1/service-categories/abc", CategoryRequest{Name: "x"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，实际 %d", w.Code)
	}
	// 删除
	w = doJSON(t, r, http.MethodDelete, "/api/v1/service-categories/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("删除分类应 200，实际 %d", w.Code)
	}
}

func TestHandler_ItemLifecycle(t *testing.T) {
	e := newTestEnv(t)
	r := newTestEngine(t, e, role.Admin)
	cat := e.seedCategory(t, "根")

	body := ItemRequest{Name: "申请 VPN", CategoryID: cat.ID, SLAPolicyID: ptrU64(1), FormSchema: validSchema, DefaultPriority: "P2"}
	w := doJSON(t, r, http.MethodPost, "/api/v1/service-items", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("新建服务项应 201，实际 %d (%s)", w.Code, w.Body.String())
	}
	// 参数非法（缺 name / category_id）
	w = doJSON(t, r, http.MethodPost, "/api/v1/service-items", map[string]any{"name": ""})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法服务项应 400，实际 %d", w.Code)
	}
	// 列表（含筛选项）
	w = doJSON(t, r, http.MethodGet, "/api/v1/service-items?status=draft&category_id=1&keyword=vpn&page=1&page_size=10", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("服务项列表应 200，实际 %d", w.Code)
	}
	// 详情
	w = doJSON(t, r, http.MethodGet, "/api/v1/service-items/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("详情应 200，实际 %d", w.Code)
	}
	// 详情不存在
	w = doJSON(t, r, http.MethodGet, "/api/v1/service-items/999", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("不存在应 404，实际 %d", w.Code)
	}
	// 编辑
	w = doJSON(t, r, http.MethodPut, "/api/v1/service-items/1", body)
	if w.Code != http.StatusOK {
		t.Fatalf("编辑应 200，实际 %d", w.Code)
	}
	// 提交流转
	w = doJSON(t, r, http.MethodPost, "/api/v1/service-items/1/transition", ItemTransitionRequest{Action: ActionSubmitReview})
	if w.Code != http.StatusOK {
		t.Fatalf("流转应 200，实际 %d (%s)", w.Code, w.Body.String())
	}
	// 发布
	w = doJSON(t, r, http.MethodPost, "/api/v1/service-items/1/publish", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("发布应 200，实际 %d (%s)", w.Code, w.Body.String())
	}
	// 下线
	w = doJSON(t, r, http.MethodPost, "/api/v1/service-items/1/offline", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("下线应 200，实际 %d", w.Code)
	}
	// 删除（offline -> archived）
	w = doJSON(t, r, http.MethodDelete, "/api/v1/service-items/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("归档应 200，实际 %d (%s)", w.Code, w.Body.String())
	}
}

func TestHandler_UserCatalogAndOrder(t *testing.T) {
	e := newTestEnv(t)
	cat := e.seedCategory(t, "根")
	pub := e.seedItem(t, "申请 VPN", cat.ID)
	e.publishItem(t, pub.ID)

	r := newTestEngine(t, e, role.Requestor)
	w := doJSON(t, r, http.MethodGet, "/api/v1/catalog/categories", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("用户侧分类应 200，实际 %d", w.Code)
	}
	w = doJSON(t, r, http.MethodGet, "/api/v1/catalog/items?category_id=1&keyword=vpn", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("用户侧服务项应 200，实际 %d", w.Code)
	}
	// 下单成功
	w = doJSON(t, r, http.MethodPost, "/api/v1/catalog/items/1/order", OrderRequest{FormData: map[string]any{"reason": "远程办公"}})
	if w.Code != http.StatusCreated {
		t.Fatalf("下单应 201，实际 %d (%s)", w.Code, w.Body.String())
	}
	// 下单缺必填 → 400
	w = doJSON(t, r, http.MethodPost, "/api/v1/catalog/items/1/order", OrderRequest{FormData: map[string]any{}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("缺必填应 400，实际 %d", w.Code)
	}
}

func TestHandler_RoleForbidden(t *testing.T) {
	e := newTestEnv(t)
	// requestor 无 perm.catalog.manage → 管理台接口 403
	r := newTestEngine(t, e, role.Requestor)
	w := doJSON(t, r, http.MethodGet, "/api/v1/service-items", nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("越权应 403，实际 %d", w.Code)
	}
}
