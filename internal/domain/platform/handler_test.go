package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/chixiaowen/itsm-core/internal/pkg/middleware"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
	"github.com/chixiaowen/itsm-core/internal/pkg/security"
)

// newPlatformEngine 装配一个最小可用的 platform HTTP 引擎（fake repo + JWT + Auth + RBAC）。
func newPlatformEngine(t *testing.T) (*gin.Engine, *security.JWTManager, *testEnv) {
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

func tokenFor(t *testing.T, m *security.JWTManager, id uint64, r string) string {
	t.Helper()
	tok, _, err := m.Issue(id, "u", r)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

func req(r *gin.Engine, method, path, body, token, contentType string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	w := httptest.NewRecorder()
	httpReq := httptest.NewRequest(method, path, reader)
	if contentType == "" {
		contentType = "application/json"
	}
	httpReq.Header.Set("Content-Type", contentType)
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}
	r.ServeHTTP(w, httpReq)
	return w
}

func TestPlatformHTTP_UserRBAC(t *testing.T) {
	r, jwt, _ := newPlatformEngine(t)
	admin := tokenFor(t, jwt, 100, role.Admin)
	requestor := tokenFor(t, jwt, 5, role.Requestor)

	// 无 token -> 401
	if w := req(r, http.MethodGet, "/api/v1/users", "", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401，实际 %d", w.Code)
	}
	// requestor -> 403
	if w := req(r, http.MethodGet, "/api/v1/users", "", requestor, ""); w.Code != http.StatusForbidden {
		t.Fatalf("requestor 应 403，实际 %d", w.Code)
	}

	// admin 创建用户 -> 201
	w := req(r, http.MethodPost, "/api/v1/users",
		`{"username":"carol","display_name":"Carol","role":"agent","password":"secret1"}`, admin, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("创建用户应 201，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 重复 -> 409
	if w := req(r, http.MethodPost, "/api/v1/users",
		`{"username":"carol","display_name":"Carol","role":"agent","password":"secret1"}`, admin, ""); w.Code != http.StatusConflict {
		t.Fatalf("重复应 409，实际 %d", w.Code)
	}
	// 参数缺失 -> 400
	if w := req(r, http.MethodPost, "/api/v1/users", `{}`, admin, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("缺参应 400，实际 %d", w.Code)
	}

	// 列表 -> 200
	if w := req(r, http.MethodGet, "/api/v1/users?page=1&page_size=10", "", admin, ""); w.Code != http.StatusOK {
		t.Fatalf("列表应 200，实际 %d", w.Code)
	}
	// 详情 -> 200
	if w := req(r, http.MethodGet, "/api/v1/users/1", "", admin, ""); w.Code != http.StatusOK {
		t.Fatalf("详情应 200，实际 %d", w.Code)
	}
	// 非法 id -> 400
	if w := req(r, http.MethodGet, "/api/v1/users/abc", "", admin, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 id 应 400，实际 %d", w.Code)
	}
	// 不存在 -> 404
	if w := req(r, http.MethodGet, "/api/v1/users/999", "", admin, ""); w.Code != http.StatusNotFound {
		t.Fatalf("不存在应 404，实际 %d", w.Code)
	}
	// 编辑 -> 200
	if w := req(r, http.MethodPut, "/api/v1/users/1", `{"display_name":"Carol 2"}`, admin, ""); w.Code != http.StatusOK {
		t.Fatalf("编辑应 200，实际 %d", w.Code)
	}
	// 删除自己（admin id=100 不存在，这里删 id=1）-> 200
	if w := req(r, http.MethodDelete, "/api/v1/users/1", "", admin, ""); w.Code != http.StatusOK {
		t.Fatalf("删除应 200，实际 %d", w.Code)
	}

	// 角色矩阵 -> 200
	if w := req(r, http.MethodGet, "/api/v1/roles", "", admin, ""); w.Code != http.StatusOK {
		t.Fatalf("角色矩阵应 200，实际 %d", w.Code)
	}
}

func TestPlatformHTTP_DeleteSelfForbidden(t *testing.T) {
	r, jwt, _ := newPlatformEngine(t)
	admin := tokenFor(t, jwt, 100, role.Admin)
	// 先建一个用户拿到 id=1，然后 admin 用 id=1 的 token 删除自己
	tok1 := tokenFor(t, jwt, 1, role.Admin)
	_ = admin
	if w := req(r, http.MethodDelete, "/api/v1/users/1", "", tok1, ""); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("删除自己应 422，实际 %d", w.Code)
	}
}

func TestPlatformHTTP_SLA(t *testing.T) {
	r, jwt, _ := newPlatformEngine(t)
	admin := tokenFor(t, jwt, 100, role.Admin)

	if w := req(r, http.MethodPost, "/api/v1/sla-policies",
		`{"name":"P1 策略","priority":"P1","response_minutes":15,"resolve_minutes":240,"pause_on_pending":true}`, admin, ""); w.Code != http.StatusCreated {
		t.Fatalf("创建 SLA 应 201，实际 %d body=%s", w.Code, w.Body.String())
	}
	if w := req(r, http.MethodPost, "/api/v1/sla-policies",
		`{"name":"dup","priority":"P1","response_minutes":1,"resolve_minutes":1}`, admin, ""); w.Code != http.StatusConflict {
		t.Fatalf("重复优先级应 409，实际 %d", w.Code)
	}
	if w := req(r, http.MethodGet, "/api/v1/sla-policies", "", admin, ""); w.Code != http.StatusOK {
		t.Fatalf("SLA 列表应 200，实际 %d", w.Code)
	}
	if w := req(r, http.MethodPut, "/api/v1/sla-policies/1",
		`{"name":"P1 更新","priority":"P1","response_minutes":20,"resolve_minutes":300}`, admin, ""); w.Code != http.StatusOK {
		t.Fatalf("更新 SLA 应 200，实际 %d", w.Code)
	}
	if w := req(r, http.MethodDelete, "/api/v1/sla-policies/1", "", admin, ""); w.Code != http.StatusOK {
		t.Fatalf("删除 SLA 应 200，实际 %d", w.Code)
	}
}

func TestPlatformHTTP_AuditAndComments(t *testing.T) {
	r, jwt, _ := newPlatformEngine(t)
	admin := tokenFor(t, jwt, 100, role.Admin)

	if w := req(r, http.MethodGet, "/api/v1/audit-logs?page=1", "", admin, ""); w.Code != http.StatusOK {
		t.Fatalf("审计查询应 200，实际 %d", w.Code)
	}
	// 评论：缺 biz_id -> 400
	if w := req(r, http.MethodGet, "/api/v1/comments?biz_type=ticket", "", admin, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("缺 biz_id 应 400，实际 %d", w.Code)
	}
	if w := req(r, http.MethodGet, "/api/v1/comments?biz_type=ticket&biz_id=1", "", admin, ""); w.Code != http.StatusOK {
		t.Fatalf("评论列表应 200，实际 %d", w.Code)
	}
	if w := req(r, http.MethodPost, "/api/v1/comments",
		`{"biz_type":"ticket","biz_id":1,"content":"hi","is_internal":false}`, admin, ""); w.Code != http.StatusCreated {
		t.Fatalf("创建评论应 201，实际 %d body=%s", w.Code, w.Body.String())
	}
	// 非法 biz_type -> 400
	if w := req(r, http.MethodPost, "/api/v1/comments",
		`{"biz_type":"nope","biz_id":1,"content":"hi"}`, admin, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 biz_type 应 400，实际 %d", w.Code)
	}
}

func TestPlatformHTTP_Attachments(t *testing.T) {
	r, jwt, _ := newPlatformEngine(t)
	admin := tokenFor(t, jwt, 100, role.Admin)

	// 构造 multipart
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	_ = mw.WriteField("biz_type", "ticket")
	_ = mw.WriteField("biz_id", "1")
	fw, err := mw.CreateFormFile("file", "note.txt")
	if err != nil {
		t.Fatalf("multipart: %v", err)
	}
	_, _ = fw.Write([]byte("hello attachment"))
	_ = mw.Close()

	w := req(r, http.MethodPost, "/api/v1/attachments", buf.String(), admin, mw.FormDataContentType())
	if w.Code != http.StatusCreated {
		t.Fatalf("上传应 201，实际 %d body=%s", w.Code, w.Body.String())
	}

	// 下载
	if w := req(r, http.MethodGet, "/api/v1/attachments/1/download", "", admin, ""); w.Code != http.StatusOK {
		t.Fatalf("下载应 200，实际 %d", w.Code)
	}
	// 删除
	if w := req(r, http.MethodDelete, "/api/v1/attachments/1", "", admin, ""); w.Code != http.StatusOK {
		t.Fatalf("删除附件应 200，实际 %d", w.Code)
	}
	// 缺文件字段 -> 400
	if w := req(r, http.MethodPost, "/api/v1/attachments", "not-multipart", admin, "application/json"); w.Code != http.StatusBadRequest {
		t.Fatalf("缺文件应 400，实际 %d", w.Code)
	}
}

func TestPlatformHTTP_JSONRoundTrip(t *testing.T) {
	// 覆盖 User 的 JSON 序列化（PasswordHash 不外泄）。
	u := User{ID: 1, Username: "a", PasswordHash: "hash"}
	b, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(b, []byte("hash")) {
		t.Fatalf("PasswordHash 不应出现在 JSON 中: %s", string(b))
	}
}

// TestPlatformHTTP_UserOptions 覆盖 GET /users/options：
// 任意登录用户可访问（不挂 RequirePerm）、按 role 过滤、响应不含敏感字段。
func TestPlatformHTTP_UserOptions(t *testing.T) {
	r, jwt, env := newPlatformEngine(t)
	ctx := context.Background()

	// 造数据：2 个 agent（1 个禁用）+ 1 个 admin
	if _, err := env.svc.CreateUser(ctx, 1, CreateUserRequest{Username: "oa", DisplayName: "A1", Role: role.Agent, Password: "secret1", Email: "a1@x.com"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	a2, err := env.svc.CreateUser(ctx, 1, CreateUserRequest{Username: "ob", DisplayName: "A2", Role: role.Agent, Password: "secret1", Email: "a2@x.com"})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := env.svc.CreateUser(ctx, 1, CreateUserRequest{Username: "ad", DisplayName: "AD", Role: role.Admin, Password: "secret1"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	dis := UserStatusDisabled
	if _, err := env.svc.UpdateUser(ctx, 1, a2.ID, UpdateUserRequest{Status: &dis}); err != nil {
		t.Fatalf("disable: %v", err)
	}

	requestor := tokenFor(t, jwt, 5, role.Requestor)

	// 无 token -> 401
	if w := req(r, http.MethodGet, "/api/v1/users/options", "", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("无 token 应 401，实际 %d", w.Code)
	}

	// requestor（任意登录用户，无 UserManage 权限）-> 200（不挂 RequirePerm）
	w := req(r, http.MethodGet, "/api/v1/users/options", "", requestor, "")
	if w.Code != http.StatusOK {
		t.Fatalf("任意登录用户应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, sensitive := range []string{"password_hash", "password", "email", "secret1"} {
		if strings.Contains(body, sensitive) {
			t.Fatalf("响应含敏感字段 %s: %s", sensitive, body)
		}
	}

	var resp struct {
		Code int          `json:"code"`
		Data []UserOption `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if resp.Code != 0 {
		t.Fatalf("code 应为 0，实际 %d", resp.Code)
	}
	// 仅 active：oa + ad = 2
	if len(resp.Data) != 2 {
		t.Fatalf("active 用户应 2 个，实际 %d (%+v)", len(resp.Data), resp.Data)
	}

	// role=agent 过滤 -> 仅 1 个
	w = req(r, http.MethodGet, "/api/v1/users/options?role=agent", "", requestor, "")
	if w.Code != http.StatusOK {
		t.Fatalf("role 过滤应 200，实际 %d", w.Code)
	}
	resp.Data = nil
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Role != role.Agent {
		t.Fatalf("agent 过滤应 1 个，实际 %+v", resp.Data)
	}
}

// TestPlatformHTTP_AuditEntityFilter 覆盖 GET /audit-logs 的 entity_type/entity_id 过滤。
func TestPlatformHTTP_AuditEntityFilter(t *testing.T) {
	r, jwt, env := newPlatformEngine(t)
	ctx := context.Background()
	_ = env.svc.AppendAudit(ctx, AuditEntry{ActorID: 1, Action: "transition", BizType: "ticket", BizID: 7, FromStatus: "new", ToStatus: "assigned"})
	_ = env.svc.AppendAudit(ctx, AuditEntry{ActorID: 1, Action: "transition", BizType: "ticket", BizID: 8, FromStatus: "assigned", ToStatus: "resolved"})

	admin := tokenFor(t, jwt, 100, role.Admin)
	w := req(r, http.MethodGet, "/api/v1/audit-logs?entity_type=ticket&entity_id=7", "", admin, "")
	if w.Code != http.StatusOK {
		t.Fatalf("审计查询应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Total int64      `json:"total"`
			Items []AuditLog `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if resp.Data.Total != 1 || len(resp.Data.Items) != 1 {
		t.Fatalf("entity 过滤应 1 条，实际 total=%d", resp.Data.Total)
	}
	if resp.Data.Items[0].BizID != 7 || resp.Data.Items[0].ToStatus != "assigned" {
		t.Fatalf("过滤结果/status 字段不符: %+v", resp.Data.Items[0])
	}
}

// TestPlatformHTTP_AuditAccessControl 覆盖 /audit-logs 的实体范围访问控制：
// 全局浏览仅 admin（perm.audit.view）；带实体范围（type+id 同时提供）任意登录用户可读。
func TestPlatformHTTP_AuditAccessControl(t *testing.T) {
	r, jwt, env := newPlatformEngine(t)
	ctx := context.Background()

	// 造数据：ticket#7 两条、ticket#8 一条
	_ = env.svc.AppendAudit(ctx, AuditEntry{ActorID: 1, Action: "transition", BizType: "ticket", BizID: 7, FromStatus: "new", ToStatus: "assigned"})
	_ = env.svc.AppendAudit(ctx, AuditEntry{ActorID: 1, Action: "transition", BizType: "ticket", BizID: 7, FromStatus: "assigned", ToStatus: "resolved"})
	_ = env.svc.AppendAudit(ctx, AuditEntry{ActorID: 1, Action: "transition", BizType: "ticket", BizID: 8, FromStatus: "new", ToStatus: "assigned"})

	admin := tokenFor(t, jwt, 100, role.Admin)
	requestor := tokenFor(t, jwt, 5, role.Requestor)

	// 1) 无实体参数 + requestor -> 403
	if w := req(r, http.MethodGet, "/api/v1/audit-logs?page=1", "", requestor, ""); w.Code != http.StatusForbidden {
		t.Fatalf("requestor 全局审计应 403，实际 %d", w.Code)
	}
	// 2) 无实体参数 + admin -> 200
	if w := req(r, http.MethodGet, "/api/v1/audit-logs?page=1", "", admin, ""); w.Code != http.StatusOK {
		t.Fatalf("admin 全局审计应 200，实际 %d", w.Code)
	}

	// 3) entity_type + entity_id + requestor -> 200，且仅返回该实体记录
	w := req(r, http.MethodGet, "/api/v1/audit-logs?entity_type=ticket&entity_id=7", "", requestor, "")
	if w.Code != http.StatusOK {
		t.Fatalf("requestor 实体范围审计应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Total int64      `json:"total"`
			Items []AuditLog `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if resp.Data.Total != 2 || len(resp.Data.Items) != 2 {
		t.Fatalf("实体范围应仅 2 条，实际 total=%d len=%d", resp.Data.Total, len(resp.Data.Items))
	}
	for _, it := range resp.Data.Items {
		if it.BizType != "ticket" || it.BizID != 7 {
			t.Fatalf("返回越界记录: %+v", it)
		}
	}

	// 4) 只带 entity_type（无 id）+ requestor -> 403（防全表枚举）
	if w := req(r, http.MethodGet, "/api/v1/audit-logs?entity_type=ticket", "", requestor, ""); w.Code != http.StatusForbidden {
		t.Fatalf("仅 entity_type 应 403，实际 %d", w.Code)
	}

	// 附加：biz_type + biz_id 别名同样对任意登录用户开放
	if w := req(r, http.MethodGet, "/api/v1/audit-logs?biz_type=ticket&biz_id=8", "", requestor, ""); w.Code != http.StatusOK {
		t.Fatalf("biz_* 别名应 200，实际 %d", w.Code)
	}
}
