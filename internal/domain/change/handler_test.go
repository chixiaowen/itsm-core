package change

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

func newChangeEngine(t *testing.T) (*gin.Engine, *security.JWTManager, *testEnv) {
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

func chgToken(t *testing.T, m *security.JWTManager, id uint64, r string) string {
	t.Helper()
	tok, _, err := m.Issue(id, "u", r)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func chgReq(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
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

func TestChangeHTTP_FullFlow(t *testing.T) {
	r, jwt, _ := newChangeEngine(t)
	cmTok := chgToken(t, jwt, 7, role.ChangeManager)
	cm2Tok := chgToken(t, jwt, 8, role.ChangeManager)
	resTok := chgToken(t, jwt, 3, role.Resolver)
	adminTok := chgToken(t, jwt, 9, role.Admin)
	reqTok := chgToken(t, jwt, 5, role.Requestor)

	// 无 token -> 401
	if w := chgReq(r, http.MethodGet, "/api/v1/changes", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401，实际 %d", w.Code)
	}
	// requestor 无 change.submit 权限 -> 403
	if w := chgReq(r, http.MethodPost, "/api/v1/changes", `{"title":"t","change_type":"normal","risk_level":"high"}`, reqTok); w.Code != http.StatusForbidden {
		t.Fatalf("requestor 创建应 403，实际 %d", w.Code)
	}
	// 创建 -> 201
	if w := chgReq(r, http.MethodPost, "/api/v1/changes", `{"title":"升级DB","change_type":"normal","risk_level":"high"}`, cmTok); w.Code != http.StatusCreated {
		t.Fatalf("创建应 201，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 缺字段 -> 400
	if w := chgReq(r, http.MethodPost, "/api/v1/changes", `{"title":"x"}`, cmTok); w.Code != http.StatusBadRequest {
		t.Fatalf("缺字段应 400")
	}
	// 列表 -> 200
	if w := chgReq(r, http.MethodGet, "/api/v1/changes?status=draft&change_type=normal&risk_level=high&page=1", "", cmTok); w.Code != http.StatusOK {
		t.Fatalf("列表应 200，实际 %d", w.Code)
	}
	// 详情 -> 200
	if w := chgReq(r, http.MethodGet, "/api/v1/changes/1", "", cmTok); w.Code != http.StatusOK {
		t.Fatalf("详情应 200，实际 %d", w.Code)
	}
	// 非法 id -> 400
	if w := chgReq(r, http.MethodGet, "/api/v1/changes/abc", "", cmTok); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400")
	}
	// 不存在 -> 404
	if w := chgReq(r, http.MethodGet, "/api/v1/changes/999", "", cmTok); w.Code != http.StatusNotFound {
		t.Fatalf("不存在应 404")
	}
	// 编辑（补方案）-> 200
	if w := chgReq(r, http.MethodPut, "/api/v1/changes/1", `{"plan":"步骤","rollback_plan":"回滚"}`, cmTok); w.Code != http.StatusOK {
		t.Fatalf("编辑应 200，实际 %d", w.Code)
	}
	// submit_assessment -> 200（仅限变更创建者角色，此处由 change_manager 提交）
	if w := chgReq(r, http.MethodPost, "/api/v1/changes/1/transition", `{"action":"submit_assessment"}`, cmTok); w.Code != http.StatusOK {
		t.Fatalf("submit_assessment 应 200，实际 %d", w.Code)
	}
	// submit_approval -> 200（仅创建者本人/变更经理/管理员可推进；此处由创建者 cmTok 提交）
	if w := chgReq(r, http.MethodPost, "/api/v1/changes/1/transition", `{"action":"submit_approval"}`, cmTok); w.Code != http.StatusOK {
		t.Fatalf("submit_approval 应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 第 1 票 -> 200（仍 pending）
	if w := chgReq(r, http.MethodPost, "/api/v1/changes/1/approvals", `{"decision":"approved"}`, cmTok); w.Code != http.StatusOK {
		t.Fatalf("第 1 票应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 第 2 票 -> 200（approved）
	if w := chgReq(r, http.MethodPost, "/api/v1/changes/1/approvals", `{"decision":"approved"}`, cm2Tok); w.Code != http.StatusOK {
		t.Fatalf("第 2 票应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 审批记录 -> 200
	if w := chgReq(r, http.MethodGet, "/api/v1/changes/1/approvals", "", cmTok); w.Code != http.StatusOK {
		t.Fatalf("审批记录应 200，实际 %d", w.Code)
	}
	// 排期 -> 200
	ws := "2026-09-16T20:00:00Z"
	we := "2026-09-16T22:00:00Z"
	if w := chgReq(r, http.MethodPost, "/api/v1/changes/1/transition",
		`{"action":"schedule","window_start":"`+ws+`","window_end":"`+we+`"}`, cmTok); w.Code != http.StatusOK {
		t.Fatalf("排期应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 开始实施 -> 200
	if w := chgReq(r, http.MethodPost, "/api/v1/changes/1/transition", `{"action":"start_implement"}`, resTok); w.Code != http.StatusOK {
		t.Fatalf("开始实施应 200，实际 %d", w.Code)
	}
	// 实施成功 -> 200
	if w := chgReq(r, http.MethodPost, "/api/v1/changes/1/transition", `{"action":"complete","result":"已上线"}`, resTok); w.Code != http.StatusOK {
		t.Fatalf("实施成功应 200，实际 %d", w.Code)
	}
	// 回顾 -> 200
	if w := chgReq(r, http.MethodPost, "/api/v1/changes/1/transition", `{"action":"review"}`, cmTok); w.Code != http.StatusOK {
		t.Fatalf("回顾应 200，实际 %d", w.Code)
	}
	// 关闭 -> 200
	if w := chgReq(r, http.MethodPost, "/api/v1/changes/1/transition", `{"action":"close","conclusion":"验证通过"}`, cmTok); w.Code != http.StatusOK {
		t.Fatalf("关闭应 200，实际 %d", w.Code)
	}
	// 删除（admin）-> 200
	if w := chgReq(r, http.MethodDelete, "/api/v1/changes/1", "", adminTok); w.Code != http.StatusOK {
		t.Fatalf("删除应 200，实际 %d", w.Code)
	}
}

func TestChangeHTTP_ApprovalRBAC(t *testing.T) {
	r, jwt, _ := newChangeEngine(t)
	cmTok := chgToken(t, jwt, 7, role.ChangeManager)
	resTok := chgToken(t, jwt, 3, role.Resolver)

	chgReq(r, http.MethodPost, "/api/v1/changes", `{"title":"t","change_type":"normal","risk_level":"high"}`, cmTok)
	// resolver 无 approve 权限 -> 403
	if w := chgReq(r, http.MethodPost, "/api/v1/changes/1/approvals", `{"decision":"approved"}`, resTok); w.Code != http.StatusForbidden {
		t.Fatalf("resolver 审批应 403，实际 %d", w.Code)
	}
	// resolver 删除 -> 403
	if w := chgReq(r, http.MethodDelete, "/api/v1/changes/1", "", resTok); w.Code != http.StatusForbidden {
		t.Fatalf("resolver 删除应 403，实际 %d", w.Code)
	}
}
