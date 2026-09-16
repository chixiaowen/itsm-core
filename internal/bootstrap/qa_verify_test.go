// 本文件是 QA 工程师的**独立对抗性验证**用例（独立于 T11 开发自测），
// 复用 integration_test.go 的同包脚手架（openTestDB / testConfig / client 等），
// 但**不修改** integration_test.go，也不修改任何业务源码。
//
// 设计原则：
//   - 每个用例都从外部（真实 HTTP + 真实 GORM + 嵌入式 SQLite）攻击被测实现，
//     而非复述开发自写的断言；
//   - 断言一律对齐 docs/PRD.md（§2.2 权限矩阵 / §5 状态机 / §5.2.1 SLA 口径）与
//     docs/API.md，**绝不放宽断言去迎合实现**；
//   - 若某断言期望「PRD 允许的行为」而实现返回拒绝，或期望「PRD 禁止的行为」而实现放行，
//     则视为**真实缺陷**并以 t.Errorf 记录（见 TestQA_PRD_Deviations），供转交域负责人。
package bootstrap_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/chixiaowen/itsm-core/internal/bootstrap"
)

// qaEnv 构造一套独立环境：全新内存库 + 迁移 + 种子 + 完整装配 + gin 引擎。
func qaEnv(t *testing.T) (*client, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := zap.NewNop()
	db := openTestDB(t)
	if err := bootstrap.Migrate(db); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if err := bootstrap.Seed(db, log); err != nil {
		t.Fatalf("种子失败: %v", err)
	}
	cfg := testConfig(t)
	app, err := bootstrap.New(cfg, log, db)
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	engine := bootstrap.BuildEngine(cfg, log, db, app)
	return &client{t: t, engine: engine}, db
}

// raw 发送原始请求体（用于构造非法 JSON 等场景，绕过 json.Marshal）。
func (c *client) raw(method, path, token, rawBody string) *httptest.ResponseRecorder {
	c.t.Helper()
	var r io.Reader
	if rawBody != "" {
		r = strings.NewReader(rawBody)
	}
	req := httptest.NewRequest(method, path, r)
	if rawBody != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	c.engine.ServeHTTP(rec, req)
	return rec
}

// qaParseTime 解析 RFC3339 / RFC3339Nano 时间字符串。
func qaParseTime(t *testing.T, v any) time.Time {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		t.Fatalf("期望时间字符串，实际 %T=%v", v, v)
	}
	tt, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		tt, err = time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatalf("解析时间失败 %q: %v", s, err)
		}
	}
	return tt
}

// qaPage 从统一分页响应中取字段（page/page_size/total）。
func qaPageField(t *testing.T, rec *httptest.ResponseRecorder, key string) float64 {
	t.Helper()
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析分页响应失败: %v；raw=%s", err, rec.Body.String())
	}
	v, ok := body.Data[key].(float64)
	if !ok {
		t.Fatalf("分页字段 %q 不是数字: %v", key, body.Data)
	}
	return v
}

// TestQA_SeedIdempotency 回归锁：Migrate + Seed 连跑两次，断言无重复键错误、
// 8 个账号存在、username 不落空串（对齐已修缺陷 D1：Where(字符串).Attrs() 导致 username 为空）。
func TestQA_SeedIdempotency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := zap.NewNop()
	db := openTestDB(t)

	// 迁移两遍（幂等）。
	for i := 0; i < 2; i++ {
		if err := bootstrap.Migrate(db); err != nil {
			t.Fatalf("第 %d 次迁移失败: %v", i+1, err)
		}
	}
	// 种子两遍（幂等，第 2 次不得因唯一索引冲突报错）。
	for i := 0; i < 2; i++ {
		if err := bootstrap.Seed(db, log); err != nil {
			t.Fatalf("第 %d 次种子失败（疑似 D1 幂等回归）: %v", i+1, err)
		}
	}

	// 直查库：用户总数应为 8，且不存在 username='' 的脏数据。
	var total int64
	if err := db.Table("users").Where("deleted_at IS NULL").Count(&total).Error; err != nil {
		t.Fatalf("统计用户失败: %v", err)
	}
	if total != 8 {
		t.Errorf("期望 8 个种子账号，实际 %d", total)
	}
	var blank int64
	if err := db.Table("users").Where("username = ''").Count(&blank).Error; err != nil {
		t.Fatalf("统计空用户名失败: %v", err)
	}
	if blank != 0 {
		t.Errorf("存在 %d 条 username 为空串的记录（D1 未修复）", blank)
	}

	// 黑盒确认：admin 可登录（D1 的症状是 admin 无法登录）。
	cfg := testConfig(t)
	app, err := bootstrap.New(cfg, log, db)
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	engine := bootstrap.BuildEngine(cfg, log, db, app)
	c := &client{t: t, engine: engine}
	admin := c.login("admin", "admin123")
	if admin.Token == "" {
		t.Fatalf("admin 登录未返回 token")
	}
}

// TestQA_DemoAccountsLoginAndPermissions 8 个账号逐一真实登录 + 权限边界交叉验证。
func TestQA_DemoAccountsLoginAndPermissions(t *testing.T) {
	c, _ := qaEnv(t)

	accounts := []string{"admin", "requestor01", "agent01", "resolver01", "pm01", "cm01", "cm02", "cmdb01"}
	tokens := map[string]string{}
	for _, u := range accounts {
		tok := c.login(u, "admin123").Token
		if tok == "" {
			t.Errorf("账号 %s 登录未返回 token", u)
		}
		tokens[u] = tok
	}

	// requestor01 调管理员接口 /users → 403。
	rec := c.do(http.MethodGet, "/api/v1/users", tokens["requestor01"], nil)
	c.mustStatus(rec, http.StatusForbidden)

	// cmdb01 调变更审批 → 403（缺 perm.change.approve）。
	rec = c.do(http.MethodPost, "/api/v1/changes/999/approvals", tokens["cmdb01"],
		map[string]any{"decision": "approved", "comment": "x"})
	c.mustStatus(rec, http.StatusForbidden)

	// 反向：admin 调管理员接口 → 200。
	rec = c.do(http.MethodGet, "/api/v1/users?page_size=100", tokens["admin"], nil)
	c.mustStatus(rec, http.StatusOK)
	if got := qaPageField(t, rec, "total"); got != 8 {
		t.Errorf("用户目录 total 期望 8，实际 %v", got)
	}
}

// TestQA_CabSingleVoterBypass 独立复现 CAB 单人绕过防护（不依赖开发自写用例）。
func TestQA_CabSingleVoterBypass(t *testing.T) {
	c, _ := qaEnv(t)
	cm01 := c.login("cm01", "admin123")
	cm02 := c.login("cm02", "admin123")
	admin := c.login("admin", "admin123")

	// 普通变更 → 提交风险评估 → 提交审批（pending_approval）。
	rec := c.do(http.MethodPost, "/api/v1/changes", cm01.Token, map[string]any{
		"title": "QA-普通变更", "change_type": "normal", "risk_level": "medium",
	})
	c.mustStatus(rec, http.StatusCreated)
	id := idOf(t, c.obj(rec), "id")

	rec = c.do(http.MethodPut, fmt.Sprintf("/api/v1/changes/%d", id), cm01.Token, map[string]any{
		"plan": "QA plan", "rollback_plan": "QA rollback",
	})
	c.mustStatus(rec, http.StatusOK)

	// 注：draft→assessment（提交风险评估）当前仅 role.Requestor/Admin 可执行，
	// 故此处用 admin 推进到 assessment（该角色映射问题详见 TestQA_PRD_Deviations）。
	rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/changes/%d/transition", id), admin.Token,
		map[string]any{"action": "submit_assessment"})
	c.mustStatus(rec, http.StatusOK)
	rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/changes/%d/transition", id), cm01.Token,
		map[string]any{"action": "submit_approval"})
	c.mustStatus(rec, http.StatusOK)
	if s := c.obj(rec)["status"]; s != "pending_approval" {
		t.Fatalf("提交审批后应为 pending_approval，实际 %v", s)
	}

	// 同一审批人第 1 票：记录成功（200），但因普通变更需 2 人，状态仍待审。
	rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/changes/%d/approvals", id), cm01.Token,
		map[string]any{"decision": "approved", "comment": "同意"})
	c.mustStatus(rec, http.StatusOK)

	// 同一审批人第 2 票：必须 409（单人不可凑票）。
	rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/changes/%d/approvals", id), cm01.Token,
		map[string]any{"decision": "approved", "comment": "再投一次"})
	c.mustStatus(rec, http.StatusConflict)

	// 状态仍为 pending_approval。
	rec = c.do(http.MethodGet, fmt.Sprintf("/api/v1/changes/%d", id), cm01.Token, nil)
	c.mustStatus(rec, http.StatusOK)
	if s := c.obj(rec)["status"]; s != "pending_approval" {
		t.Fatalf("单人重复投票后状态应仍为 pending_approval，实际 %v", s)
	}

	// 第二位审批人投通过 → approved（会签达成）。
	rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/changes/%d/approvals", id), cm02.Token,
		map[string]any{"decision": "approved", "comment": "同意"})
	c.mustStatus(rec, http.StatusOK)
	if s := c.obj(rec)["status"]; s != "approved" {
		t.Fatalf("2 名审批人全部通过后应为 approved，实际 %v", s)
	}
}

// TestQA_TicketSLA 对齐 PRD §5.2.1：P1=4h / P4=72h 解决目标；挂起暂停并回补。
func TestQA_TicketSLA(t *testing.T) {
	c, db := qaEnv(t)
	requestor := c.login("requestor01", "admin123")
	agent := c.login("agent01", "admin123")

	// P1 → 解决目标 4h（240min）。
	rec := c.do(http.MethodPost, "/api/v1/tickets", requestor.Token,
		map[string]any{"title": "QA-P1", "priority": "P1"})
	c.mustStatus(rec, http.StatusCreated)
	o := c.obj(rec)
	created := qaParseTime(t, o["created_at"])
	dueP1 := qaParseTime(t, o["resolve_due_at"])
	if d := dueP1.Sub(created); d < 239*time.Minute || d > 241*time.Minute {
		t.Errorf("P1 解决目标应约 240min，实际 %v", d)
	}

	// P4 → 解决目标 72h（4320min）。
	rec = c.do(http.MethodPost, "/api/v1/tickets", requestor.Token,
		map[string]any{"title": "QA-P4", "priority": "P4"})
	c.mustStatus(rec, http.StatusCreated)
	o = c.obj(rec)
	created = qaParseTime(t, o["created_at"])
	dueP4 := qaParseTime(t, o["resolve_due_at"])
	if d := dueP4.Sub(created); d < 4319*time.Minute || d > 4321*time.Minute {
		t.Errorf("P4 解决目标应约 4320min，实际 %v", d)
	}

	// 挂起/恢复：paused_minutes 增长且 due_at 被回补。
	rec = c.do(http.MethodPost, "/api/v1/tickets", requestor.Token,
		map[string]any{"title": "QA-P3-pause", "priority": "P3"})
	c.mustStatus(rec, http.StatusCreated)
	o = c.obj(rec)
	tkID := idOf(t, o, "id")
	rd0 := qaParseTime(t, o["resolve_due_at"])

	// new → assigned → in_progress → pending。
	rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/assign", tkID), agent.Token,
		map[string]any{"assignee_id": agent.ID})
	c.mustStatus(rec, http.StatusOK)
	rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/transition", tkID), agent.Token,
		map[string]any{"action": "start"})
	c.mustStatus(rec, http.StatusOK)
	rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/transition", tkID), agent.Token,
		map[string]any{"action": "pending", "reason": "等待备件"})
	c.mustStatus(rec, http.StatusOK)
	if s := c.obj(rec)["status"]; s != "pending" {
		t.Fatalf("挂起后应为 pending，实际 %v", s)
	}

	// 把 paused_at 回拨 90 分钟，模拟真实挂起时长（不改业务代码，仅操纵测试库数据）。
	if err := db.Exec("UPDATE tickets SET paused_at = ? WHERE id = ?",
		time.Now().UTC().Add(-90*time.Minute), tkID).Error; err != nil {
		t.Fatalf("回拨 paused_at 失败: %v", err)
	}

	// pending → in_progress。
	rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/transition", tkID), agent.Token,
		map[string]any{"action": "resume"})
	c.mustStatus(rec, http.StatusOK)
	o = c.obj(rec)
	if s := o["status"]; s != "in_progress" {
		t.Fatalf("恢复后应为 in_progress，实际 %v", s)
	}
	pm, _ := o["paused_minutes"].(float64)
	if pm < 60 {
		t.Errorf("恢复后 paused_minutes 应约为 90，实际 %v", o["paused_minutes"])
	}
	rd1 := qaParseTime(t, o["resolve_due_at"])
	if grow := rd1.Sub(rd0); grow < 60*time.Minute {
		t.Errorf("恢复后 resolve_due_at 应被回补（约 +90min），实际增长 %v", grow)
	}
}

// TestQA_BoundariesAndErrorCodes 边界与错误码。
func TestQA_BoundariesAndErrorCodes(t *testing.T) {
	c, _ := qaEnv(t)
	admin := c.login("admin", "admin123")
	requestor := c.login("requestor01", "admin123")

	// 不存在的资源 → 404。
	rec := c.do(http.MethodGet, "/api/v1/tickets/999999", admin.Token, nil)
	c.mustStatus(rec, http.StatusNotFound)

	// 非法 JSON → 400。
	rec = c.raw(http.MethodPost, "/api/v1/tickets", requestor.Token, "{not-json")
	c.mustStatus(rec, http.StatusBadRequest)

	// page_size=100000 → 被钳制为 100（不得返回全表）。
	rec = c.do(http.MethodGet, "/api/v1/tickets?page_size=100000", admin.Token, nil)
	c.mustStatus(rec, http.StatusOK)
	if ps := qaPageField(t, rec, "page_size"); ps != 100 {
		t.Errorf("page_size 上限应钳制为 100，实际 %v", ps)
	}

	// 无 token → 401。
	rec = c.do(http.MethodGet, "/api/v1/tickets", "", nil)
	c.mustStatus(rec, http.StatusUnauthorized)

	// 不存在的路径 → 404。
	rec = c.do(http.MethodGet, "/api/v1/no-such-endpoint", admin.Token, nil)
	c.mustStatus(rec, http.StatusNotFound)
}

// TestQA_AuditTrail 状态流转后审计可查且含 from_status/to_status（实体范围访问控制）。
func TestQA_AuditTrail(t *testing.T) {
	c, _ := qaEnv(t)
	requestor := c.login("requestor01", "admin123")
	agent := c.login("agent01", "admin123")

	rec := c.do(http.MethodPost, "/api/v1/tickets", requestor.Token, map[string]any{"title": "QA-审计"})
	c.mustStatus(rec, http.StatusCreated)
	tkID := idOf(t, c.obj(rec), "id")

	// 制造一次状态流转 new → assigned。
	rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/assign", tkID), agent.Token,
		map[string]any{"assignee_id": agent.ID})
	c.mustStatus(rec, http.StatusOK)

	// requestor（非 admin）带实体范围查审计 → 200。
	rec = c.do(http.MethodGet,
		fmt.Sprintf("/api/v1/audit-logs?entity_type=ticket&entity_id=%d", tkID), requestor.Token, nil)
	c.mustStatus(rec, http.StatusOK)
	var body struct {
		Data struct {
			Total int64            `json:"total"`
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析审计失败: %v", err)
	}
	if body.Data.Total < 1 {
		t.Fatalf("工单 %d 应有 ≥1 条审计，实际 %d", tkID, body.Data.Total)
	}
	foundTransition := false
	for _, it := range body.Data.Items {
		if it["from_status"] == "new" && it["to_status"] == "assigned" {
			foundTransition = true
		}
	}
	if !foundTransition {
		t.Errorf("审计项应含 from_status=new / to_status=assigned，实际 items=%v", body.Data.Items)
	}

	// 不带实体范围的全局审计（requestor）→ 403。
	rec = c.do(http.MethodGet, "/api/v1/audit-logs", requestor.Token, nil)
	c.mustStatus(rec, http.StatusForbidden)
}

// TestQA_IncidentPriorityMatrix 对齐 PRD §5.3.1（至少 3 组，含中间值）。
func TestQA_IncidentPriorityMatrix(t *testing.T) {
	c, _ := qaEnv(t)
	agent := c.login("agent01", "admin123")

	cases := []struct {
		impact, urgency, want string
	}{
		{"high", "high", "P1"},
		{"low", "low", "P4"},
		{"medium", "medium", "P3"},
		{"high", "low", "P3"},
		{"medium", "high", "P2"},
	}
	for _, tc := range cases {
		rec := c.do(http.MethodPost, "/api/v1/incidents", agent.Token, map[string]any{
			"title": "QA-矩阵", "impact": tc.impact, "urgency": tc.urgency,
		})
		c.mustStatus(rec, http.StatusCreated)
		if got := c.obj(rec)["priority"]; got != tc.want {
			t.Errorf("影响=%s×紧急=%s 应定级 %s，实际 %v", tc.impact, tc.urgency, tc.want, got)
		}
	}
}

// TestQA_CMDBRelationConstraints 自环 400、重复 409。
func TestQA_CMDBRelationConstraints(t *testing.T) {
	c, _ := qaEnv(t)
	cmdb01 := c.login("cmdb01", "admin123")

	rec := c.do(http.MethodPost, "/api/v1/cis", cmdb01.Token,
		map[string]any{"code": "qa-ci-a", "name": "QA-A", "ci_type": "server"})
	c.mustStatus(rec, http.StatusCreated)
	a := idOf(t, c.obj(rec), "id")
	rec = c.do(http.MethodPost, "/api/v1/cis", cmdb01.Token,
		map[string]any{"code": "qa-ci-b", "name": "QA-B", "ci_type": "database"})
	c.mustStatus(rec, http.StatusCreated)
	b := idOf(t, c.obj(rec), "id")

	// 自环 → 400。
	rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/cis/%d/relations", a), cmdb01.Token,
		map[string]any{"target_ci_id": a, "relation_type": "depends_on"})
	c.mustStatus(rec, http.StatusBadRequest)

	// 正常 → 201。
	rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/cis/%d/relations", a), cmdb01.Token,
		map[string]any{"target_ci_id": b, "relation_type": "depends_on"})
	c.mustStatus(rec, http.StatusCreated)

	// 重复 → 409。
	rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/cis/%d/relations", a), cmdb01.Token,
		map[string]any{"target_ci_id": b, "relation_type": "depends_on"})
	c.mustStatus(rec, http.StatusConflict)
}

// TestQA_SoftDelete 删除后列表不可见、详情 404、审计仍可查。
func TestQA_SoftDelete(t *testing.T) {
	c, _ := qaEnv(t)
	cmdb01 := c.login("cmdb01", "admin123")

	rec := c.do(http.MethodPost, "/api/v1/cis", cmdb01.Token,
		map[string]any{"code": "qa-del-ci", "name": "QA-待删", "ci_type": "server"})
	c.mustStatus(rec, http.StatusCreated)
	id := idOf(t, c.obj(rec), "id")

	rec = c.do(http.MethodDelete, fmt.Sprintf("/api/v1/cis/%d", id), cmdb01.Token, nil)
	// 注：httpx.NoContent 有意返回 200 + {code:0,data:null}（保持统一响应体，供前端拦截器解析），
	// 而非 HTTP 204（见 internal/pkg/httpx/response.go）。
	c.mustStatus(rec, http.StatusOK)

	// 详情 → 404。
	rec = c.do(http.MethodGet, fmt.Sprintf("/api/v1/cis/%d", id), cmdb01.Token, nil)
	c.mustStatus(rec, http.StatusNotFound)

	// 列表不再返回。
	rec = c.do(http.MethodGet, "/api/v1/cis?page_size=100", cmdb01.Token, nil)
	c.mustStatus(rec, http.StatusOK)
	var ciPage struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ciPage); err != nil {
		t.Fatalf("解析 CI 列表失败: %v", err)
	}
	for _, it := range ciPage.Data.Items {
		if idOf(t, it, "id") == id {
			t.Errorf("软删除后 CI %d 仍出现在列表中", id)
		}
	}

	// 审计仍可查（entity_type=ci）。
	rec = c.do(http.MethodGet,
		fmt.Sprintf("/api/v1/audit-logs?entity_type=ci&entity_id=%d", id), cmdb01.Token, nil)
	c.mustStatus(rec, http.StatusOK)
	if total := qaPageField(t, rec, "total"); total < 1 {
		t.Errorf("软删除后审计应仍可追溯，实际 total=%v", total)
	}
}

// TestQA_PRD_Deviations 记录**与 PRD 明确不符**的行为。
//
// 本用例故意按 PRD 期望断言；若实现与之不符，则断言失败——**失败即真实缺陷**，
// 禁止放宽断言掩盖。发现后应转交对应域负责人修复（勿由 QA 改业务代码）。
func TestQA_PRD_Deviations(t *testing.T) {
	c, _ := qaEnv(t)
	agent := c.login("agent01", "admin123")
	resolver := c.login("resolver01", "admin123")
	pm := c.login("pm01", "admin123")
	admin := c.login("admin", "admin123")
	requestor := c.login("requestor01", "admin123")

	// 缺陷 #1（P1）：PRD §2.2「事件升级」与 §5.3 状态机均明确 problem_manager 可升级
	// （`in_progress` | 升级 | agent, resolver, problem_manager, admin | `escalated`），
	// 且其已持有 perm.incident.escalate / 路由放行；
	// 但 incidentMachine 的 ActionEscalate 仅允许 role.Resolvers(agent/resolver/admin)，
	// 导致 problem_manager 被状态机拒绝（403）——路由授权与状态机授权不一致。
	t.Run("problem_manager_escalate_PRD_allows", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/v1/incidents", agent.Token, map[string]any{
			"title": "QA-升级偏差", "impact": "high", "urgency": "high",
		})
		c.mustStatus(rec, http.StatusCreated)
		incID := idOf(t, c.obj(rec), "id")

		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/incidents/%d/transition", incID), agent.Token,
			map[string]any{"action": "triage"})
		c.mustStatus(rec, http.StatusOK)
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/incidents/%d/transition", incID), agent.Token,
			map[string]any{"action": "confirm", "assignee_id": resolver.ID})
		c.mustStatus(rec, http.StatusOK)
		if s := c.obj(rec)["status"]; s != "in_progress" {
			t.Fatalf("确认后应为 in_progress，实际 %v", s)
		}

		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/incidents/%d/escalate", incID), pm.Token,
			map[string]any{"type": "functional", "reason": "QA 权限偏差验证"})
		if rec.Code != http.StatusOK {
			t.Errorf("[P1 偏差] problem_manager 升级事件：PRD/路由期望 200，实际 %d；body=%s",
				rec.Code, rec.Body.String())
		}
	})

	// 缺陷 #2（P1）：PRD §5.5 状态机规定 `draft` | 提交风险评估 | **requester(工程师)** | `assessment`，
	// 而「变更申请人」即创建者（agent/resolver/change_manager 等持 perm.change.submit 的角色；
	// 注：role.requestor 本身无 perm.change.submit，无法创建变更）。
	// 但 changeMachine.StatusDraft.ActionSubmitAssessment 仅允许 `role.Requestor` 与 admin，
	// 既排除真正的申请人（agent/resolver），也排除 change_manager——导致该步**实际仅 admin 可执行**，
	// 非 admin 走不通「创建变更 → 提交风险评估」这一必经步骤。
	t.Run("change_requester_submit_assessment_PRD_allows", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/v1/changes", agent.Token, map[string]any{
			"title": "QA-工程师变更", "change_type": "normal", "risk_level": "low",
		})
		c.mustStatus(rec, http.StatusCreated)
		chID := idOf(t, c.obj(rec), "id")

		rec = c.do(http.MethodPut, fmt.Sprintf("/api/v1/changes/%d", chID), agent.Token, map[string]any{
			"plan": "QA plan", "rollback_plan": "QA rollback",
		})
		c.mustStatus(rec, http.StatusOK)

		// 申请人（= 工程师/创建者）提交风险评估，PRD 期望 200。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/changes/%d/transition", chID), agent.Token,
			map[string]any{"action": "submit_assessment"})
		if rec.Code != http.StatusOK {
			t.Errorf("[P1 偏差] 变更申请人提交风险评估：PRD/§5.5 期望 200，实际 %d；body=%s",
				rec.Code, rec.Body.String())
		}
	})

	// 缺陷 #3（P2）：PRD §2.2「满意度评价」仅 requestor 为 ✓，admin 为 ✗；
	// 但 role.go 将 PermTicketRate 授予 admin，且 ticket.Service.Rate 显式放行 admin，
	// 导致 admin 可对工单评分（越权面扩大）。
	t.Run("admin_cannot_rate_PRD_forbids", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/v1/tickets", requestor.Token,
			map[string]any{"title": "QA-评分偏差", "priority": "P3"})
		c.mustStatus(rec, http.StatusCreated)
		tkID := idOf(t, c.obj(rec), "id")

		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/assign", tkID), agent.Token,
			map[string]any{"assignee_id": agent.ID})
		c.mustStatus(rec, http.StatusOK)
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/transition", tkID), agent.Token,
			map[string]any{"action": "start"})
		c.mustStatus(rec, http.StatusOK)
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/transition", tkID), agent.Token,
			map[string]any{"action": "resolve", "solution": "QA 修复完成"})
		c.mustStatus(rec, http.StatusOK)

		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/rating", tkID), admin.Token,
			map[string]any{"rating": 5, "comment": "admin 越权评分"})
		if rec.Code != http.StatusForbidden {
			t.Errorf("[P2 偏差] admin 对工单评分：PRD 期望 403（仅 requestor 可评），实际 %d；body=%s",
				rec.Code, rec.Body.String())
		}
	})
}
