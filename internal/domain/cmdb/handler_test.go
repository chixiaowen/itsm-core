package cmdb

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

func TestHandler_CILifecycle(t *testing.T) {
	e := newTestEnv(t)
	r := newTestEngine(t, e, role.CmdbManager)

	// 新建
	w := doJSON(t, r, http.MethodPost, "/api/v1/cis", CIRequest{Code: "srv-01", Name: "服务器", CIType: CITypeServer})
	if w.Code != http.StatusCreated {
		t.Fatalf("新建 CI 应 201，实际 %d (%s)", w.Code, w.Body.String())
	}
	// 参数非法
	w = doJSON(t, r, http.MethodPost, "/api/v1/cis", map[string]any{"name": "x"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("缺字段应 400，实际 %d", w.Code)
	}
	// 列表
	w = doJSON(t, r, http.MethodGet, "/api/v1/cis?ci_type=server&status=planned&owner_id=1&keyword=srv&page=1&page_size=10", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("列表应 200，实际 %d", w.Code)
	}
	// 详情
	w = doJSON(t, r, http.MethodGet, "/api/v1/cis/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("详情应 200，实际 %d", w.Code)
	}
	// 非法 id
	w = doJSON(t, r, http.MethodGet, "/api/v1/cis/abc", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，实际 %d", w.Code)
	}
	// 编辑
	w = doJSON(t, r, http.MethodPut, "/api/v1/cis/1", CIRequest{Code: "srv-01", Name: "服务器改", CIType: CITypeServer})
	if w.Code != http.StatusOK {
		t.Fatalf("编辑应 200，实际 %d", w.Code)
	}
	// CI 类型枚举
	w = doJSON(t, r, http.MethodGet, "/api/v1/ci-types", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("枚举应 200，实际 %d", w.Code)
	}
	// 删除
	w = doJSON(t, r, http.MethodDelete, "/api/v1/cis/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("删除应 200，实际 %d", w.Code)
	}
}

func TestHandler_RelationsAndTopology(t *testing.T) {
	e := newTestEnv(t)
	r := newTestEngine(t, e, role.CmdbManager)
	a := e.seedCI(t, "a", "A", CITypeServer)
	b := e.seedCI(t, "b", "B", CITypeDatabase)

	// 新增关系
	w := doJSON(t, r, http.MethodPost, "/api/v1/cis/1/relations", RelationRequest{TargetCIID: b.ID, RelationType: RelationDependsOn})
	if w.Code != http.StatusCreated {
		t.Fatalf("新增关系应 201，实际 %d (%s)", w.Code, w.Body.String())
	}
	// 自环 → 400
	w = doJSON(t, r, http.MethodPost, "/api/v1/cis/1/relations", RelationRequest{TargetCIID: a.ID, RelationType: RelationDependsOn})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("自环应 400，实际 %d", w.Code)
	}
	// 重复 → 409
	w = doJSON(t, r, http.MethodPost, "/api/v1/cis/1/relations", RelationRequest{TargetCIID: b.ID, RelationType: RelationDependsOn})
	if w.Code != http.StatusConflict {
		t.Fatalf("重复应 409，实际 %d", w.Code)
	}
	// 拓扑
	w = doJSON(t, r, http.MethodGet, "/api/v1/cis/1/topology?depth=2&direction=both", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("拓扑应 200，实际 %d", w.Code)
	}
	// 删除关系
	w = doJSON(t, r, http.MethodDelete, "/api/v1/cis/1/relations/1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("删除关系应 200，实际 %d", w.Code)
	}
	// relId 非法
	w = doJSON(t, r, http.MethodDelete, "/api/v1/cis/1/relations/abc", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法 relId 应 400，实际 %d", w.Code)
	}
}

func TestHandler_RoleForbidden(t *testing.T) {
	e := newTestEnv(t)
	// requestor 无 perm.cmdb.manage → 403
	r := newTestEngine(t, e, role.Requestor)
	w := doJSON(t, r, http.MethodGet, "/api/v1/cis", nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("越权应 403，实际 %d", w.Code)
	}
}
