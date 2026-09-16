package ticket

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

// newTicketEngine 装配最小 ticket HTTP 引擎（fake repo + JWT + Auth + RBAC）。
func newTicketEngine(t *testing.T) (*gin.Engine, *security.JWTManager, *testEnv) {
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

func tkToken(t *testing.T, m *security.JWTManager, id uint64, r string) string {
	t.Helper()
	tok, _, err := m.Issue(id, "u", r)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func tkReq(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
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

func TestTicketHTTP_FullFlow(t *testing.T) {
	r, jwt, _ := newTicketEngine(t)
	adminTok := tkToken(t, jwt, 9, role.Admin)
	agentTok := tkToken(t, jwt, 7, role.Agent)
	reqTok := tkToken(t, jwt, 20, role.Requestor)

	// 无 token -> 401
	if w := tkReq(r, http.MethodGet, "/api/v1/tickets", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401，实际 %d", w.Code)
	}
	// requestor 创建 -> 201
	w := tkReq(r, http.MethodPost, "/api/v1/tickets", `{"title":"账号问题","priority":"P3"}`, reqTok)
	if w.Code != http.StatusCreated {
		t.Fatalf("创建应 201，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 缺标题 -> 400
	if w := tkReq(r, http.MethodPost, "/api/v1/tickets", `{}`, reqTok); w.Code != http.StatusBadRequest {
		t.Fatalf("缺标题应 400，实际 %d", w.Code)
	}
	// 列表 -> 200
	if w := tkReq(r, http.MethodGet, "/api/v1/tickets?page=1&page_size=10&status=new", "", agentTok); w.Code != http.StatusOK {
		t.Fatalf("列表应 200，实际 %d", w.Code)
	}
	// 详情（含 ci/评论）-> 200
	if w := tkReq(r, http.MethodGet, "/api/v1/tickets/1?include_internal=true", "", agentTok); w.Code != http.StatusOK {
		t.Fatalf("详情应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 非法 id -> 400
	if w := tkReq(r, http.MethodGet, "/api/v1/tickets/abc", "", agentTok); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400")
	}
	// 不存在 -> 404
	if w := tkReq(r, http.MethodGet, "/api/v1/tickets/999", "", agentTok); w.Code != http.StatusNotFound {
		t.Fatalf("不存在应 404")
	}
	// 编辑 -> 200
	if w := tkReq(r, http.MethodPut, "/api/v1/tickets/1", `{"title":"账号问题2","priority":"P1"}`, agentTok); w.Code != http.StatusOK {
		t.Fatalf("编辑应 200，实际 %d", w.Code)
	}
	// 指派 -> 200
	if w := tkReq(r, http.MethodPost, "/api/v1/tickets/1/assign", `{"assignee_id":7}`, agentTok); w.Code != http.StatusOK {
		t.Fatalf("指派应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 流转 start -> 200
	if w := tkReq(r, http.MethodPost, "/api/v1/tickets/1/transition", `{"action":"start"}`, agentTok); w.Code != http.StatusOK {
		t.Fatalf("start 应 200，实际 %d", w.Code)
	}
	// 非法流转 new→in_progress 用另一张
	tkReq(r, http.MethodPost, "/api/v1/tickets", `{"title":"t2"}`, reqTok)
	if w := tkReq(r, http.MethodPost, "/api/v1/tickets/2/transition", `{"action":"start"}`, agentTok); w.Code != http.StatusConflict {
		t.Fatalf("非法流转应 409，实际 %d", w.Code)
	}
	// 挂起无原因 -> 422
	if w := tkReq(r, http.MethodPost, "/api/v1/tickets/1/transition", `{"action":"pending"}`, agentTok); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("挂起无原因应 422，实际 %d", w.Code)
	}
	// 解决无 solution -> 422
	if w := tkReq(r, http.MethodPost, "/api/v1/tickets/1/transition", `{"action":"resolve"}`, agentTok); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("无 solution 应 422，实际 %d", w.Code)
	}
	// 解决 -> 200
	if w := tkReq(r, http.MethodPost, "/api/v1/tickets/1/transition", `{"action":"resolve","solution":"已处理"}`, agentTok); w.Code != http.StatusOK {
		t.Fatalf("解决应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 评价 -> 200
	if w := tkReq(r, http.MethodPost, "/api/v1/tickets/1/rating", `{"rating":5,"comment":"good"}`, reqTok); w.Code != http.StatusOK {
		t.Fatalf("评价应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 评论 -> 201
	if w := tkReq(r, http.MethodPost, "/api/v1/tickets/1/comment", `{"content":"hi","is_internal":false}`, agentTok); w.Code != http.StatusCreated {
		t.Fatalf("评论应 201，实际 %d", w.Code)
	}
	// 关联 CI -> 200
	if w := tkReq(r, http.MethodPost, "/api/v1/tickets/1/cis", `{"ci_ids":[101,102]}`, agentTok); w.Code != http.StatusOK {
		t.Fatalf("关联 CI 应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 解除 CI -> 200
	if w := tkReq(r, http.MethodDelete, "/api/v1/tickets/1/cis/101", "", agentTok); w.Code != http.StatusOK {
		t.Fatalf("解除 CI 应 200，实际 %d", w.Code)
	}
	// 关闭 -> 200
	if w := tkReq(r, http.MethodPost, "/api/v1/tickets/1/transition", `{"action":"close"}`, agentTok); w.Code != http.StatusOK {
		t.Fatalf("关闭应 200，实际 %d", w.Code)
	}
	// 删除（admin）-> 200
	if w := tkReq(r, http.MethodDelete, "/api/v1/tickets/1", "", adminTok); w.Code != http.StatusOK {
		t.Fatalf("删除应 200，实际 %d", w.Code)
	}
}

func TestTicketHTTP_RatingRBAC(t *testing.T) {
	r, jwt, _ := newTicketEngine(t)
	// requestor 无 ticket.handle：不能关闭/关联 CI（RBAC 403）
	reqTok := tkToken(t, jwt, 20, role.Requestor)
	if w := tkReq(r, http.MethodPost, "/api/v1/tickets/1/cis", `{"ci_ids":[1]}`, reqTok); w.Code != http.StatusForbidden {
		t.Fatalf("requestor 关联 CI 应 403，实际 %d", w.Code)
	}
	if w := tkReq(r, http.MethodDelete, "/api/v1/tickets/1", "", reqTok); w.Code != http.StatusForbidden {
		t.Fatalf("requestor 删除应 403，实际 %d", w.Code)
	}
}

func TestTicketHTTP_Categories(t *testing.T) {
	r, jwt, _ := newTicketEngine(t)
	adminTok := tkToken(t, jwt, 9, role.Admin)
	agentTok := tkToken(t, jwt, 7, role.Agent)

	// 非 admin 创建分类 -> 403
	if w := tkReq(r, http.MethodPost, "/api/v1/ticket-categories", `{"name":"x"}`, agentTok); w.Code != http.StatusForbidden {
		t.Fatalf("非 admin 建分类应 403，实际 %d", w.Code)
	}
	// admin 创建 -> 201
	if w := tkReq(r, http.MethodPost, "/api/v1/ticket-categories", `{"name":"硬件故障","sort_order":1}`, adminTok); w.Code != http.StatusCreated {
		t.Fatalf("建分类应 201，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 列表 -> 200
	if w := tkReq(r, http.MethodGet, "/api/v1/ticket-categories", "", agentTok); w.Code != http.StatusOK {
		t.Fatalf("分类列表应 200")
	}
	// 更新 -> 200
	if w := tkReq(r, http.MethodPut, "/api/v1/ticket-categories/1", `{"name":"硬件类","sort_order":2}`, adminTok); w.Code != http.StatusOK {
		t.Fatalf("更新分类应 200，实际 %d", w.Code)
	}
	// 删除 -> 200
	if w := tkReq(r, http.MethodDelete, "/api/v1/ticket-categories/1", "", adminTok); w.Code != http.StatusOK {
		t.Fatalf("删除分类应 200，实际 %d", w.Code)
	}
}
