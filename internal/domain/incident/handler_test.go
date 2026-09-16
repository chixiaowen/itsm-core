package incident

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

func newIncidentEngine(t *testing.T) (*gin.Engine, *security.JWTManager, *testEnv) {
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

func incToken(t *testing.T, m *security.JWTManager, id uint64, r string) string {
	t.Helper()
	tok, _, err := m.Issue(id, "u", r)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func incReq(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
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

func TestIncidentHTTP_Flow(t *testing.T) {
	r, jwt, _ := newIncidentEngine(t)
	adminTok := incToken(t, jwt, 9, role.Admin)
	agentTok := incToken(t, jwt, 7, role.Agent)
	reqTok := incToken(t, jwt, 20, role.Requestor)

	// 无 token -> 401
	if w := incReq(r, http.MethodGet, "/api/v1/incidents", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401，实际 %d", w.Code)
	}
	// 矩阵 / 看板 -> 200
	if w := incReq(r, http.MethodGet, "/api/v1/incidents/priority-matrix", "", reqTok); w.Code != http.StatusOK {
		t.Fatalf("矩阵应 200，实际 %d", w.Code)
	}
	if w := incReq(r, http.MethodGet, "/api/v1/incidents/stats", "", reqTok); w.Code != http.StatusOK {
		t.Fatalf("看板应 200，实际 %d", w.Code)
	}
	// 上报 -> 201
	if w := incReq(r, http.MethodPost, "/api/v1/incidents", `{"title":"宕机","impact":"high","urgency":"high"}`, reqTok); w.Code != http.StatusCreated {
		t.Fatalf("上报应 201，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 缺字段 -> 400
	if w := incReq(r, http.MethodPost, "/api/v1/incidents", `{"title":"x"}`, reqTok); w.Code != http.StatusBadRequest {
		t.Fatalf("缺字段应 400，实际 %d", w.Code)
	}
	// 列表 -> 200
	if w := incReq(r, http.MethodGet, "/api/v1/incidents?status=reported&escalation_level=0&page=1", "", agentTok); w.Code != http.StatusOK {
		t.Fatalf("列表应 200，实际 %d", w.Code)
	}
	// 详情 -> 200
	if w := incReq(r, http.MethodGet, "/api/v1/incidents/1", "", agentTok); w.Code != http.StatusOK {
		t.Fatalf("详情应 200，实际 %d", w.Code)
	}
	// 非法 id -> 400
	if w := incReq(r, http.MethodGet, "/api/v1/incidents/abc", "", agentTok); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400")
	}
	// 编辑 -> 200
	if w := incReq(r, http.MethodPut, "/api/v1/incidents/1", `{"title":"宕机2"}`, agentTok); w.Code != http.StatusOK {
		t.Fatalf("编辑应 200，实际 %d", w.Code)
	}
	// 覆盖优先级 -> 200
	if w := incReq(r, http.MethodPost, "/api/v1/incidents/1/priority", `{"priority":"P2"}`, agentTok); w.Code != http.StatusOK {
		t.Fatalf("覆盖优先级应 200，实际 %d", w.Code)
	}
	// 流转 triage -> 200
	if w := incReq(r, http.MethodPost, "/api/v1/incidents/1/transition", `{"action":"triage"}`, agentTok); w.Code != http.StatusOK {
		t.Fatalf("triage 应 200，实际 %d", w.Code)
	}
	// confirm 无 assignee -> 422
	if w := incReq(r, http.MethodPost, "/api/v1/incidents/1/transition", `{"action":"confirm"}`, agentTok); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("confirm 无指派应 422，实际 %d", w.Code)
	}
	// confirm -> 200
	if w := incReq(r, http.MethodPost, "/api/v1/incidents/1/transition", `{"action":"confirm","assignee_id":7}`, agentTok); w.Code != http.StatusOK {
		t.Fatalf("confirm 应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	// escalate -> 200
	if w := incReq(r, http.MethodPost, "/api/v1/incidents/1/escalate", `{"type":"functional","reason":"need expert"}`, agentTok); w.Code != http.StatusOK {
		t.Fatalf("升级应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 关联 CI -> 200
	if w := incReq(r, http.MethodPost, "/api/v1/incidents/1/cis", `{"ci_ids":[1,2]}`, agentTok); w.Code != http.StatusOK {
		t.Fatalf("关联 CI 应 200，实际 %d", w.Code)
	}
	// 解除 CI -> 200
	if w := incReq(r, http.MethodDelete, "/api/v1/incidents/1/cis/1", "", agentTok); w.Code != http.StatusOK {
		t.Fatalf("解除 CI 应 200，实际 %d", w.Code)
	}
	// 关联已有工单 -> 200
	if w := incReq(r, http.MethodPost, "/api/v1/incidents/1/link-ticket", `{"ticket_id":77}`, agentTok); w.Code != http.StatusOK {
		t.Fatalf("关联工单应 200，实际 %d", w.Code)
	}
	// 转工单 -> 201（先 take_over 回到 in_progress 才能转？escalated 不可转单，先 take_over）
	if w := incReq(r, http.MethodPost, "/api/v1/incidents/1/transition", `{"action":"take_over"}`, agentTok); w.Code != http.StatusOK {
		t.Fatalf("take_over 应 200，实际 %d", w.Code)
	}
	// 该事件已 link ticket，转单仍应允许（link 不阻止 convert，convert 只在 ticket_id 非空时 409）
	// 这里 ticket_id 已非空 -> 409
	if w := incReq(r, http.MethodPost, "/api/v1/incidents/1/convert-to-ticket", `{}`, agentTok); w.Code != http.StatusConflict {
		t.Fatalf("已关联工单转单应 409，实际 %d", w.Code)
	}
	// 删除（admin）-> 200
	if w := incReq(r, http.MethodDelete, "/api/v1/incidents/1", "", adminTok); w.Code != http.StatusOK {
		t.Fatalf("删除应 200，实际 %d", w.Code)
	}
}

func TestIncidentHTTP_ConvertAndRBAC(t *testing.T) {
	r, jwt, _ := newIncidentEngine(t)
	agentTok := incToken(t, jwt, 7, role.Agent)
	reqTok := incToken(t, jwt, 20, role.Requestor)

	// requestor 上报后，agent 转单成功 -> 201
	incReq(r, http.MethodPost, "/api/v1/incidents", `{"title":"t","impact":"low","urgency":"low"}`, reqTok)
	incReq(r, http.MethodPost, "/api/v1/incidents/1/transition", `{"action":"triage"}`, agentTok)
	incReq(r, http.MethodPost, "/api/v1/incidents/1/transition", `{"action":"confirm","assignee_id":7}`, agentTok)
	if w := incReq(r, http.MethodPost, "/api/v1/incidents/1/convert-to-ticket", `{"title":"工单"}`, agentTok); w.Code != http.StatusCreated {
		t.Fatalf("转单应 201，实际 %d body=%s", w.Code, w.Body.String())
	}
	// requestor 删除 -> 403（需 admin）
	if w := incReq(r, http.MethodDelete, "/api/v1/incidents/1", "", reqTok); w.Code != http.StatusForbidden {
		t.Fatalf("requestor 删除应 403，实际 %d", w.Code)
	}
	// requestor 编辑 -> 403
	if w := incReq(r, http.MethodPut, "/api/v1/incidents/1", `{"title":"x"}`, reqTok); w.Code != http.StatusForbidden {
		t.Fatalf("requestor 编辑应 403，实际 %d", w.Code)
	}
}
