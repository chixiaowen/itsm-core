// 本文件是 T11 装配批次的端到端集成测试，用纯 Go 嵌入式数据库 + 真实 HTTP 打穿
// 「HTTP -> 中间件 -> handler -> service -> repository -> GORM -> 数据库」全链路。
//
// 背景与效力边界（务必阅读）：
//
//   - 此前全部单测都在 service 层注入内存 fake，GORM 模型定义、AutoMigrate、
//     repository_gorm.go 的查询语句**从未被真实验证**。本测试正是为补这一盲区。
//   - 默认使用 github.com/glebarez/sqlite（纯 Go、无 CGO）作为「PostgreSQL / 达梦的行为代理」。
//     它验证的是：GORM 模型与迁移、查询语句、双库无关的 SQL 语义、以及服务编排的正确性。
//   - **它不能替代达梦 / PostgreSQL 的方言级验证**（如不同驱动的 DDL 方言、达梦 30 字符
//     标识符限制、大小写/时区差异等）。方言级验证属「人工验证清单」，不在本测试效力范围内。
//   - 当环境变量 ITSM_TEST_DSN 存在时改用 PostgreSQL 连接（CI 中提供真实 postgres:16），
//     从而在 CI 里有真实 PostgreSQL 验证；本地无 DSN 时回退 SQLite，零外部依赖可跑。
//
// 本测试一旦失败，绝不通过放宽断言 / 删除用例来「变绿」；失败即视为集成期真实缺陷，
// 需如实记录（请求、期望码、实际码、错误信息）。
package bootstrap_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/chixiaowen/itsm-core/internal/bootstrap"
	"github.com/chixiaowen/itsm-core/internal/config"
)

// knownTables 是全部 9 个域的表名（用于每次测试前清库，保证 PostgreSQL 复用场景可重跑）。
var knownTables = []string{
	"users", "sla_policies", "audit_logs", "comments", "attachments",
	"cis", "ci_relations",
	"assets", "asset_histories",
	"service_categories", "service_items",
	"ticket_categories", "tickets", "ticket_cis",
	"incidents", "incident_escalations", "incident_cis",
	"changes", "change_approvals",
	"problems", "problem_changes",
}

// openTestDB 打开测试数据库：ITSM_TEST_DSN 存在则用 PostgreSQL，否则回退内存 SQLite。
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormCfg := &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   gormlogger.Default.LogMode(gormlogger.Silent),
	}
	if dsn := os.Getenv("ITSM_TEST_DSN"); dsn != "" {
		db, err := gorm.Open(postgres.Open(dsn), gormCfg)
		if err != nil {
			t.Fatalf("连接 PostgreSQL 失败（ITSM_TEST_DSN）: %v", err)
		}
		// 清库以保证可重复运行（CI 每次都是全新库，此步无害）。
		if err := db.Migrator().DropTable(knownTables); err != nil {
			t.Logf("清理 PostgreSQL 旧表（可忽略）: %v", err)
		}
		return db
	}
	name := fmt.Sprintf("file:itsm_it_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(name), gormCfg)
	if err != nil {
		t.Fatalf("打开内存 SQLite 失败: %v", err)
	}
	return db
}

// testConfig 构造最小可用配置。
func testConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{}
	cfg.App.Env = config.EnvDevelopment
	cfg.App.Name = "itsm-core-it"
	cfg.App.UploadDir = t.TempDir()
	cfg.App.MaxUploadMB = 20
	cfg.Auth.JWTSecret = "integration-test-secret"
	cfg.Auth.JWTTTLHours = 24
	cfg.Change.EnforceWindow = false
	return cfg
}

// bodyResp 是统一响应体 {"code","message","data"}。
type bodyResp struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// client 是对 gin 引擎的薄封装，发真实 HTTP 请求。
type client struct {
	t      *testing.T
	engine *gin.Engine
}

func (c *client) do(method, path, token string, body any) *httptest.ResponseRecorder {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("序列化请求体失败: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	c.engine.ServeHTTP(rec, req)
	return rec
}

// mustStatus 断言 HTTP 状态码。
func (c *client) mustStatus(rec *httptest.ResponseRecorder, want int) {
	c.t.Helper()
	if rec.Code != want {
		c.t.Fatalf("期望 HTTP %d，实际 %d；body=%s", want, rec.Code, rec.Body.String())
	}
}

// decode 解析统一响应体。
func (c *client) decode(rec *httptest.ResponseRecorder) bodyResp {
	c.t.Helper()
	var b bodyResp
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		c.t.Fatalf("解析响应体失败: %v；raw=%s", err, rec.Body.String())
	}
	return b
}

// obj 把 data 解析为对象。
func (c *client) obj(rec *httptest.ResponseRecorder) map[string]any {
	c.t.Helper()
	b := c.decode(rec)
	var m map[string]any
	if err := json.Unmarshal(b.Data, &m); err != nil {
		c.t.Fatalf("data 不是对象: %v；raw=%s", err, string(b.Data))
	}
	return m
}

// arr 把 data 解析为对象数组。
func (c *client) arr(rec *httptest.ResponseRecorder) []map[string]any {
	c.t.Helper()
	b := c.decode(rec)
	var a []map[string]any
	if err := json.Unmarshal(b.Data, &a); err != nil {
		c.t.Fatalf("data 不是数组: %v；raw=%s", err, string(b.Data))
	}
	return a
}

// idOf 从对象中取无符号整型字段。
func idOf(t *testing.T, m map[string]any, key string) uint64 {
	t.Helper()
	v, ok := m[key]
	if !ok || v == nil {
		t.Fatalf("响应对象缺少字段 %q: %v", key, m)
	}
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("字段 %q 不是数字: %v", key, v)
	}
	return uint64(f)
}

// loginResult 是登录结果。
type loginResult struct {
	Token string
	ID    uint64
	Role  string
}

// login 登录并返回 token 与用户信息。
func (c *client) login(username, password string) loginResult {
	c.t.Helper()
	rec := c.do(http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": username, "password": password,
	})
	c.mustStatus(rec, http.StatusOK)
	b := c.decode(rec)
	if b.Code != 0 {
		c.t.Fatalf("登录 %s 返回业务码 %d: %s", username, b.Code, b.Message)
	}
	var payload struct {
		Token string `json:"token"`
		User  struct {
			ID   uint64 `json:"id"`
			Role string `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal(b.Data, &payload); err != nil {
		c.t.Fatalf("解析登录响应失败: %v", err)
	}
	if payload.Token == "" || payload.User.ID == 0 {
		c.t.Fatalf("登录 %s 未返回有效 token/user", username)
	}
	return loginResult{Token: payload.Token, ID: payload.User.ID, Role: payload.User.Role}
}

// TestITSMIntegrationEndToEnd 是端到端集成总用例（子用例按顺序共享同一库，非并行）。
func TestITSMIntegrationEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := zap.NewNop()

	db := openTestDB(t)
	cfg := testConfig(t)

	// ---- 1. 迁移：调用全部 9 个域 Migrate，断言零错误 ----
	t.Run("01_Migrate_全部域零错误", func(t *testing.T) {
		if err := bootstrap.Migrate(db); err != nil {
			t.Fatalf("按拓扑序迁移失败: %v", err)
		}
	})

	// ---- 2. 种子数据：写入并断言 8 个账号存在且可登录 ----
	t.Run("02_Seed_演示账号与登录", func(t *testing.T) {
		if err := bootstrap.Seed(db, log); err != nil {
			t.Fatalf("种子数据写入失败: %v", err)
		}
	})

	app, err := bootstrap.New(cfg, log, db)
	if err != nil {
		t.Fatalf("装配失败: %v", err)
	}
	engine := bootstrap.BuildEngine(cfg, log, db, app)
	c := &client{t: t, engine: engine}

	// 登录全部 8 个演示账号。
	admin := c.login("admin", "admin123")
	requestor := c.login("requestor01", "admin123")
	agent := c.login("agent01", "admin123")
	resolver := c.login("resolver01", "admin123")
	_ = c.login("pm01", "admin123")
	cm01 := c.login("cm01", "admin123")
	cm02 := c.login("cm02", "admin123")
	cmdb01 := c.login("cmdb01", "admin123")

	t.Run("02b_用户目录含8个演示账号", func(t *testing.T) {
		rec := c.do(http.MethodGet, "/api/v1/users?page_size=100", admin.Token, nil)
		c.mustStatus(rec, http.StatusOK)
		b := c.decode(rec)
		var payload struct {
			Items []struct {
				Username string `json:"username"`
			} `json:"items"`
		}
		if err := json.Unmarshal(b.Data, &payload); err != nil {
			t.Fatalf("解析用户列表失败: %v", err)
		}
		got := map[string]bool{}
		for _, it := range payload.Items {
			got[it.Username] = true
		}
		for _, want := range []string{"admin", "requestor01", "agent01", "resolver01", "pm01", "cm01", "cm02", "cmdb01"} {
			if !got[want] {
				t.Fatalf("演示账号缺失: %s（实际: %v）", want, got)
			}
		}
	})

	t.Run("02c_健康检查", func(t *testing.T) {
		rec := c.do(http.MethodGet, "/healthz", "", nil)
		c.mustStatus(rec, http.StatusOK)
	})

	var ticketID, illegalTicketID uint64

	// ---- 3. 工单全生命周期 ----
	t.Run("03_工单全生命周期", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/v1/tickets", requestor.Token, map[string]any{
			"title": "打印机无法打印", "priority": "P3",
		})
		c.mustStatus(rec, http.StatusCreated)
		o := c.obj(rec)
		ticketID = idOf(t, o, "id")
		if o["status"] != "new" {
			t.Fatalf("新建工单初始状态应为 new，实际 %v", o["status"])
		}

		// 非法流转：对 new 工单调 start → 409。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/transition", ticketID), agent.Token,
			map[string]any{"action": "start"})
		c.mustStatus(rec, http.StatusConflict)

		// 指派
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/assign", ticketID), agent.Token,
			map[string]any{"assignee_id": agent.ID})
		c.mustStatus(rec, http.StatusOK)
		if s := c.obj(rec)["status"]; s != "assigned" {
			t.Fatalf("指派后状态应为 assigned，实际 %v", s)
		}

		// 开始处理
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/transition", ticketID), agent.Token,
			map[string]any{"action": "start"})
		c.mustStatus(rec, http.StatusOK)
		if s := c.obj(rec)["status"]; s != "in_progress" {
			t.Fatalf("开始处理后状态应为 in_progress，实际 %v", s)
		}

		// 挂起
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/transition", ticketID), agent.Token,
			map[string]any{"action": "pending", "reason": "等待备件"})
		c.mustStatus(rec, http.StatusOK)
		if s := c.obj(rec)["status"]; s != "pending" {
			t.Fatalf("挂起后状态应为 pending，实际 %v", s)
		}

		// 恢复
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/transition", ticketID), agent.Token,
			map[string]any{"action": "resume"})
		c.mustStatus(rec, http.StatusOK)
		if s := c.obj(rec)["status"]; s != "in_progress" {
			t.Fatalf("恢复后状态应为 in_progress，实际 %v", s)
		}

		// resolve 缺 solution → 422。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/transition", ticketID), agent.Token,
			map[string]any{"action": "resolve"})
		c.mustStatus(rec, http.StatusUnprocessableEntity)

		// resolve 带 solution → resolved，且 resolved_at 写入。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/transition", ticketID), agent.Token,
			map[string]any{"action": "resolve", "solution": "更换硒鼓并重启打印服务"})
		c.mustStatus(rec, http.StatusOK)
		o = c.obj(rec)
		if o["status"] != "resolved" {
			t.Fatalf("解决后状态应为 resolved，实际 %v", o["status"])
		}
		if o["resolved_at"] == nil {
			t.Fatalf("解决后 resolved_at 应被写入，实际 nil")
		}

		// 关闭 → closed，且 closed_at 写入。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/transition", ticketID), agent.Token,
			map[string]any{"action": "close"})
		c.mustStatus(rec, http.StatusOK)
		o = c.obj(rec)
		if o["status"] != "closed" {
			t.Fatalf("关闭后状态应为 closed，实际 %v", o["status"])
		}
		if o["closed_at"] == nil {
			t.Fatalf("关闭后 closed_at 应被写入，实际 nil")
		}
	})

	t.Run("03b_工单非法流转_新建即开始_409", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/v1/tickets", requestor.Token, map[string]any{"title": "另一张工单"})
		c.mustStatus(rec, http.StatusCreated)
		illegalTicketID = idOf(t, c.obj(rec), "id")

		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/tickets/%d/transition", illegalTicketID), agent.Token,
			map[string]any{"action": "start"})
		c.mustStatus(rec, http.StatusConflict)
	})

	// ---- 4. 事件 ----
	t.Run("04_事件_矩阵_升级_转单", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/v1/incidents", agent.Token, map[string]any{
			"title": "核心交换机宕机", "impact": "high", "urgency": "high",
		})
		c.mustStatus(rec, http.StatusCreated)
		o := c.obj(rec)
		incidentID := idOf(t, o, "id")
		if o["priority"] != "P1" {
			t.Fatalf("高影响×高紧急应定级 P1，实际 %v", o["priority"])
		}

		// 分诊
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/incidents/%d/transition", incidentID), agent.Token,
			map[string]any{"action": "triage"})
		c.mustStatus(rec, http.StatusOK)

		// 确认（指派）→ in_progress
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/incidents/%d/transition", incidentID), agent.Token,
			map[string]any{"action": "confirm", "assignee_id": resolver.ID})
		c.mustStatus(rec, http.StatusOK)
		if s := c.obj(rec)["status"]; s != "in_progress" {
			t.Fatalf("确认后状态应为 in_progress，实际 %v", s)
		}

		// 越级升级 → 409。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/incidents/%d/escalate", incidentID), agent.Token,
			map[string]any{"type": "functional", "reason": "越级测试", "level": 3})
		c.mustStatus(rec, http.StatusConflict)

		// 转工单 → 201。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/incidents/%d/convert-to-ticket", incidentID), agent.Token,
			map[string]any{})
		c.mustStatus(rec, http.StatusCreated)
		if idOf(t, c.obj(rec), "ticket_id") == 0 {
			t.Fatalf("转单应返回非零 ticket_id")
		}

		// 重复转单 → 409。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/incidents/%d/convert-to-ticket", incidentID), agent.Token,
			map[string]any{})
		c.mustStatus(rec, http.StatusConflict)
	})

	// ---- 5. 变更：标准免审直通 + 普通 CAB 会签 ----
	t.Run("05_变更_标准免审直通", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/v1/changes", cm01.Token, map[string]any{
			"title": "标准变更-重启服务", "change_type": "standard", "risk_level": "low",
		})
		c.mustStatus(rec, http.StatusCreated)
		id := idOf(t, c.obj(rec), "id")

		rec = c.do(http.MethodPut, fmt.Sprintf("/api/v1/changes/%d", id), cm01.Token, map[string]any{
			"plan": "滚动重启", "rollback_plan": "回滚至上一版本",
		})
		c.mustStatus(rec, http.StatusOK)

		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/changes/%d/transition", id), admin.Token,
			map[string]any{"action": "pre_authorize"})
		c.mustStatus(rec, http.StatusOK)
		if s := c.obj(rec)["status"]; s != "approved" {
			t.Fatalf("标准变更免审应直通 approved，实际 %v", s)
		}
	})

	t.Run("05b_变更_普通CAB会签", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/v1/changes", cm01.Token, map[string]any{
			"title": "普通变更-升级数据库", "change_type": "normal", "risk_level": "medium",
		})
		c.mustStatus(rec, http.StatusCreated)
		id := idOf(t, c.obj(rec), "id")

		rec = c.do(http.MethodPut, fmt.Sprintf("/api/v1/changes/%d", id), cm01.Token, map[string]any{
			"plan": "主备切换", "rollback_plan": "切回主库",
		})
		c.mustStatus(rec, http.StatusOK)

		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/changes/%d/transition", id), admin.Token,
			map[string]any{"action": "submit_assessment"})
		c.mustStatus(rec, http.StatusOK)

		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/changes/%d/transition", id), cm01.Token,
			map[string]any{"action": "submit_approval"})
		c.mustStatus(rec, http.StatusOK)
		if s := c.obj(rec)["status"]; s != "pending_approval" {
			t.Fatalf("提交审批后应为 pending_approval，实际 %v", s)
		}

		// 第一位审批人通过：普通变更需 2 人，故仍非 approved。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/changes/%d/approvals", id), cm01.Token,
			map[string]any{"decision": "approved", "comment": "同意"})
		c.mustStatus(rec, http.StatusOK)
		if s := c.obj(rec)["status"]; s == "approved" {
			t.Fatalf("仅 1 名审批人通过时不应为 approved，实际 %v", s)
		}

		// 第二位审批人通过：会签达成 → approved。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/changes/%d/approvals", id), cm02.Token,
			map[string]any{"decision": "approved", "comment": "同意"})
		c.mustStatus(rec, http.StatusOK)
		if s := c.obj(rec)["status"]; s != "approved" {
			t.Fatalf("2 名审批人全部通过后应为 approved，实际 %v", s)
		}
	})

	// ---- 6. 服务目录：发布 -> 下单 -> 生成工单 ----
	var serviceItemID uint64
	t.Run("06_服务目录_下单转工单", func(t *testing.T) {
		// 取一条 SLA 策略 id。
		rec := c.do(http.MethodGet, "/api/v1/sla-policies", admin.Token, nil)
		c.mustStatus(rec, http.StatusOK)
		slaList := c.arr(rec)
		if len(slaList) == 0 {
			t.Fatalf("种子 SLA 策略为空")
		}
		slaID := idOf(t, slaList[0], "id")

		// 建分类。
		rec = c.do(http.MethodPost, "/api/v1/service-categories", admin.Token, map[string]any{
			"name": "办公支持", "sort_order": 1,
		})
		c.mustStatus(rec, http.StatusCreated)
		categoryID := idOf(t, c.obj(rec), "id")

		// 建服务项（draft）。
		rec = c.do(http.MethodPost, "/api/v1/service-items", admin.Token, map[string]any{
			"name": "邮箱申请", "description": "新员工邮箱开通",
			"category_id": categoryID, "sla_policy_id": slaID,
			"default_priority": "P3", "form_schema": "{}",
		})
		c.mustStatus(rec, http.StatusCreated)
		o := c.obj(rec)
		serviceItemID = idOf(t, o, "id")
		if o["status"] != "draft" {
			t.Fatalf("新建服务项应为 draft，实际 %v", o["status"])
		}

		// 提交发布审核 → pending_approval。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/service-items/%d/transition", serviceItemID), admin.Token,
			map[string]any{"action": "submit_review"})
		c.mustStatus(rec, http.StatusOK)
		if s := c.obj(rec)["status"]; s != "pending_approval" {
			t.Fatalf("提交审核后应为 pending_approval，实际 %v", s)
		}

		// 发布 → published。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/service-items/%d/publish", serviceItemID), admin.Token,
			map[string]any{})
		c.mustStatus(rec, http.StatusOK)
		if s := c.obj(rec)["status"]; s != "published" {
			t.Fatalf("发布后应为 published，实际 %v", s)
		}

		// 用户下单 → 生成工单。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/catalog/items/%d/order", serviceItemID), requestor.Token,
			map[string]any{"form_data": map[string]any{}})
		c.mustStatus(rec, http.StatusCreated)
		orderedTicketID := idOf(t, c.obj(rec), "ticket_id")
		if orderedTicketID == 0 {
			t.Fatalf("下单未返回工单 id")
		}

		// 断言生成的工单 service_item_id 非空。
		rec = c.do(http.MethodGet, fmt.Sprintf("/api/v1/tickets/%d", orderedTicketID), requestor.Token, nil)
		c.mustStatus(rec, http.StatusOK)
		tk := c.decode(rec)
		var detail map[string]any
		if err := json.Unmarshal(tk.Data, &detail); err != nil {
			t.Fatalf("解析工单详情失败: %v", err)
		}
		if detail["service_item_id"] == nil {
			t.Fatalf("下单生成的工单 service_item_id 应非空，实际 %v", detail["service_item_id"])
		}
		if got := idOf(t, detail, "service_item_id"); got != serviceItemID {
			t.Fatalf("工单 service_item_id 应为 %d，实际 %d", serviceItemID, got)
		}
	})

	// ---- 7. CMDB：CI / 关系 / 拓扑 ----
	var ciA uint64
	t.Run("07_CMDB_关系与拓扑", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/v1/cis", cmdb01.Token, map[string]any{
			"code": "it-ci-a", "name": "应用服务器 A", "ci_type": "server",
		})
		c.mustStatus(rec, http.StatusCreated)
		ciA = idOf(t, c.obj(rec), "id")

		rec = c.do(http.MethodPost, "/api/v1/cis", cmdb01.Token, map[string]any{
			"code": "it-ci-b", "name": "数据库 B", "ci_type": "database",
		})
		c.mustStatus(rec, http.StatusCreated)
		ciB := idOf(t, c.obj(rec), "id")

		// 自环 → 400。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/cis/%d/relations", ciA), cmdb01.Token,
			map[string]any{"target_ci_id": ciA, "relation_type": "depends_on"})
		c.mustStatus(rec, http.StatusBadRequest)

		// 正常关系 → 201。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/cis/%d/relations", ciA), cmdb01.Token,
			map[string]any{"target_ci_id": ciB, "relation_type": "depends_on"})
		c.mustStatus(rec, http.StatusCreated)

		// 重复关系 → 409。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/cis/%d/relations", ciA), cmdb01.Token,
			map[string]any{"target_ci_id": ciB, "relation_type": "depends_on"})
		c.mustStatus(rec, http.StatusConflict)

		// 拓扑 → nodes/edges。
		rec = c.do(http.MethodGet, fmt.Sprintf("/api/v1/cis/%d/topology?depth=2", ciA), cmdb01.Token, nil)
		c.mustStatus(rec, http.StatusOK)
		g := c.obj(rec)
		nodes, _ := g["nodes"].([]any)
		edges, _ := g["edges"].([]any)
		if len(nodes) < 2 {
			t.Fatalf("拓扑 nodes 应含 ≥2 个节点，实际 %d", len(nodes))
		}
		if len(edges) < 1 {
			t.Fatalf("拓扑 edges 应含 ≥1 条边，实际 %d", len(edges))
		}
	})

	// ---- 8. 资产：绑定 CI，冲突 409 ----
	t.Run("08_资产_绑定CI冲突", func(t *testing.T) {
		rec := c.do(http.MethodPost, "/api/v1/assets", cmdb01.Token, map[string]any{
			"asset_no": "it-ast-1", "name": "研发笔记本", "category": "laptop",
		})
		c.mustStatus(rec, http.StatusCreated)
		asset1 := idOf(t, c.obj(rec), "id")

		rec = c.do(http.MethodPost, "/api/v1/assets", cmdb01.Token, map[string]any{
			"asset_no": "it-ast-2", "name": "备用笔记本", "category": "laptop",
		})
		c.mustStatus(rec, http.StatusCreated)
		asset2 := idOf(t, c.obj(rec), "id")

		// 资产1 绑定 CI → 200。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/assets/%d/bind-ci", asset1), cmdb01.Token,
			map[string]any{"ci_id": ciA})
		c.mustStatus(rec, http.StatusOK)
		if got := idOf(t, c.obj(rec), "ci_id"); got != ciA {
			t.Fatalf("资产应绑定 CI %d，实际 %d", ciA, got)
		}

		// 同一 CI 被第二个资产绑定 → 409。
		rec = c.do(http.MethodPost, fmt.Sprintf("/api/v1/assets/%d/bind-ci", asset2), cmdb01.Token,
			map[string]any{"ci_id": ciA})
		c.mustStatus(rec, http.StatusConflict)
	})

	// ---- 9. 鉴权与错误码 ----
	t.Run("09_鉴权与错误码", func(t *testing.T) {
		// 无 token → 401。
		rec := c.do(http.MethodGet, "/api/v1/tickets", "", nil)
		c.mustStatus(rec, http.StatusUnauthorized)

		// requestor 访问管理员接口 → 403。
		rec = c.do(http.MethodGet, "/api/v1/users", requestor.Token, nil)
		c.mustStatus(rec, http.StatusForbidden)

		// 不存在的资源 → 404。
		rec = c.do(http.MethodGet, "/api/v1/tickets/99999999", admin.Token, nil)
		c.mustStatus(rec, http.StatusNotFound)

		// 带实体范围的审计查询（requestor）→ 200。
		rec = c.do(http.MethodGet,
			fmt.Sprintf("/api/v1/audit-logs?entity_type=ticket&entity_id=%d", ticketID), requestor.Token, nil)
		c.mustStatus(rec, http.StatusOK)
		b := c.decode(rec)
		var page struct {
			Total int64 `json:"total"`
		}
		if err := json.Unmarshal(b.Data, &page); err != nil {
			t.Fatalf("解析审计分页失败: %v", err)
		}
		if page.Total < 1 {
			t.Fatalf("工单 %d 的审计时间线应 ≥1 条，实际 %d", ticketID, page.Total)
		}

		// 不带实体范围的全局审计（requestor）→ 403。
		rec = c.do(http.MethodGet, "/api/v1/audit-logs", requestor.Token, nil)
		c.mustStatus(rec, http.StatusForbidden)
	})
}
