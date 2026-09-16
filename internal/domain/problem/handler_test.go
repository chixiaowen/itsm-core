package problem

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
	"github.com/chixiaowen/itsm-core/internal/pkg/security"
)

func newProblemEngine(t *testing.T) (*gin.Engine, *security.JWTManager, *testEnv) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	env := newTestEnv(t)
	h := NewHandler(env.svc, nil)
	jwtMgr := security.NewJWTManager("secret", time.Hour)
	r := gin.New()
	r.Use(middleware.RequestID())
	rg := r.Group("/api/v1", middleware.Auth(jwtMgr, "/healthz"))
	Register(rg, h)
	return r, jwtMgr, env
}

func prbToken(t *testing.T, m *security.JWTManager, id uint64, r string) string {
	t.Helper()
	tok, _, err := m.Issue(id, "u", r)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func prbReq(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestProblemHTTP_Flow(t *testing.T) {
	r, jwt, _ := newProblemEngine(t)
	pmTok := prbToken(t, jwt, 7, role.ProblemManager)
	adminTok := prbToken(t, jwt, 9, role.Admin)
	reqTok := prbToken(t, jwt, 20, role.Requestor)

	// 无 token -> 401
	if w := prbReq(r, http.MethodGet, "/api/v1/problems", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401，实际 %d", w.Code)
	}
	// 聚合建议 -> 200
	if w := prbReq(r, http.MethodGet, "/api/v1/problems/aggregate-suggestions?ci_id=5&days=30", "", pmTok); w.Code != http.StatusOK {
		t.Fatalf("建议聚合应 200，实际 %d", w.Code)
	}
	// 手动新建 -> 201
	if w := prbReq(r, http.MethodPost, "/api/v1/problems", `{"title":"数据库超时","source":"manual"}`, pmTok); w.Code != http.StatusCreated {
		t.Fatalf("新建应 201，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 空标题 -> 400
	if w := prbReq(r, http.MethodPost, "/api/v1/problems", `{"title":""}`, pmTok); w.Code != http.StatusBadRequest {
		t.Fatalf("空标题应 400，实际 %d", w.Code)
	}
	// 列表 -> 200
	if w := prbReq(r, http.MethodGet, "/api/v1/problems?status=new&known_error=false&page=1", "", pmTok); w.Code != http.StatusOK {
		t.Fatalf("列表应 200，实际 %d", w.Code)
	}
	// 详情 -> 200
	if w := prbReq(r, http.MethodGet, "/api/v1/problems/1", "", pmTok); w.Code != http.StatusOK {
		t.Fatalf("详情应 200，实际 %d", w.Code)
	}
	// 非法 id -> 400
	if w := prbReq(r, http.MethodGet, "/api/v1/problems/abc", "", pmTok); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400")
	}
	// 编辑 -> 200
	if w := prbReq(r, http.MethodPut, "/api/v1/problems/1", `{"symptom":"连接超时"}`, pmTok); w.Code != http.StatusOK {
		t.Fatalf("编辑应 200，实际 %d", w.Code)
	}
	// 流转 triage -> 200
	if w := prbReq(r, http.MethodPost, "/api/v1/problems/1/transition", `{"action":"triage"}`, pmTok); w.Code != http.StatusOK {
		t.Fatalf("triage 应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	// investigate 无 assignee -> 422
	if w := prbReq(r, http.MethodPost, "/api/v1/problems/1/transition", `{"action":"investigate"}`, pmTok); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("investigate 无指派应 422，实际 %d", w.Code)
	}
	// investigate -> 200
	if w := prbReq(r, http.MethodPost, "/api/v1/problems/1/transition", `{"action":"investigate","assignee_id":7}`, pmTok); w.Code != http.StatusOK {
		t.Fatalf("investigate 应 200，实际 %d", w.Code)
	}
	// known-error 缺字段 -> 422（service 层前置条件）
	if w := prbReq(r, http.MethodPost, "/api/v1/problems/1/known-error", `{"root_cause":"r"}`, pmTok); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("known-error 缺字段应 422，实际 %d", w.Code)
	}
	// known-error -> 200
	if w := prbReq(r, http.MethodPost, "/api/v1/problems/1/known-error", `{"root_cause":"r","workaround":"w"}`, pmTok); w.Code != http.StatusOK {
		t.Fatalf("known-error 应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 关联变更 -> 200
	if w := prbReq(r, http.MethodPost, "/api/v1/problems/1/changes", `{"change_ids":[11,12]}`, pmTok); w.Code != http.StatusOK {
		t.Fatalf("关联变更应 200，实际 %d", w.Code)
	}
	// 解除变更 -> 200
	if w := prbReq(r, http.MethodDelete, "/api/v1/problems/1/changes/11", "", pmTok); w.Code != http.StatusOK {
		t.Fatalf("解除变更应 200，实际 %d", w.Code)
	}
	// resolve（无 closed 变更、无 no_change_reason）-> 422
	if w := prbReq(r, http.MethodPost, "/api/v1/problems/1/transition", `{"action":"resolve"}`, pmTok); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("resolve 无约束应 422，实际 %d", w.Code)
	}
	// resolve 带 no_change_reason -> 200
	if w := prbReq(r, http.MethodPost, "/api/v1/problems/1/transition", `{"action":"resolve","no_change_reason":"无需变更"}`, pmTok); w.Code != http.StatusOK {
		t.Fatalf("resolve 带原因应 200，实际 %d", w.Code)
	}
	// close -> 200
	if w := prbReq(r, http.MethodPost, "/api/v1/problems/1/transition", `{"action":"close"}`, pmTok); w.Code != http.StatusOK {
		t.Fatalf("close 应 200，实际 %d", w.Code)
	}
	// 删除（admin）-> 200
	if w := prbReq(r, http.MethodDelete, "/api/v1/problems/1", "", adminTok); w.Code != http.StatusOK {
		t.Fatalf("删除应 200，实际 %d", w.Code)
	}

	// requestor 列表 -> 403（需 problem_manager/resolver/admin）
	if w := prbReq(r, http.MethodGet, "/api/v1/problems", "", reqTok); w.Code != http.StatusForbidden {
		t.Fatalf("requestor 列表应 403，实际 %d", w.Code)
	}
	// requestor 新建 -> 403
	if w := prbReq(r, http.MethodPost, "/api/v1/problems", `{"title":"x"}`, reqTok); w.Code != http.StatusForbidden {
		t.Fatalf("requestor 新建应 403，实际 %d", w.Code)
	}
	// requestor 删除 -> 403
	if w := prbReq(r, http.MethodDelete, "/api/v1/problems/2", "", reqTok); w.Code != http.StatusForbidden {
		t.Fatalf("requestor 删除应 403，实际 %d", w.Code)
	}
}
