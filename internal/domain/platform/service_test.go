package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

type testEnv struct {
	users       *fakeUserRepo
	sla         *fakeSLARepo
	audits      *fakeAuditRepo
	comments    *fakeCommentRepo
	attachments *fakeAttachmentRepo
	svc         *Service
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	e := &testEnv{
		users:       newFakeUserRepo(),
		sla:         newFakeSLARepo(),
		audits:      newFakeAuditRepo(),
		comments:    newFakeCommentRepo(),
		attachments: newFakeAttachmentRepo(),
	}
	e.svc = NewService(Deps{
		Users:          e.users,
		SLAPolicies:    e.sla,
		Audits:         e.audits,
		Comments:       e.comments,
		Attachments:    e.attachments,
		UploadDir:      t.TempDir(),
		MaxUploadBytes: 1024,
		Now:            func() time.Time { return time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC) },
	})
	return e
}

func appErrCode(t *testing.T, err error) int {
	t.Helper()
	var ae *httpx.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("期望 *httpx.AppError，实际 %T (%v)", err, err)
	}
	return ae.Code
}

func sampleUserRequest() CreateUserRequest {
	return CreateUserRequest{Username: "alice", DisplayName: "Alice", Role: role.Agent, Password: "secret1", Email: "a@b.com"}
}

// ---------------- 用户 ----------------

func TestCreateUser_SuccessAndDuplicate(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	u, err := e.svc.CreateUser(ctx, 1, sampleUserRequest())
	if err != nil {
		t.Fatalf("CreateUser 失败: %v", err)
	}
	if u.ID == 0 || u.Status != UserStatusActive {
		t.Fatalf("新建用户字段不符: %+v", u)
	}
	if u.PasswordHash == "" || u.PasswordHash == "secret1" {
		t.Fatalf("密码应被哈希存储")
	}

	if _, err := e.svc.CreateUser(ctx, 1, sampleUserRequest()); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("重复用户名应 409，实际 %v", err)
	}
}

func TestCreateUser_InvalidRole(t *testing.T) {
	e := newTestEnv(t)
	req := sampleUserRequest()
	req.Role = "superuser"
	if _, err := e.svc.CreateUser(context.Background(), 1, req); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法角色应 400，实际 %v", err)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	e := newTestEnv(t)
	if _, err := e.svc.GetUser(context.Background(), 999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404，实际 %v", err)
	}
}

func TestUpdateUser(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	u, _ := e.svc.CreateUser(ctx, 1, sampleUserRequest())

	name := "Alice 2"
	newRole := role.Resolver
	pwd := "newsecret"
	status := UserStatusDisabled

	got, err := e.svc.UpdateUser(ctx, 1, u.ID, UpdateUserRequest{
		DisplayName: &name, Role: &newRole, Password: &pwd, Status: &status,
	})
	if err != nil {
		t.Fatalf("UpdateUser 失败: %v", err)
	}
	if got.DisplayName != name || got.Role != newRole || got.Status != status {
		t.Fatalf("更新字段不符: %+v", got)
	}
	if got.PasswordHash == "" {
		t.Fatalf("密码应已重置")
	}

	// 非法角色
	bad := "nope"
	if _, err := e.svc.UpdateUser(ctx, 1, u.ID, UpdateUserRequest{Role: &bad}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法角色应 400")
	}
	// 空展示名
	empty := "  "
	if _, err := e.svc.UpdateUser(ctx, 1, u.ID, UpdateUserRequest{DisplayName: &empty}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("空展示名应 400")
	}
	// 非法状态
	badStatus := "frozen"
	if _, err := e.svc.UpdateUser(ctx, 1, u.ID, UpdateUserRequest{Status: &badStatus}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法状态应 400")
	}
	// 不存在
	if _, err := e.svc.UpdateUser(ctx, 1, 999, UpdateUserRequest{}); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404")
	}
}

func TestDeleteUser(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	u, _ := e.svc.CreateUser(ctx, 1, sampleUserRequest())

	// 不能删除自己
	if err := e.svc.DeleteUser(ctx, u.ID, u.ID); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("删除自己应 422，实际 %v", err)
	}
	if err := e.svc.DeleteUser(ctx, 100, u.ID); err != nil {
		t.Fatalf("删除应成功: %v", err)
	}
	if err := e.svc.DeleteUser(ctx, 100, u.ID); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("删除不存在应 404")
	}
}

func TestListUsers(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		req := CreateUserRequest{Username: "user" + string(rune('a'+i)), DisplayName: "U", Role: role.Agent, Password: "secret1"}
		if _, err := e.svc.CreateUser(ctx, 1, req); err != nil {
			t.Fatalf("准备数据失败: %v", err)
		}
	}
	items, total, err := e.svc.ListUsers(ctx, UserListQuery{Offset: 0, Limit: 2})
	if err != nil {
		t.Fatalf("ListUsers 失败: %v", err)
	}
	if total != 5 || len(items) != 2 {
		t.Fatalf("分页不符: total=%d len=%d", total, len(items))
	}

	items, total, _ = e.svc.ListUsers(ctx, UserListQuery{Keyword: "usera", Offset: 0, Limit: 10})
	if total != 1 || len(items) != 1 {
		t.Fatalf("关键字过滤不符: total=%d", total)
	}

	// 角色过滤（无匹配）
	_, total, _ = e.svc.ListUsers(ctx, UserListQuery{Role: role.Admin})
	if total != 0 {
		t.Fatalf("角色过滤应为 0，实际 %d", total)
	}
}

func TestListUserOptions_RoleFilterAndActiveOnly(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// 3 个 agent + 1 个 admin（均 active）
	seeds := []CreateUserRequest{
		{Username: "oa", DisplayName: "A", Role: role.Agent, Password: "secret1"},
		{Username: "ob", DisplayName: "B", Role: role.Agent, Password: "secret1"},
		{Username: "oc", DisplayName: "C", Role: role.Agent, Password: "secret1"},
		{Username: "od", DisplayName: "D", Role: role.Admin, Password: "secret1"},
	}
	var agentToDisable uint64
	for i, req := range seeds {
		u, err := e.svc.CreateUser(ctx, 1, req)
		if err != nil {
			t.Fatalf("准备数据失败: %v", err)
		}
		if i == 0 {
			agentToDisable = u.ID
		}
	}
	// 禁用其中一个 agent
	dis := UserStatusDisabled
	if _, err := e.svc.UpdateUser(ctx, 1, agentToDisable, UpdateUserRequest{Status: &dis}); err != nil {
		t.Fatalf("禁用用户失败: %v", err)
	}

	// 不带 role：仅返回 active（3 个）
	all, err := e.svc.ListUserOptions(ctx, "")
	if err != nil {
		t.Fatalf("ListUserOptions 失败: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("应返回 3 个 active 用户，实际 %d", len(all))
	}
	for _, o := range all {
		if o.ID == agentToDisable {
			t.Fatalf("已禁用用户不应出现在候选中: %+v", o)
		}
	}

	// 按 role=agent 过滤：仅 2 个 active agent
	agents, err := e.svc.ListUserOptions(ctx, role.Agent)
	if err != nil {
		t.Fatalf("按角色过滤失败: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("active agent 应 2 个，实际 %d", len(agents))
	}
	for _, o := range agents {
		if o.Role != role.Agent {
			t.Fatalf("角色过滤失效，出现非 agent: %+v", o)
		}
		if o.ID == agentToDisable {
			t.Fatalf("已禁用 agent 仍被返回")
		}
	}

	// 按 role=admin 过滤：仅 1 个
	admins, _ := e.svc.ListUserOptions(ctx, role.Admin)
	if len(admins) != 1 || admins[0].Role != role.Admin {
		t.Fatalf("admin 过滤不符: %+v", admins)
	}

	// 仓储缺失 -> 500
	if _, err := NewService(Deps{}).ListUserOptions(ctx, ""); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺少仓储应 500")
	}
}

func TestListUserOptions_NoSensitiveFields(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	if _, err := e.svc.CreateUser(ctx, 1, sampleUserRequest()); err != nil {
		t.Fatalf("准备数据失败: %v", err)
	}
	opts, err := e.svc.ListUserOptions(ctx, "")
	if err != nil {
		t.Fatalf("ListUserOptions 失败: %v", err)
	}
	if len(opts) != 1 {
		t.Fatalf("应 1 条，实际 %d", len(opts))
	}

	raw, err := json.Marshal(opts[0])
	if err != nil {
		t.Fatalf("marshal 失败: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal 失败: %v", err)
	}
	want := map[string]bool{"id": true, "display_name": true, "role": true}
	if len(m) != len(want) {
		t.Fatalf("响应字段数应为 %d，实际 %v", len(want), m)
	}
	for k := range m {
		if !want[k] {
			t.Fatalf("响应含非预期字段: %s", k)
		}
	}
	for _, sensitive := range []string{"password", "password_hash", "email", "phone", "mobile"} {
		if _, ok := m[sensitive]; ok {
			t.Fatalf("响应不应含敏感字段 %s", sensitive)
		}
	}
	if bytes.Contains(raw, []byte("secret1")) {
		t.Fatalf("响应不应出现明文密码")
	}
}

// ---------------- SLA 策略 ----------------

func TestSLAPolicy_CRUDAndUniqueness(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	p1, err := e.svc.CreateSLAPolicy(ctx, 1, SLAPolicyRequest{Name: "P1 策略", Priority: "P1", ResponseMinutes: 15, ResolveMinutes: 240, PauseOnPending: true})
	if err != nil {
		t.Fatalf("创建 P1 失败: %v", err)
	}
	if _, err := e.svc.CreateSLAPolicy(ctx, 1, SLAPolicyRequest{Name: "重复", Priority: "P1", ResponseMinutes: 1, ResolveMinutes: 1}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("重复优先级应 409，实际 %v", err)
	}

	p2, _ := e.svc.CreateSLAPolicy(ctx, 1, SLAPolicyRequest{Name: "P2 策略", Priority: "P2", ResponseMinutes: 30, ResolveMinutes: 480})

	// 更新 P2 为 P1 -> 冲突
	if _, err := e.svc.UpdateSLAPolicy(ctx, 1, p2.ID, SLAPolicyRequest{Name: "P2", Priority: "P1", ResponseMinutes: 30, ResolveMinutes: 480}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("改优先级冲突应 409，实际 %v", err)
	}

	// 正常更新
	updated, err := e.svc.UpdateSLAPolicy(ctx, 1, p2.ID, SLAPolicyRequest{Name: "P3 策略", Priority: "P3", ResponseMinutes: 60, ResolveMinutes: 600, PauseOnPending: true})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if updated.Priority != "P3" || updated.ResponseMinutes != 60 {
		t.Fatalf("更新字段不符: %+v", updated)
	}

	items, err := e.svc.ListSLAPolicies(ctx)
	if err != nil || len(items) != 2 {
		t.Fatalf("列表不符: len=%d err=%v", len(items), err)
	}

	if _, err := e.svc.GetSLAPolicy(ctx, p1.ID); err != nil {
		t.Fatalf("GetSLAPolicy 失败: %v", err)
	}
	if _, err := e.svc.GetSLAPolicy(ctx, 999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404")
	}
	if err := e.svc.DeleteSLAPolicy(ctx, 1, p1.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if err := e.svc.DeleteSLAPolicy(ctx, 1, p1.ID); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("删除不存在应 404")
	}
	if _, err := e.svc.UpdateSLAPolicy(ctx, 1, 999, SLAPolicyRequest{Name: "x", Priority: "P4", ResponseMinutes: 1, ResolveMinutes: 1}); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("更新不存在应 404")
	}
}

// ---------------- 审计 ----------------

func TestAudit_AppendAndList(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	if err := e.svc.AppendAudit(ctx, AuditEntry{ActorID: 7, Action: "transition", BizType: "ticket", BizID: 1, FromStatus: "new", ToStatus: "assigned", ClientIP: "127.0.0.1"}); err != nil {
		t.Fatalf("AppendAudit 失败: %v", err)
	}
	// 业务操作应产生审计
	u, _ := e.svc.CreateUser(ctx, 7, sampleUserRequest())
	_ = u

	items, total, err := e.svc.ListAuditLogs(ctx, AuditListQuery{})
	if err != nil {
		t.Fatalf("ListAuditLogs 失败: %v", err)
	}
	if total < 2 || len(items) < 2 {
		t.Fatalf("审计条数不符: total=%d", total)
	}

	_, total, _ = e.svc.ListAuditLogs(ctx, AuditListQuery{ActorID: 7, BizType: "ticket", BizID: 1, Action: "transition"})
	if total != 1 {
		t.Fatalf("审计过滤应为 1，实际 %d", total)
	}
}

func TestListAuditLogs_EntityTypeAndIDFilter(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// 三条审计：ticket#11、ticket#22、incident#11
	_ = e.svc.AppendAudit(ctx, AuditEntry{ActorID: 1, Action: "transition", BizType: "ticket", BizID: 11, FromStatus: "new", ToStatus: "assigned"})
	_ = e.svc.AppendAudit(ctx, AuditEntry{ActorID: 1, Action: "transition", BizType: "ticket", BizID: 22, FromStatus: "assigned", ToStatus: "resolved"})
	_ = e.svc.AppendAudit(ctx, AuditEntry{ActorID: 1, Action: "transition", BizType: "incident", BizID: 11, FromStatus: "new", ToStatus: "closed"})

	// entity_type + entity_id 联合过滤
	items, total, err := e.svc.ListAuditLogs(ctx, AuditListQuery{EntityType: "ticket", EntityID: 11})
	if err != nil {
		t.Fatalf("ListAuditLogs 失败: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("entity 联合过滤应为 1，实际 total=%d len=%d", total, len(items))
	}
	if items[0].BizType != "ticket" || items[0].BizID != 11 {
		t.Fatalf("过滤结果不符: %+v", items[0])
	}
	// 返回结构含 from_status/to_status（前端状态流转时间线所需）
	if items[0].FromStatus != "new" || items[0].ToStatus != "assigned" {
		t.Fatalf("from_status/to_status 缺失: %+v", items[0])
	}

	// 仅 entity_type 过滤 -> 2 条 ticket
	_, total, _ = e.svc.ListAuditLogs(ctx, AuditListQuery{EntityType: "ticket"})
	if total != 2 {
		t.Fatalf("ticket 应为 2，实际 %d", total)
	}

	// biz_type/biz_id 与 entity_type/entity_id 等价
	_, total, _ = e.svc.ListAuditLogs(ctx, AuditListQuery{BizType: "ticket", BizID: 22})
	if total != 1 {
		t.Fatalf("biz 过滤应为 1，实际 %d", total)
	}
}

// ---------------- 评论 ----------------

func TestComment_CreateAndList(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	if _, err := e.svc.CreateComment(ctx, Operator{ID: 1}, CommentRequest{BizType: "bad", BizID: 1, Content: "x"}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法 biz_type 应 400")
	}
	if _, err := e.svc.CreateComment(ctx, Operator{ID: 1}, CommentRequest{BizType: BizTypeTicket, BizID: 1, Content: "   "}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("空内容应 400")
	}
	if _, err := e.svc.CreateComment(ctx, Operator{ID: 1}, CommentRequest{BizType: BizTypeTicket, BizID: 1, Content: "公开回复"}); err != nil {
		t.Fatalf("创建评论失败: %v", err)
	}
	if _, err := e.svc.CreateComment(ctx, Operator{ID: 2}, CommentRequest{BizType: BizTypeTicket, BizID: 1, Content: "内部备注", IsInternal: true}); err != nil {
		t.Fatalf("创建内部备注失败: %v", err)
	}

	visible, err := e.svc.ListComments(ctx, BizTypeTicket, 1, false)
	if err != nil {
		t.Fatalf("ListComments 失败: %v", err)
	}
	if len(visible) != 1 {
		t.Fatalf("对外应仅 1 条，实际 %d", len(visible))
	}
	all, _ := e.svc.ListComments(ctx, BizTypeTicket, 1, true)
	if len(all) != 2 {
		t.Fatalf("含内部应 2 条，实际 %d", len(all))
	}
	if _, err := e.svc.ListComments(ctx, "bad", 1, false); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法 biz_type 应 400")
	}
}

// ---------------- 附件 ----------------

func TestAttachment_SaveLimitAndDelete(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// 超限
	big := bytes.Repeat([]byte("x"), 2048)
	if _, err := e.svc.SaveAttachment(ctx, Operator{ID: 1}, BizTypeTicket, 1, "big.txt", "text/plain", int64(len(big)), bytes.NewReader(big)); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("超限应 400，实际 %v", err)
	}
	// 空文件
	if _, err := e.svc.SaveAttachment(ctx, Operator{ID: 1}, BizTypeTicket, 1, "empty.txt", "text/plain", 0, bytes.NewReader(nil)); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("空文件应 400")
	}
	// 非法 biz_type
	if _, err := e.svc.SaveAttachment(ctx, Operator{ID: 1}, "bad", 1, "a.txt", "text/plain", 3, bytes.NewReader([]byte("abc"))); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法 biz_type 应 400")
	}

	// 正常
	content := []byte("hello")
	a, err := e.svc.SaveAttachment(ctx, Operator{ID: 1}, BizTypeTicket, 1, "hello.txt", "text/plain", int64(len(content)), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("保存附件失败: %v", err)
	}
	if a.ID == 0 || a.Size != int64(len(content)) {
		t.Fatalf("附件字段不符: %+v", a)
	}

	if _, err := e.svc.GetAttachment(ctx, a.ID); err != nil {
		t.Fatalf("GetAttachment 失败: %v", err)
	}
	if _, err := e.svc.GetAttachment(ctx, 999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404")
	}
	list, _ := e.svc.ListAttachments(ctx, BizTypeTicket, 1)
	if len(list) != 1 {
		t.Fatalf("附件列表应 1 条")
	}

	// 非上传者且非 admin -> 403
	if err := e.svc.DeleteAttachment(ctx, Operator{ID: 2, Role: role.Agent}, a.ID); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("非上传者删除应 403，实际 %v", err)
	}
	// admin 可删
	if err := e.svc.DeleteAttachment(ctx, Operator{ID: 9, Role: role.Admin}, a.ID); err != nil {
		t.Fatalf("admin 删除应成功: %v", err)
	}
	if err := e.svc.DeleteAttachment(ctx, Operator{ID: 9, Role: role.Admin}, a.ID); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("再次删除应 404")
	}
}

func TestRolesDelegation(t *testing.T) {
	e := newTestEnv(t)
	roles := e.svc.Roles()
	if len(roles) != 7 {
		t.Fatalf("应返回 7 个角色，实际 %d", len(roles))
	}
}

func TestService_MissingRepos(t *testing.T) {
	svc := NewService(Deps{})
	if _, _, err := svc.ListUsers(context.Background(), UserListQuery{}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺少仓储应 500")
	}
	if _, err := svc.ListSLAPolicies(context.Background()); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺少仓储应 500")
	}
	if _, _, err := svc.ListAuditLogs(context.Background(), AuditListQuery{}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺少仓储应 500")
	}
	if _, err := svc.ListComments(context.Background(), BizTypeTicket, 1, false); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺少仓储应 500")
	}
	if err := svc.DeleteAttachment(context.Background(), Operator{ID: 1}, 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺少仓储应 500")
	}
	if _, err := svc.GetUser(context.Background(), 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺少仓储应 500")
	}
	if _, err := svc.CreateUser(context.Background(), 1, sampleUserRequest()); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺少仓储应 500")
	}
	if err := svc.DeleteUser(context.Background(), 1, 2); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺少仓储应 500")
	}
	if _, err := svc.CreateSLAPolicy(context.Background(), 1, SLAPolicyRequest{Priority: "P1"}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺少仓储应 500")
	}
	if err := svc.DeleteSLAPolicy(context.Background(), 1, 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺少仓储应 500")
	}
	if _, err := svc.CreateComment(context.Background(), Operator{ID: 1}, CommentRequest{BizType: BizTypeTicket, BizID: 1, Content: "x"}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺少仓储应 500")
	}
}

func TestNewServiceDefaults(t *testing.T) {
	svc := NewService(Deps{})
	if svc.MaxUploadBytes() <= 0 {
		t.Fatalf("默认上传上限应 > 0")
	}
	_ = svc
}
