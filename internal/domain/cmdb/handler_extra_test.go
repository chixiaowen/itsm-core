package cmdb

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

// doRaw 发送原始字符串请求体（用于触发 JSON 绑定失败）。
func doRaw(t *testing.T, r *gin.Engine, method, path, raw string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_BadInputs(t *testing.T) {
	e := newTestEnv(t)
	r := newTestEngine(t, e, role.CmdbManager)

	// 非法 JSON 绑定失败
	if w := doRaw(t, r, http.MethodPost, "/api/v1/cis", "{bad"); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 JSON 应 400，实际 %d", w.Code)
	}
	// 编辑：非法 id
	if w := doJSON(t, r, http.MethodPut, "/api/v1/cis/abc", CIRequest{Code: "x", Name: "x", CIType: CITypeServer}); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，实际 %d", w.Code)
	}
	// 编辑：不存在
	if w := doJSON(t, r, http.MethodPut, "/api/v1/cis/999", CIRequest{Code: "x", Name: "x", CIType: CITypeServer}); w.Code != http.StatusNotFound {
		t.Fatalf("不存在应 404，实际 %d", w.Code)
	}
	// 编辑：非法 JSON
	if w := doRaw(t, r, http.MethodPut, "/api/v1/cis/1", "{bad"); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 JSON 应 400，实际 %d", w.Code)
	}
	// 详情：非法 id
	if w := doJSON(t, r, http.MethodGet, "/api/v1/cis/abc", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，实际 %d", w.Code)
	}
	// 删除：非法 id
	if w := doJSON(t, r, http.MethodDelete, "/api/v1/cis/abc", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，实际 %d", w.Code)
	}
	// 删除：不存在 → 404
	if w := doJSON(t, r, http.MethodDelete, "/api/v1/cis/999", nil); w.Code != http.StatusNotFound {
		t.Fatalf("不存在应 404，实际 %d", w.Code)
	}
	// 关系：非法 JSON
	if w := doRaw(t, r, http.MethodPost, "/api/v1/cis/1/relations", "{bad"); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 JSON 应 400，实际 %d", w.Code)
	}
	// 关系：缺 target → 400
	if w := doJSON(t, r, http.MethodPost, "/api/v1/cis/1/relations", map[string]any{"relation_type": RelationDependsOn}); w.Code != http.StatusBadRequest {
		t.Fatalf("缺 target 应 400，实际 %d", w.Code)
	}
	// 拓扑：非法 id
	if w := doJSON(t, r, http.MethodGet, "/api/v1/cis/abc/topology", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，实际 %d", w.Code)
	}
}

func TestHandler_DuplicateCode409(t *testing.T) {
	e := newTestEnv(t)
	r := newTestEngine(t, e, role.Admin)
	if w := doJSON(t, r, http.MethodPost, "/api/v1/cis", CIRequest{Code: "dup", Name: "A", CIType: CITypeServer}); w.Code != http.StatusCreated {
		t.Fatalf("首次应 201，实际 %d", w.Code)
	}
	if w := doJSON(t, r, http.MethodPost, "/api/v1/cis", CIRequest{Code: "dup", Name: "B", CIType: CITypeServer}); w.Code != http.StatusConflict {
		t.Fatalf("重复应 409，实际 %d", w.Code)
	}
}

func TestModel_TableNamesAndEnums(t *testing.T) {
	if (CI{}).TableName() != "cis" {
		t.Fatalf("CI 表名错误")
	}
	if (CIRelation{}).TableName() != "ci_relations" {
		t.Fatalf("CIRelation 表名错误")
	}
	if len(CITypes()) != 6 || len(RelationTypes()) != 3 {
		t.Fatalf("枚举数量不符")
	}
	if !ValidCIType(CITypeApp) || ValidCIType("vm") {
		t.Fatalf("CI 类型校验异常")
	}
	if !ValidCIStatus(StatusDisposed) || ValidCIStatus("zombie") {
		t.Fatalf("CI 状态校验异常")
	}
	if !ValidRelationType(RelationContains) || ValidRelationType("owns") {
		t.Fatalf("关系类型校验异常")
	}
}
