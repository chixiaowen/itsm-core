package ticket

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
	repo     *fakeRepo
	cats     *fakeCategoryRepo
	users    *fakeUsers
	slas     *fakeSLAs
	comments *fakeCommentStore
	svc      *Service
	clock    *time.Time
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	base := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	e := &testEnv{
		repo:     newFakeRepo(),
		cats:     newFakeCategoryRepo(),
		users:    newFakeUsers(),
		slas:     newFakeSLAs(),
		comments: newFakeCommentStore(),
	}
	cur := base
	e.clock = &cur
	e.svc = NewService(Deps{
		Repo: e.repo, Categories: e.cats, Users: e.users, SLAs: e.slas, Comments: e.comments,
		Now: func() time.Time { return *e.clock },
	})
	return e
}

func (e *testEnv) advance(d time.Duration) { *e.clock = e.clock.Add(d) }

func appErrCode(t *testing.T, err error) int {
	t.Helper()
	var ae *httpx.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("期望 *httpx.AppError，实际 %T (%v)", err, err)
	}
	return ae.Code
}

func agent(id uint64) Actor { return Actor{UserID: id, Role: role.Agent} }
func admin() Actor          { return Actor{UserID: 9, Role: role.Admin} }
func requestor(id uint64) Actor {
	return Actor{UserID: id, Role: role.Requestor}
}

// createTicket 创建一个 P3 工单并返回。
func (e *testEnv) createTicket(t *testing.T, reqActor Actor) *Ticket {
	t.Helper()
	tk, err := e.svc.Create(context.Background(), reqActor, CreateTicketRequest{Title: "打印机故障", Priority: PriorityP3, RequesterID: &reqActor.UserID})
	if err != nil {
		t.Fatalf("创建工单失败: %v", err)
	}
	return tk
}

// drive 将工单推进到目标状态（走合法流转路径）。
func (e *testEnv) drive(t *testing.T, id uint64, actions ...step) *Ticket {
	t.Helper()
	var tk *Ticket
	for _, a := range actions {
		got, err := e.svc.Transition(context.Background(), agent(7), id, a.action, a.req)
		if err != nil {
			t.Fatalf("流转 %s 失败: %v", a.action, err)
		}
		tk = got
	}
	return tk
}

type step = struct {
	action string
	req    TransitionRequest
}

// ---------------- 创建 & SLA ----------------

func TestCreate_SuccessAndSLA(t *testing.T) {
	e := newTestEnv(t)
	tk := e.createTicket(t, requestor(20))
	if !strings.HasPrefix(tk.Code, "TKT-20260916-") {
		t.Fatalf("编号格式不符: %s", tk.Code)
	}
	if tk.Status != StatusNew {
		t.Fatalf("初始状态应为 new，实际 %s", tk.Status)
	}
	if tk.SLAStatus == "" {
		t.Fatalf("应注入 SLA 状态")
	}
	if tk.ResolveDueAt == nil || tk.ResponseDueAt == nil {
		t.Fatalf("应落 SLA 截止时间")
	}
	// P3 响应 120m
	want := e.clock.Add(120 * time.Minute)
	if !tk.ResponseDueAt.Equal(want) {
		t.Fatalf("响应截止不符: got=%v want=%v", tk.ResponseDueAt, want)
	}
}

func TestCreate_InvalidPriorityAndEmptyTitle(t *testing.T) {
	e := newTestEnv(t)
	if _, err := e.svc.Create(context.Background(), requestor(20), CreateTicketRequest{Title: "x", Priority: "P9"}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法优先级应 400，实际 %v", err)
	}
	if _, err := e.svc.Create(context.Background(), requestor(20), CreateTicketRequest{Title: "   "}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("空标题应 400")
	}
}

func TestCreateFromIncidentAndServiceItem(t *testing.T) {
	e := newTestEnv(t)
	id, code, err := e.svc.CreateFromIncident(context.Background(), "宕机", "db down", 20, 100, PriorityP1)
	if err != nil {
		t.Fatalf("转单失败: %v", err)
	}
	tk, _ := e.repo.Get(context.Background(), id)
	if tk.SourceIncidentID == nil || *tk.SourceIncidentID != 100 || tk.Type != TypeIncident {
		t.Fatalf("转单字段不符: %+v", tk)
	}
	if !strings.HasPrefix(code, "TKT-") {
		t.Fatalf("编号前缀不符: %s", code)
	}

	_, _, err = e.svc.CreateFromServiceItem(context.Background(), "开通账号", "", 20, 1, 5, PriorityP4, `{"a":1}`)
	if err != nil {
		t.Fatalf("下单转单失败: %v", err)
	}
}

// ---------------- 状态机（合法路径 + 非法流转）----------------

func TestMachine_LegalPath(t *testing.T) {
	e := newTestEnv(t)
	tk := e.createTicket(t, requestor(20))
	aid := uint64(7)

	got := e.drive(t, tk.ID,
		step{ActionAssign, TransitionRequest{Action: ActionAssign, AssigneeID: &aid}},
		step{ActionStart, TransitionRequest{Action: ActionStart}},
		step{ActionPending, TransitionRequest{Action: ActionPending, Reason: "等待供应商"}},
		step{ActionResume, TransitionRequest{Action: ActionResume}},
		step{ActionResolve, TransitionRequest{Action: ActionResolve, Solution: "更换硒鼓"}},
		step{ActionClose, TransitionRequest{Action: ActionClose}},
	)
	if got.Status != StatusClosed {
		t.Fatalf("最终状态应为 closed，实际 %s", got.Status)
	}
	if got.ResolvedAt == nil || got.ClosedAt == nil {
		t.Fatalf("应写入 resolved_at / closed_at")
	}
}

func TestMachine_IllegalTransitions(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// new → in_progress（跳过 assign）→ 409
	tk := e.createTicket(t, requestor(20))
	if _, err := e.svc.Transition(ctx, agent(7), tk.ID, ActionStart, TransitionRequest{Action: ActionStart}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("new→in_progress 应 409，实际 %v", err)
	}

	// draft → resolved（不在表）→ 409；同时 draft→cancel 合法
	draft := &Ticket{Code: "TKT-20260916-9999", Title: "草稿", Status: StatusDraft, Priority: PriorityP4, RequesterID: 20}
	_ = e.repo.Create(ctx, draft)
	if _, err := e.svc.Transition(ctx, agent(7), draft.ID, ActionResolve, TransitionRequest{Action: ActionResolve, Solution: "x"}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("draft→resolved 应 409，实际 %v", err)
	}

	// resolved → assigned（不在表）→ 409
	resolved := &Ticket{Code: "TKT-20260916-8888", Title: "已解决", Status: StatusResolved, Priority: PriorityP4, RequesterID: 20}
	_ = e.repo.Create(ctx, resolved)
	if _, err := e.svc.Transition(ctx, agent(7), resolved.ID, ActionAssign, TransitionRequest{Action: ActionAssign, AssigneeID: ptrU64(7)}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("resolved→assigned 应 409，实际 %v", err)
	}

	// closed → 任意 → 409
	closed := &Ticket{Code: "TKT-20260916-7777", Title: "已关闭", Status: StatusClosed, Priority: PriorityP4, RequesterID: 20}
	_ = e.repo.Create(ctx, closed)
	if _, err := e.svc.Transition(ctx, agent(7), closed.ID, ActionStart, TransitionRequest{Action: ActionStart}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("closed→* 应 409，实际 %v", err)
	}
}

func TestResolve_RequiresSolution(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	aid := uint64(7)
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionAssign, TransitionRequest{Action: ActionAssign, AssigneeID: &aid})
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionStart, TransitionRequest{Action: ActionStart})

	// 无 solution → 422
	if _, err := e.svc.Transition(ctx, agent(7), tk.ID, ActionResolve, TransitionRequest{Action: ActionResolve}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("无 solution 解决应 422，实际 %v", err)
	}
	// 有 solution → 成功
	got, err := e.svc.Transition(ctx, agent(7), tk.ID, ActionResolve, TransitionRequest{Action: ActionResolve, Solution: "已修复"})
	if err != nil || got.Status != StatusResolved {
		t.Fatalf("解决失败: %v", err)
	}
}

func TestReopen_Window(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	aid := uint64(7)
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionAssign, TransitionRequest{Action: ActionAssign, AssigneeID: &aid})
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionStart, TransitionRequest{Action: ActionStart})
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionResolve, TransitionRequest{Action: ActionResolve, Solution: "ok"})

	// 请求人 7 天内重开 → reopened
	got, err := e.svc.Transition(ctx, requestor(20), tk.ID, ActionReopen, TransitionRequest{Action: ActionReopen})
	if err != nil || got.Status != StatusReopened {
		t.Fatalf("7 天内重开应成功: %v", err)
	}

	// 超过 7 天再重开另一张 → 409
	e2 := newTestEnv(t)
	tk2 := e2.createTicket(t, requestor(20))
	_, _ = e2.svc.Transition(ctx, agent(7), tk2.ID, ActionAssign, TransitionRequest{Action: ActionAssign, AssigneeID: &aid})
	_, _ = e2.svc.Transition(ctx, agent(7), tk2.ID, ActionStart, TransitionRequest{Action: ActionStart})
	_, _ = e2.svc.Transition(ctx, agent(7), tk2.ID, ActionResolve, TransitionRequest{Action: ActionResolve, Solution: "ok"})
	e2.advance(8 * 24 * time.Hour)
	if _, err := e2.svc.Transition(ctx, requestor(20), tk2.ID, ActionReopen, TransitionRequest{Action: ActionReopen}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("超过 7 天重开应 409，实际 %v", err)
	}
}

func TestCancel_OnlyOwner(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	// 非本人 requestor 取消 → 403
	if _, err := e.svc.Transition(ctx, requestor(21), tk.ID, ActionCancel, TransitionRequest{Action: ActionCancel}); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("非本人取消应 403，实际 %v", err)
	}
	// 本人取消 → cancelled
	got, err := e.svc.Transition(ctx, requestor(20), tk.ID, ActionCancel, TransitionRequest{Action: ActionCancel})
	if err != nil || got.Status != StatusCancelled {
		t.Fatalf("本人取消应成功: %v", err)
	}
}

func TestAssign_InvalidAssignee(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	if _, err := e.svc.Assign(ctx, agent(7), tk.ID, 999); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("无效指派人应 422，实际 %v", err)
	}
	if _, err := e.svc.Assign(ctx, agent(7), tk.ID, 7); err != nil {
		t.Fatalf("有效指派应成功: %v", err)
	}
}

func TestStart_OnlyAssigneeOrAdmin(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	aid := uint64(7)
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionAssign, TransitionRequest{Action: ActionAssign, AssigneeID: &aid})
	// 非指派人（agent 8）→ 403
	if _, err := e.svc.Transition(ctx, agent(8), tk.ID, ActionStart, TransitionRequest{Action: ActionStart}); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("非指派人 start 应 403，实际 %v", err)
	}
	// admin → 放行
	if _, err := e.svc.Transition(ctx, admin(), tk.ID, ActionStart, TransitionRequest{Action: ActionStart}); err != nil {
		t.Fatalf("admin start 应成功: %v", err)
	}
}

// ---------------- 挂起/恢复 + SLA 回补 ----------------

func TestPauseResume_PausedMinutesAndDueShift(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	aid := uint64(7)
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionAssign, TransitionRequest{Action: ActionAssign, AssigneeID: &aid})
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionStart, TransitionRequest{Action: ActionStart})

	dueBefore := *tk.ResolveDueAt
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionPending, TransitionRequest{Action: ActionPending, Reason: "等待用户"})
	e.advance(60 * time.Minute)
	got, err := e.svc.Transition(ctx, agent(7), tk.ID, ActionResume, TransitionRequest{Action: ActionResume})
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if got.PausedMinutes != 60 {
		t.Fatalf("paused_minutes 应为 60，实际 %d", got.PausedMinutes)
	}
	if got.ResolveDueAt == nil || !got.ResolveDueAt.Equal(dueBefore.Add(60*time.Minute)) {
		t.Fatalf("解决截止应回补 60m: got=%v want=%v", got.ResolveDueAt, dueBefore.Add(60*time.Minute))
	}
	if got.Status != StatusInProgress {
		t.Fatalf("恢复后应为 in_progress，实际 %s", got.Status)
	}
}

func TestPending_RequiresReason(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	aid := uint64(7)
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionAssign, TransitionRequest{Action: ActionAssign, AssigneeID: &aid})
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionStart, TransitionRequest{Action: ActionStart})
	if _, err := e.svc.Transition(ctx, agent(7), tk.ID, ActionPending, TransitionRequest{Action: ActionPending}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("无原因挂起应 422，实际 %v", err)
	}
}

// ---------------- 评价 ----------------

func TestRating_OnlyOnceAndOnlyRequestor(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	aid := uint64(7)
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionAssign, TransitionRequest{Action: ActionAssign, AssigneeID: &aid})
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionStart, TransitionRequest{Action: ActionStart})

	// 未解决 → 422
	if _, err := e.svc.Rate(ctx, requestor(20), tk.ID, RatingRequest{Rating: 5}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("未解决评价应 422")
	}
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionResolve, TransitionRequest{Action: ActionResolve, Solution: "ok"})

	// 非请求人 → 403
	if _, err := e.svc.Rate(ctx, agent(8), tk.ID, RatingRequest{Rating: 5}); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("非请求人评价应 403，实际 %v", err)
	}
	// 请求人评价 → 成功
	if _, err := e.svc.Rate(ctx, requestor(20), tk.ID, RatingRequest{Rating: 5, Comment: "good"}); err != nil {
		t.Fatalf("评价失败: %v", err)
	}
	// 重复 → 409
	if _, err := e.svc.Rate(ctx, requestor(20), tk.ID, RatingRequest{Rating: 4}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("重复评价应 409，实际 %v", err)
	}
}

// ---------------- 列表 / 鉴权 ----------------

func TestList_RequesterOnlyOwn(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	_, _ = e.svc.Create(ctx, requestor(20), CreateTicketRequest{Title: "A", RequesterID: ptrU64(20)})
	_, _ = e.svc.Create(ctx, requestor(21), CreateTicketRequest{Title: "B", RequesterID: ptrU64(21)})

	items, total, err := e.svc.List(ctx, requestor(20), ListQuery{Offset: 0, Limit: 10})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].Title != "A" {
		t.Fatalf("requestor 应仅见本人: total=%d", total)
	}

	// agent 可见全部
	_, total, _ = e.svc.List(ctx, agent(7), ListQuery{Offset: 0, Limit: 10})
	if total != 2 {
		t.Fatalf("agent 应见全部 2 条，实际 %d", total)
	}

	// 显式 requester_id 过滤（staff）
	_, total, _ = e.svc.List(ctx, agent(7), ListQuery{RequesterID: ptrU64(21), Offset: 0, Limit: 10})
	if total != 1 {
		t.Fatalf("requester_id 过滤应 1，实际 %d", total)
	}
}

func TestList_SLAStatusFilter(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	_, _ = e.svc.Create(ctx, requestor(20), CreateTicketRequest{Title: "A", Priority: PriorityP1})

	// 未超期 → normal
	items, _, _ := e.svc.List(ctx, agent(7), ListQuery{SLAStatus: "normal", Offset: 0, Limit: 10})
	if len(items) != 1 {
		t.Fatalf("normal 应为 1，实际 %d", len(items))
	}
	// 前进 5 小时（P1 解决 240m）→ breached
	e.advance(5 * time.Hour)
	items, total, _ := e.svc.List(ctx, agent(7), ListQuery{SLAStatus: "breached", Offset: 0, Limit: 10})
	if total != 1 || len(items) != 1 {
		t.Fatalf("breached 应为 1，实际 total=%d", total)
	}
}

func TestGet_AccessDenied(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	if _, err := e.svc.Get(ctx, requestor(21), tk.ID); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("非本人查看应 403，实际 %v", err)
	}
	if _, err := e.svc.Get(ctx, admin(), tk.ID); err != nil {
		t.Fatalf("admin 查看应成功: %v", err)
	}
	if _, err := e.svc.Get(ctx, admin(), 9999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404")
	}
}

// ---------------- 编辑 / 删除 ----------------

func TestUpdate_PriorityRecalcSLA(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20)) // P3
	p1 := PriorityP1
	got, err := e.svc.Update(ctx, agent(7), tk.ID, UpdateTicketRequest{Priority: &p1})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if got.Priority != PriorityP1 {
		t.Fatalf("优先级未更新")
	}
	want := e.clock.Add(15 * time.Minute) // P1 响应 15m
	if !got.ResponseDueAt.Equal(want) {
		t.Fatalf("改优先级后 SLA 应重算: got=%v want=%v", got.ResponseDueAt, want)
	}
}

func TestUpdate_ForbiddenWhenClosed(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	closed := &Ticket{Code: "TKT-20260916-5555", Title: "x", Status: StatusClosed, Priority: PriorityP4, RequesterID: 20}
	_ = e.repo.Create(ctx, closed)
	title := "y"
	if _, err := e.svc.Update(ctx, agent(7), closed.ID, UpdateTicketRequest{Title: &title}); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("已关闭编辑应 403，实际 %v", err)
	}
}

func TestDelete(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	if err := e.svc.Delete(ctx, admin(), tk.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if err := e.svc.Delete(ctx, admin(), tk.ID); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("重复删除应 404")
	}
}

// ---------------- 评论（首次响应）----------------

func TestComment_FirstResponse(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	// requestor 评论不触发首次响应
	_, _ = e.svc.AddComment(ctx, requestor(20), tk.ID, TicketCommentRequest{Content: "补充信息"})
	got, _ := e.svc.Get(ctx, agent(7), tk.ID)
	if got.FirstRespondedAt != nil {
		t.Fatalf("requestor 评论不应触发首次响应")
	}
	// 坐席公开回复 → 触发
	_, err := e.svc.AddComment(ctx, agent(7), tk.ID, TicketCommentRequest{Content: "正在处理"})
	if err != nil {
		t.Fatalf("坐席评论失败: %v", err)
	}
	got, _ = e.svc.Get(ctx, agent(7), tk.ID)
	if got.FirstRespondedAt == nil {
		t.Fatalf("坐席公开回复应触发首次响应")
	}
	// 空内容 → 400
	if _, err := e.svc.AddComment(ctx, agent(7), tk.ID, TicketCommentRequest{Content: "  "}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("空评论应 400")
	}
}

func TestComment_InternalHiddenFromDetail(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	_, _ = e.svc.AddComment(ctx, agent(7), tk.ID, TicketCommentRequest{Content: "公开", IsInternal: false})
	_, _ = e.svc.AddComment(ctx, agent(7), tk.ID, TicketCommentRequest{Content: "内部", IsInternal: true})

	d, err := e.svc.Detail(ctx, requestor(20), tk.ID, false)
	if err != nil {
		t.Fatalf("Detail 失败: %v", err)
	}
	if len(d.Comments) != 1 {
		t.Fatalf("请求人应仅见公开评论，实际 %d", len(d.Comments))
	}
}

// ---------------- 关联 CI ----------------

func TestCIs_AttachListDetach(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	if err := e.svc.AddCIs(ctx, agent(7), tk.ID, []uint64{101, 102}); err != nil {
		t.Fatalf("关联 CI 失败: %v", err)
	}
	if err := e.svc.AddCIs(ctx, agent(7), tk.ID, nil); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("空 ci_ids 应 400")
	}
	d, _ := e.svc.Detail(ctx, admin(), tk.ID, false)
	if len(d.CIs) != 2 {
		t.Fatalf("关联 CI 应 2，实际 %d", len(d.CIs))
	}
	if err := e.svc.RemoveCI(ctx, agent(7), tk.ID, 101); err != nil {
		t.Fatalf("解除 CI 失败: %v", err)
	}
	d, _ = e.svc.Detail(ctx, admin(), tk.ID, false)
	if len(d.CIs) != 1 {
		t.Fatalf("解除后应 1，实际 %d", len(d.CIs))
	}
}

// ---------------- 分类 ----------------

func TestCategory_CRUDAndDeleteGuards(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	parent, err := e.svc.CreateCategory(ctx, admin(), CategoryRequest{Name: "硬件故障"})
	if err != nil {
		t.Fatalf("创建分类失败: %v", err)
	}
	child, _ := e.svc.CreateCategory(ctx, admin(), CategoryRequest{Name: "打印机", ParentID: &parent.ID})

	// 含子分类 → 409
	if err := e.svc.DeleteCategory(ctx, admin(), parent.ID); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("含子分类删除应 409，实际 %v", err)
	}
	// 含工单 → 409
	e.cats.ticketCount[child.ID] = 3
	if err := e.svc.DeleteCategory(ctx, admin(), child.ID); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("含工单删除应 409，实际 %v", err)
	}
	// 正常删除
	e.cats.ticketCount[child.ID] = 0
	if err := e.svc.DeleteCategory(ctx, admin(), child.ID); err != nil {
		t.Fatalf("删除分类失败: %v", err)
	}
	// 更新
	upd, err := e.svc.UpdateCategory(ctx, admin(), parent.ID, CategoryRequest{Name: "硬件类", SortOrder: 2})
	if err != nil || upd.Name != "硬件类" {
		t.Fatalf("更新分类失败: %v", err)
	}
	// 列表
	list, _ := e.svc.ListCategories(ctx)
	if len(list) != 1 {
		t.Fatalf("分类列表应 1，实际 %d", len(list))
	}
	// 不存在
	if _, err := e.svc.UpdateCategory(ctx, admin(), 999, CategoryRequest{Name: "x"}); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("更新不存在分类应 404")
	}
}

// ---------------- 缺依赖 ----------------

func TestService_MissingRepo(t *testing.T) {
	svc := NewService(Deps{})
	if _, err := svc.Create(context.Background(), agent(7), CreateTicketRequest{Title: "x"}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
	if _, err := svc.Get(context.Background(), agent(7), 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
	if _, _, err := svc.List(context.Background(), agent(7), ListQuery{}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
}

// ---------------- helpers ----------------

func ptrU64(v uint64) *uint64 { return &v }
