package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

type testEnv struct {
	cats    *fakeCategoryRepo
	items   *fakeItemRepo
	audit   *fakeAuditWriter
	tickets *fakeTicketCreator
	svc     *Service
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	e := &testEnv{
		cats:    newFakeCategoryRepo(),
		items:   newFakeItemRepo(),
		audit:   newFakeAuditWriter(),
		tickets: newFakeTicketCreator(),
	}
	e.svc = NewService(Deps{
		Categories: e.cats,
		Items:      e.items,
		Audit:      e.audit,
		Tickets:    e.tickets,
		Now:        func() time.Time { return time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC) },
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

func ptrU64(v uint64) *uint64 { return &v }

func adminOp() Operator { return Operator{ID: 1, Role: role.Admin} }

const validSchema = `{"fields":[{"name":"reason","label":"申请原因","type":"text","required":true},{"name":"count","label":"数量","type":"number","required":false}]}`

// seedCategory 新建一个顶层分类。
func (e *testEnv) seedCategory(t *testing.T, name string) *ServiceCategory {
	t.Helper()
	c, err := e.svc.CreateCategory(context.Background(), adminOp(), CategoryRequest{Name: name, SortOrder: 1})
	if err != nil {
		t.Fatalf("seedCategory 失败: %v", err)
	}
	return c
}

// seedItem 新建一个 draft 服务项（已绑定 SLA 与表单定义）。
func (e *testEnv) seedItem(t *testing.T, name string, catID uint64) *ServiceItem {
	t.Helper()
	it, err := e.svc.CreateItem(context.Background(), adminOp(), ItemRequest{
		Name:            name,
		CategoryID:      catID,
		SLAPolicyID:     ptrU64(1),
		DefaultPriority: "P3",
		FormSchema:      validSchema,
	})
	if err != nil {
		t.Fatalf("seedItem 失败: %v", err)
	}
	return it
}

// publishItem 把 draft 服务项推到 published。
func (e *testEnv) publishItem(t *testing.T, id uint64) *ServiceItem {
	t.Helper()
	ctx := context.Background()
	if _, err := e.svc.TransitionItem(ctx, adminOp(), id, ActionSubmitReview, ""); err != nil {
		t.Fatalf("submit_review 失败: %v", err)
	}
	it, err := e.svc.PublishItem(ctx, adminOp(), id)
	if err != nil {
		t.Fatalf("publish 失败: %v", err)
	}
	return it
}

// ---------------- 分类 ----------------

func TestCategory_CRUDAndTree(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	root := e.seedCategory(t, "办公支持")
	child, err := e.svc.CreateCategory(ctx, adminOp(), CategoryRequest{Name: "账号与权限", ParentID: ptrU64(root.ID), SortOrder: 2})
	if err != nil {
		t.Fatalf("创建子分类失败: %v", err)
	}
	_ = child

	nodes, err := e.svc.ListCategoryTree(ctx)
	if err != nil {
		t.Fatalf("ListCategoryTree 失败: %v", err)
	}
	if len(nodes) != 1 || len(nodes[0].Children) != 1 {
		t.Fatalf("分类树层级不符: %+v", nodes)
	}
	if nodes[0].Children[0].Name != "账号与权限" {
		t.Fatalf("子分类名不符: %+v", nodes[0].Children[0])
	}
}

func TestCategory_UpdateAndSelfParent(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	root := e.seedCategory(t, "根")

	if _, err := e.svc.UpdateCategory(ctx, adminOp(), root.ID, CategoryRequest{Name: "根2", ParentID: ptrU64(root.ID)}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("父分类为自身应 400，实际 %v", err)
	}
	got, err := e.svc.UpdateCategory(ctx, adminOp(), root.ID, CategoryRequest{Name: "根2", SortOrder: 9})
	if err != nil {
		t.Fatalf("UpdateCategory 失败: %v", err)
	}
	if got.Name != "根2" || got.SortOrder != 9 {
		t.Fatalf("更新字段不符: %+v", got)
	}
}

func TestCategory_DeleteWithChildren409(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	root := e.seedCategory(t, "根")
	if _, err := e.svc.CreateCategory(ctx, adminOp(), CategoryRequest{Name: "子", ParentID: ptrU64(root.ID)}); err != nil {
		t.Fatalf("建子分类失败: %v", err)
	}
	if err := e.svc.DeleteCategory(ctx, adminOp(), root.ID); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("含子分类删除应 409，实际 %v", err)
	}
}

func TestCategory_DeleteWithItems409(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	root := e.seedCategory(t, "根")
	_ = e.seedItem(t, "服务项", root.ID)
	if err := e.svc.DeleteCategory(ctx, adminOp(), root.ID); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("含服务项删除应 409，实际 %v", err)
	}
}

func TestCategory_DeleteOK(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	root := e.seedCategory(t, "根")
	if err := e.svc.DeleteCategory(ctx, adminOp(), root.ID); err != nil {
		t.Fatalf("空分类删除应成功，实际 %v", err)
	}
	if err := e.svc.DeleteCategory(ctx, adminOp(), root.ID); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("重复删除应 404，实际 %v", err)
	}
}

func TestCategory_CreateWithBadParent(t *testing.T) {
	e := newTestEnv(t)
	_, err := e.svc.CreateCategory(context.Background(), adminOp(), CategoryRequest{Name: "x", ParentID: ptrU64(999)})
	if appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("父分类不存在应 400，实际 %v", err)
	}
}

// ---------------- 服务项 CRUD / 状态机 ----------------

func TestItem_CreateAndGet(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	cat := e.seedCategory(t, "根")
	it := e.seedItem(t, "申请 VPN", cat.ID)
	if it.Status != StatusDraft {
		t.Fatalf("新服务项应为 draft，实际 %s", it.Status)
	}
	if it.DefaultPriority != "P3" {
		t.Fatalf("默认优先级不符: %s", it.DefaultPriority)
	}
	got, err := e.svc.GetItem(ctx, it.ID)
	if err != nil || got.Name != "申请 VPN" {
		t.Fatalf("GetItem 失败: %v", err)
	}
	if _, err := e.svc.GetItem(ctx, 9999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404，实际 %v", err)
	}
}

func TestItem_CreateBadCategory(t *testing.T) {
	e := newTestEnv(t)
	_, err := e.svc.CreateItem(context.Background(), adminOp(), ItemRequest{Name: "x", CategoryID: 123, FormSchema: validSchema})
	if appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("分类不存在应 400，实际 %v", err)
	}
}

func TestItem_CreateBadFormSchema(t *testing.T) {
	e := newTestEnv(t)
	cat := e.seedCategory(t, "根")
	_, err := e.svc.CreateItem(context.Background(), adminOp(), ItemRequest{
		Name: "x", CategoryID: cat.ID, FormSchema: `{"fields":[{"name":"a","type":"nope"}]}`,
	})
	if appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法字段类型应 400，实际 %v", err)
	}
}

func TestItem_PublishFlow(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	cat := e.seedCategory(t, "根")
	it := e.seedItem(t, "重装系统", cat.ID)

	// submit_review 前置未满足（无 SLA）→ 422
	noSLA, _ := e.svc.CreateItem(ctx, adminOp(), ItemRequest{Name: "无SLA", CategoryID: cat.ID, FormSchema: validSchema})
	if _, err := e.svc.TransitionItem(ctx, adminOp(), noSLA.ID, ActionSubmitReview, ""); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("缺 SLA 提交审核应 422，实际 %v", err)
	}

	// reject 缺 reason → 422
	if _, err := e.svc.TransitionItem(ctx, adminOp(), it.ID, ActionSubmitReview, ""); err != nil {
		t.Fatalf("submit_review 失败: %v", err)
	}
	if _, err := e.svc.TransitionItem(ctx, adminOp(), it.ID, ActionReject, ""); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("驳回缺原因应 422，实际 %v", err)
	}

	// reject 带原因 → draft；再提交并发布
	if _, err := e.svc.TransitionItem(ctx, adminOp(), it.ID, ActionReject, "资料不全"); err != nil {
		t.Fatalf("reject 失败: %v", err)
	}
	got, err := e.svc.GetItem(ctx, it.ID)
	if err != nil || got.Status != StatusDraft {
		t.Fatalf("驳回后应为 draft: %+v %v", got, err)
	}
	pub := e.publishItem(t, it.ID)
	if pub.Status != StatusPublished {
		t.Fatalf("发布后应为 published，实际 %s", pub.Status)
	}
}

func TestItem_PublishedEditReturnsDraft(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	cat := e.seedCategory(t, "根")
	it := e.seedItem(t, "服务A", cat.ID)
	e.publishItem(t, it.ID)

	got, err := e.svc.UpdateItem(ctx, adminOp(), it.ID, ItemRequest{
		Name: "服务A改", CategoryID: cat.ID, SLAPolicyID: ptrU64(1), FormSchema: validSchema,
	})
	if err != nil {
		t.Fatalf("published 编辑失败: %v", err)
	}
	if got.Status != StatusDraft {
		t.Fatalf("published 编辑后应退回 draft，实际 %s", got.Status)
	}
}

func TestItem_OfflineAndRepublish(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	cat := e.seedCategory(t, "根")
	it := e.seedItem(t, "服务A", cat.ID)
	e.publishItem(t, it.ID)

	off, err := e.svc.OfflineItem(ctx, adminOp(), it.ID)
	if err != nil || off.Status != StatusOffline {
		t.Fatalf("下线失败: %+v %v", off, err)
	}
	rep, err := e.svc.PublishItem(ctx, adminOp(), it.ID)
	if err != nil || rep.Status != StatusPublished {
		t.Fatalf("重新上架失败: %+v %v", rep, err)
	}
	// 已发布状态再 offine 一次合法；再从 published 发布 → 409
	if _, err := e.svc.PublishItem(ctx, adminOp(), it.ID); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("published 直接发布应 409，实际 %v", err)
	}
}

func TestItem_DeleteArchiveRules(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	cat := e.seedCategory(t, "根")

	// draft 可直接归档
	draft := e.seedItem(t, "草稿", cat.ID)
	if err := e.svc.DeleteItem(ctx, adminOp(), draft.ID); err != nil {
		t.Fatalf("draft 归档应成功，实际 %v", err)
	}
	got, _ := e.svc.GetItem(ctx, draft.ID)
	if got.Status != StatusArchived {
		t.Fatalf("归档后应为 archived，实际 %s", got.Status)
	}

	// published 不可归档 → 409
	pubItem := e.seedItem(t, "已发布", cat.ID)
	e.publishItem(t, pubItem.ID)
	if err := e.svc.DeleteItem(ctx, adminOp(), pubItem.ID); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("published 归档应 409，实际 %v", err)
	}

	// archived 为终态：任何流转 409
	if _, err := e.svc.PublishItem(ctx, adminOp(), draft.ID); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("archived 流转应 409，实际 %v", err)
	}
}

func TestItem_IllegalTransition409(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	cat := e.seedCategory(t, "根")
	it := e.seedItem(t, "服务A", cat.ID)
	// draft 不可直接 publish
	if _, err := e.svc.PublishItem(ctx, adminOp(), it.ID); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("draft 直接发布应 409，实际 %v", err)
	}
	// draft 不可 offline
	if _, err := e.svc.OfflineItem(ctx, adminOp(), it.ID); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("draft 下线应 409，实际 %v", err)
	}
}

func TestItem_TransitionRoleForbidden(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	cat := e.seedCategory(t, "根")
	it := e.seedItem(t, "服务A", cat.ID)
	// 非 admin（agent）提交审核 → 403
	_, err := e.svc.TransitionItem(ctx, Operator{ID: 2, Role: role.Agent}, it.ID, ActionSubmitReview, "")
	if appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("非 admin 流转应 403，实际 %v", err)
	}
}

func TestItem_UpdatePendingApproval409(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	cat := e.seedCategory(t, "根")
	it := e.seedItem(t, "服务A", cat.ID)
	if _, err := e.svc.TransitionItem(ctx, adminOp(), it.ID, ActionSubmitReview, ""); err != nil {
		t.Fatalf("submit_review 失败: %v", err)
	}
	_, err := e.svc.UpdateItem(ctx, adminOp(), it.ID, ItemRequest{Name: "x", CategoryID: cat.ID, FormSchema: validSchema})
	if appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("待审核编辑应 409，实际 %v", err)
	}
}

// TestItem_IllegalTransition409CarriesStatePair 校验非法流转 409 消息统一含「当前状态 -> 目标状态」。
func TestItem_IllegalTransition409CarriesStatePair(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	cat := e.seedCategory(t, "根")
	it := e.seedItem(t, "服务A", cat.ID)

	// 通用流转：draft 直接 publish（跳过提交审核）→ 409
	_, err := e.svc.TransitionItem(ctx, adminOp(), it.ID, ActionPublish, "")
	if appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("非法流转应 409，实际 %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "current="+StatusDraft) || !strings.Contains(msg, "target="+StatusPublished) {
		t.Fatalf("通用流转 409 消息应含 current=draft -> target=published，实际 %q", msg)
	}

	// PublishItem 分支：draft 直接发布 → 409，同样含状态对
	_, err = e.svc.PublishItem(ctx, adminOp(), it.ID)
	if appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("draft 直接发布应 409，实际 %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "current="+StatusDraft) || !strings.Contains(msg, "target="+StatusPublished) {
		t.Fatalf("PublishItem 409 消息应含 current=draft -> target=published，实际 %q", msg)
	}
}

// TestItem_PublishOfflineAdminOnly 校验服务项发布/下线仅 admin 可执行（对齐 PRD §5.6 触发角色）。
func TestItem_PublishOfflineAdminOnly(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	cat := e.seedCategory(t, "根")
	it := e.seedItem(t, "服务A", cat.ID)
	// 提交审核 draft -> pending_approval（admin）
	if _, err := e.svc.TransitionItem(ctx, adminOp(), it.ID, ActionSubmitReview, ""); err != nil {
		t.Fatalf("提交审核失败: %v", err)
	}
	// 负向：非 admin（cmdb_manager）发布 → 403
	if _, err := e.svc.PublishItem(ctx, Operator{ID: 9, Role: role.CmdbManager}, it.ID); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("非 admin 发布应 403，实际 %v", err)
	}
	// 正向：admin 发布 → published
	pub, err := e.svc.PublishItem(ctx, adminOp(), it.ID)
	if err != nil || pub.Status != StatusPublished {
		t.Fatalf("admin 发布失败: %+v %v", pub, err)
	}
	// 负向：非 admin 下线 → 403
	if _, err := e.svc.OfflineItem(ctx, Operator{ID: 9, Role: role.CmdbManager}, it.ID); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("非 admin 下线应 403，实际 %v", err)
	}
	// 正向：admin 下线 → offline
	off, err := e.svc.OfflineItem(ctx, adminOp(), it.ID)
	if err != nil || off.Status != StatusOffline {
		t.Fatalf("admin 下线失败: %+v %v", off, err)
	}
}

// ---------------- 用户侧 ----------------

func TestUserCatalog_OnlyPublished(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	cat := e.seedCategory(t, "根")
	draft := e.seedItem(t, "草稿项", cat.ID)
	pub := e.seedItem(t, "发布项", cat.ID)
	e.publishItem(t, pub.ID)
	_ = draft

	items, err := e.svc.UserItems(ctx, nil, "")
	if err != nil {
		t.Fatalf("UserItems 失败: %v", err)
	}
	if len(items) != 1 || items[0].ID != pub.ID {
		t.Fatalf("用户侧应仅含 published，实际 %+v", items)
	}

	nodes, err := e.svc.UserCategoryTree(ctx)
	if err != nil {
		t.Fatalf("UserCategoryTree 失败: %v", err)
	}
	if len(nodes) != 1 || nodes[0].ItemCount != 1 {
		t.Fatalf("用户侧分类树计数不符: %+v", nodes)
	}

	// 关键字过滤
	items, _ = e.svc.UserItems(ctx, nil, "发布")
	if len(items) != 1 {
		t.Fatalf("关键字过滤失败: %+v", items)
	}
}

// ---------------- 下单 ----------------

func TestOrder_Success(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	cat := e.seedCategory(t, "根")
	it := e.seedItem(t, "申请 VPN", cat.ID)
	e.publishItem(t, it.ID)

	res, err := e.svc.OrderItem(ctx, Operator{ID: 7, Role: role.Requestor}, it.ID, OrderRequest{
		FormData: map[string]any{"reason": "远程办公", "count": float64(2)},
	})
	if err != nil {
		t.Fatalf("下单失败: %v", err)
	}
	if res.TicketID == 0 {
		t.Fatalf("应返回工单 ID")
	}
	last := e.tickets.lastReq()
	if last.ServiceItemID != it.ID {
		t.Fatalf("工单应携带 service_item_id，实际 %d", last.ServiceItemID)
	}
	if last.RequesterID != 7 || last.SLAPolicyID == nil || *last.SLAPolicyID != 1 {
		t.Fatalf("工单应继承请求人/SLA 策略: %+v", last)
	}
	if last.FormData == "" || last.FormData == "null" {
		t.Fatalf("应快照 form_data，实际 %q", last.FormData)
	}
}

func TestOrder_NotPublishedRejected(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	cat := e.seedCategory(t, "根")
	it := e.seedItem(t, "草稿项", cat.ID)
	_, err := e.svc.OrderItem(ctx, Operator{ID: 7, Role: role.Requestor}, it.ID, OrderRequest{})
	if appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("非 published 下单应 409，实际 %v", err)
	}
}

func TestOrder_MissingRequiredField400(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	cat := e.seedCategory(t, "根")
	it := e.seedItem(t, "申请 VPN", cat.ID)
	e.publishItem(t, it.ID)
	// 缺 reason（必填）
	_, err := e.svc.OrderItem(ctx, Operator{ID: 7, Role: role.Requestor}, it.ID, OrderRequest{
		FormData: map[string]any{"count": float64(1)},
	})
	if appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("必填缺失应 400，实际 %v", err)
	}
	// count 类型错误
	_, err = e.svc.OrderItem(ctx, Operator{ID: 7, Role: role.Requestor}, it.ID, OrderRequest{
		FormData: map[string]any{"reason": "x", "count": "not-number"},
	})
	if appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("类型错误应 400，实际 %v", err)
	}
}

func TestOrder_ItemNotFound(t *testing.T) {
	e := newTestEnv(t)
	_, err := e.svc.OrderItem(context.Background(), Operator{ID: 7, Role: role.Requestor}, 999, OrderRequest{})
	if appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("服务项不存在应 404，实际 %v", err)
	}
}

func TestOrder_NoTicketCreator(t *testing.T) {
	e := newTestEnv(t)
	e.svc.tickets = nil
	cat := e.seedCategory(t, "根")
	it := e.seedItem(t, "x", cat.ID)
	e.publishItem(t, it.ID)
	_, err := e.svc.OrderItem(context.Background(), Operator{ID: 7, Role: role.Requestor}, it.ID, OrderRequest{FormData: map[string]any{"reason": "a"}})
	if appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("未装配工单服务应 500，实际 %v", err)
	}
}

// ---------------- 表单解析 ----------------

func TestFormSchema_ParseAndValidate(t *testing.T) {
	fs, err := ParseFormSchema(`{"fields":[{"name":"env","label":"环境","type":"select","required":true,"options":["prod","dev"]}]}`)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if err := fs.Validate(map[string]any{"env": "prod"}); err != nil {
		t.Fatalf("合法取值应通过: %v", err)
	}
	if err := fs.Validate(map[string]any{"env": "staging"}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法选项应 400，实际 %v", err)
	}
	if err := fs.Validate(map[string]any{}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("缺必填应 400，实际 %v", err)
	}
	// 空字符串视为空定义
	if fs2, err := ParseFormSchema("  "); err != nil || len(fs2.Fields) != 0 {
		t.Fatalf("空定义解析异常: %+v %v", fs2, err)
	}
	// 非法 JSON
	if _, err := ParseFormSchema("{bad"); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法 JSON 应 400，实际 %v", err)
	}
	// select 缺 options
	if _, err := ParseFormSchema(`{"fields":[{"name":"a","type":"select"}]}`); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("select 缺 options 应 400，实际 %v", err)
	}
}

func TestItem_AuditWritten(t *testing.T) {
	e := newTestEnv(t)
	cat := e.seedCategory(t, "根")
	e.seedItem(t, "x", cat.ID)
	if e.audit.count() < 2 {
		t.Fatalf("应写入审计（分类创建 + 服务项创建），实际 %d", e.audit.count())
	}
}
