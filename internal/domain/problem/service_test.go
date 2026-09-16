package problem

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
	repo       *fakeRepo
	changes    *fakeChanges
	incidents  *fakeIncidents
	changeRead *fakeChangeReader
	users      *fakeUsers
	auditor    *fakeAuditor
	svc        *Service
	clock      *time.Time
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	base := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	cur := base
	e := &testEnv{
		repo:       newFakeRepo(),
		changes:    newFakeChanges(),
		incidents:  newFakeIncidents(),
		changeRead: newFakeChangeReader(),
		users:      newFakeUsers(),
		auditor:    newFakeAuditor(),
		clock:      &cur,
	}
	e.svc = NewService(Deps{
		Repo: e.repo, Changes: e.changes, Incidents: e.incidents, ChangeRead: e.changeRead,
		Users: e.users, Auditor: e.auditor, Now: func() time.Time { return *e.clock },
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

func pm(id uint64) Actor   { return Actor{UserID: id, Role: role.ProblemManager} }
func admin() Actor         { return Actor{UserID: 9, Role: role.Admin} }
func reqr(id uint64) Actor { return Actor{UserID: id, Role: role.Requestor} }

// createManual 便捷创建 manual 问题。
func (e *testEnv) createManual(t *testing.T, actor Actor, title string) *Problem {
	t.Helper()
	p, err := e.svc.Create(context.Background(), actor, CreateRequest{Title: title, Source: SourceManual})
	if err != nil {
		t.Fatalf("创建问题失败: %v", err)
	}
	return p
}

// advanceTo 将问题推进到目标状态（走合法路径）。
func (e *testEnv) advanceTo(t *testing.T, id uint64, status string) *Problem {
	t.Helper()
	ctx := context.Background()
	aid := uint64(7)
	steps := map[string][]struct {
		action string
		req    TransitionRequest
	}{
		StatusTriage:        {{ActionTriage, TransitionRequest{Action: ActionTriage}}},
		StatusInvestigating: {{ActionTriage, TransitionRequest{Action: ActionTriage}}, {ActionInvestigate, TransitionRequest{Action: ActionInvestigate, AssigneeID: &aid}}},
		StatusKnownError:    {{ActionTriage, TransitionRequest{Action: ActionTriage}}, {ActionInvestigate, TransitionRequest{Action: ActionInvestigate, AssigneeID: &aid}}, {ActionMarkKnownError, TransitionRequest{Action: ActionMarkKnownError, RootCause: "根因", Workaround: "规避"}}},
		StatusResolved:      {{ActionTriage, TransitionRequest{Action: ActionTriage}}, {ActionInvestigate, TransitionRequest{Action: ActionInvestigate, AssigneeID: &aid}}, {ActionResolve, TransitionRequest{Action: ActionResolve, NoChangeReason: "无需变更"}}},
	}
	var last *Problem
	for _, s := range steps[status] {
		got, err := e.svc.Transition(ctx, pm(7), id, s.action, s.req)
		if err != nil {
			t.Fatalf("流转 %s 失败: %v", s.action, err)
		}
		last = got
	}
	return last
}

// ---------------- 创建 ----------------

func TestCreate_Manual(t *testing.T) {
	e := newTestEnv(t)
	p, err := e.svc.Create(context.Background(), pm(7), CreateRequest{Title: "数据库偶发超时"})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if p.Status != StatusNew {
		t.Fatalf("初始状态应 new，实际 %s", p.Status)
	}
	if p.Source != SourceManual {
		t.Fatalf("默认来源应 manual，实际 %s", p.Source)
	}
	if !strings.HasPrefix(p.Code, "PRB-20260916-") {
		t.Fatalf("编号前缀错误: %s", p.Code)
	}
	// 空标题 -> 400
	if _, err := e.svc.Create(context.Background(), pm(7), CreateRequest{Title: "  "}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("空标题应 400")
	}
	// 非法来源 -> 400
	if _, err := e.svc.Create(context.Background(), pm(7), CreateRequest{Title: "x", Source: "bad"}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法来源应 400")
	}
}

func TestCreate_Aggregate(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	e.incidents.seed(1, "resolved")
	e.incidents.seed(2, "resolved")

	p, err := e.svc.Create(ctx, pm(7), CreateRequest{Title: "聚合问题", Source: SourceAggregate, IncidentIDs: []uint64{1, 2}})
	if err != nil {
		t.Fatalf("聚合创建失败: %v", err)
	}
	if p.Source != SourceAggregate {
		t.Fatalf("来源应 aggregate，实际 %s", p.Source)
	}
	ids, _ := e.incidents.ListIncidentIDsByProblem(ctx, p.ID)
	if len(ids) != 2 {
		t.Fatalf("应挂载 2 个事件，实际 %d", len(ids))
	}

	// 重复聚合：事件已挂到未关闭问题 -> 409
	if _, err := e.svc.Create(ctx, pm(7), CreateRequest{Title: "再次聚合", Source: SourceAggregate, IncidentIDs: []uint64{1}}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("重复聚合应 409")
	}
	// 聚合不存在的事件 -> 400
	if _, err := e.svc.Create(ctx, pm(7), CreateRequest{Title: "不存在", Source: SourceAggregate, IncidentIDs: []uint64{999}}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("聚合不存在事件应 400")
	}
	// aggregate 无 incident_ids -> 400
	if _, err := e.svc.Create(ctx, pm(7), CreateRequest{Title: "无事件", Source: SourceAggregate}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("aggregate 无事件应 400")
	}
	// 事件已挂到已关闭问题 -> 允许再次聚合
	e.incidents.seed(3, "resolved")
	e.incidents.attach(3, p.ID)
	// 将 p 关闭后事件 3 可重新聚合
	e.advanceTo(t, p.ID, StatusResolved)
	if _, err := e.svc.Transition(ctx, pm(7), p.ID, ActionClose, TransitionRequest{Action: ActionClose}); err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
	if _, err := e.svc.Create(ctx, pm(7), CreateRequest{Title: "重新聚合", Source: SourceAggregate, IncidentIDs: []uint64{3}}); err != nil {
		t.Fatalf("已关闭问题的既有事件应可重新聚合: %v", err)
	}
}

// ---------------- 状态流转 ----------------

func TestTransition_HappyPath(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	p := e.createManual(t, pm(7), "主干网络抖动")

	if _, err := e.svc.Transition(ctx, pm(7), p.ID, ActionTriage, TransitionRequest{Action: ActionTriage}); err != nil {
		t.Fatalf("triage 失败: %v", err)
	}
	aid := uint64(7)
	got, err := e.svc.Transition(ctx, pm(7), p.ID, ActionInvestigate, TransitionRequest{Action: ActionInvestigate, AssigneeID: &aid})
	if err != nil {
		t.Fatalf("investigate 失败: %v", err)
	}
	if got.Status != StatusInvestigating || got.AssigneeID == nil || *got.AssigneeID != 7 {
		t.Fatalf("investigate 后状态/指派人错误: %+v", got)
	}
	got, err = e.svc.Transition(ctx, pm(7), p.ID, ActionMarkKnownError, TransitionRequest{Action: ActionMarkKnownError, RootCause: "交换机固件缺陷", Workaround: "重启交换机"})
	if err != nil {
		t.Fatalf("mark_known_error 失败: %v", err)
	}
	if got.Status != StatusKnownError || got.RootCause == "" || got.Workaround == "" {
		t.Fatalf("known_error 字段错误: %+v", got)
	}
	got, err = e.svc.Transition(ctx, pm(7), p.ID, ActionResolve, TransitionRequest{Action: ActionResolve, NoChangeReason: "厂商已热修复"})
	if err != nil {
		t.Fatalf("resolve 失败: %v", err)
	}
	if got.Status != StatusResolved || got.ResolvedAt == nil {
		t.Fatalf("resolve 后应写 resolved_at: %+v", got)
	}
	got, err = e.svc.Transition(ctx, pm(7), p.ID, ActionClose, TransitionRequest{Action: ActionClose})
	if err != nil {
		t.Fatalf("close 失败: %v", err)
	}
	if got.Status != StatusClosed || got.ClosedAt == nil {
		t.Fatalf("close 后应写 closed_at: %+v", got)
	}
	// 终态时间戳幂等：再次 close 非法（closed 不在状态机中）-> 409
	if _, err := e.svc.Transition(ctx, pm(7), p.ID, ActionClose, TransitionRequest{Action: ActionClose}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("closed 再次 close 应 409")
	}
}

func TestTransition_Guards(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// 非法流转 new -> resolve -> 409
	p1 := e.createManual(t, pm(7), "p1")
	if _, err := e.svc.Transition(ctx, pm(7), p1.ID, ActionResolve, TransitionRequest{Action: ActionResolve}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("new->resolve 应 409")
	}
	// 无权角色 requestor -> 403
	if _, err := e.svc.Transition(ctx, reqr(20), p1.ID, ActionTriage, TransitionRequest{Action: ActionTriage}); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("requestor 流转应 403")
	}
	// investigate 无 assignee -> 422
	p2 := e.createManual(t, pm(7), "p2")
	e.advanceTo(t, p2.ID, StatusTriage)
	if _, err := e.svc.Transition(ctx, pm(7), p2.ID, ActionInvestigate, TransitionRequest{Action: ActionInvestigate}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("investigate 无指派应 422")
	}
	// cancel 无 reason -> 422（new 态）
	p3 := e.createManual(t, pm(7), "p3")
	if _, err := e.svc.Transition(ctx, pm(7), p3.ID, ActionCancel, TransitionRequest{Action: ActionCancel}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("cancel 无原因应 422")
	}
	// cancel 有 reason -> 200
	if _, err := e.svc.Transition(ctx, pm(7), p3.ID, ActionCancel, TransitionRequest{Action: ActionCancel, Reason: "误报"}); err != nil {
		t.Fatalf("cancel 有原因应成功: %v", err)
	}

	// mark_known_error 缺字段 -> 422
	p4 := e.createManual(t, pm(7), "p4")
	e.advanceTo(t, p4.ID, StatusInvestigating)
	if _, err := e.svc.Transition(ctx, pm(7), p4.ID, ActionMarkKnownError, TransitionRequest{Action: ActionMarkKnownError, RootCause: "只有根因"}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("known_error 缺 workaround 应 422")
	}

	// resolve 无 closed 变更 且 无 no_change_reason -> 422
	if _, err := e.svc.Transition(ctx, pm(7), p4.ID, ActionResolve, TransitionRequest{Action: ActionResolve}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("resolve 无约束应 422")
	}
	// 关联 1 个 closed 变更后 resolve -> 成功
	_ = e.changes.Add(ctx, p4.ID, []uint64{101})
	e.changeRead.closed[101] = true
	if _, err := e.svc.Transition(ctx, pm(7), p4.ID, ActionResolve, TransitionRequest{Action: ActionResolve}); err != nil {
		t.Fatalf("有关联 closed 变更应可 resolve: %v", err)
	}
	// recur 无 reason -> 422；有 reason -> 成功
	if _, err := e.svc.Transition(ctx, pm(7), p4.ID, ActionRecur, TransitionRequest{Action: ActionRecur}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("recur 无原因应 422")
	}
	got, err := e.svc.Transition(ctx, pm(7), p4.ID, ActionRecur, TransitionRequest{Action: ActionRecur, Reason: "复发"})
	if err != nil {
		t.Fatalf("recur 应成功: %v", err)
	}
	if got.Status != StatusInvestigating || got.ResolvedAt != nil {
		t.Fatalf("recur 应回到 investigating 并清空 resolved_at: %+v", got)
	}
}

// ---------------- 已知错误 ----------------

func TestMarkKnownError(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	p := e.createManual(t, pm(7), "k1")

	// 非 investigating/known_error -> 409
	if _, err := e.svc.MarkKnownError(ctx, pm(7), p.ID, "根因", "规避"); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("new 态标记已知错误应 409")
	}
	// 缺字段 -> 422
	e.advanceTo(t, p.ID, StatusInvestigating)
	if _, err := e.svc.MarkKnownError(ctx, pm(7), p.ID, "", "规避"); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("缺 root_cause 应 422")
	}
	// investigating -> known_error
	got, err := e.svc.MarkKnownError(ctx, pm(7), p.ID, "根因", "规避")
	if err != nil {
		t.Fatalf("标记已知错误失败: %v", err)
	}
	if got.Status != StatusKnownError {
		t.Fatalf("应 known_error，实际 %s", got.Status)
	}
	// known_error -> 更新 workaround（自环）
	got, err = e.svc.MarkKnownError(ctx, pm(7), p.ID, "根因2", "规避2")
	if err != nil {
		t.Fatalf("更新 workaround 失败: %v", err)
	}
	if got.Status != StatusKnownError || got.Workaround != "规避2" {
		t.Fatalf("更新后字段错误: %+v", got)
	}
}

// ---------------- 关联变更 ----------------

func TestAddRemoveChanges(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	p := e.createManual(t, pm(7), "c1")

	if err := e.svc.AddChanges(ctx, pm(7), p.ID, []uint64{201, 202}); err != nil {
		t.Fatalf("关联变更失败: %v", err)
	}
	ids, _ := e.changes.ListChangeIDs(ctx, p.ID)
	if len(ids) != 2 {
		t.Fatalf("应关联 2 个变更，实际 %d", len(ids))
	}
	// 空列表 -> 400
	if err := e.svc.AddChanges(ctx, pm(7), p.ID, nil); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("空 change_ids 应 400")
	}
	// 解除关联
	if err := e.svc.RemoveChange(ctx, pm(7), p.ID, 201); err != nil {
		t.Fatalf("解除关联失败: %v", err)
	}
	ids, _ = e.changes.ListChangeIDs(ctx, p.ID)
	if len(ids) != 1 || ids[0] != 202 {
		t.Fatalf("解除后应剩 202，实际 %v", ids)
	}
	// 详情包含关联 id
	d, err := e.svc.Detail(ctx, pm(7), p.ID)
	if err != nil {
		t.Fatalf("详情失败: %v", err)
	}
	if len(d.ChangeIDs) != 1 || d.ChangeIDs[0] != 202 {
		t.Fatalf("详情 change_ids 错误: %v", d.ChangeIDs)
	}
}

// ---------------- 建议聚合 ----------------

func TestAggregateSuggestions(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// 无提示
	got, err := e.svc.AggregateSuggestions(ctx, 5, "db", 30)
	if err != nil {
		t.Fatalf("建议聚合失败: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("无提示时不应返回项，实际 %v", got)
	}
	// 同 CI ≥3
	e.incidents.ciCount[5] = 4
	// 同关键词 ≥2
	e.incidents.kwCount["db"] = 2
	got, err = e.svc.AggregateSuggestions(ctx, 5, "db", 0) // days<=0 默认 30
	if err != nil {
		t.Fatalf("建议聚合失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("应返回 2 条建议，实际 %d (%v)", len(got), got)
	}
}

// ---------------- 查询 / 编辑 / 删除 ----------------

func TestList_Update_Delete(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	p := e.createManual(t, pm(7), "列表目标")

	items, total, err := e.svc.List(ctx, pm(7), ListQuery{Status: StatusNew, Keyword: "列表"})
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("列表结果错误: total=%d len=%d", total, len(items))
	}

	// 编辑：空标题 -> 400
	empty := "  "
	if _, err := e.svc.Update(ctx, pm(7), p.ID, UpdateRequest{Title: &empty}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("空标题编辑应 400")
	}
	title := "列表目标v2"
	rc := "内存泄漏"
	wa := "临时扩容"
	got, err := e.svc.Update(ctx, pm(7), p.ID, UpdateRequest{Title: &title, RootCause: &rc, Workaround: &wa})
	if err != nil {
		t.Fatalf("编辑失败: %v", err)
	}
	if got.Title != title || got.RootCause != rc {
		t.Fatalf("编辑字段未生效: %+v", got)
	}
	// Get
	if _, err := e.svc.Get(ctx, pm(7), p.ID); err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	// 不存在 -> 404
	if _, err := e.svc.Get(ctx, pm(7), 9999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404")
	}
	// Delete
	if err := e.svc.Delete(ctx, admin(), p.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if _, err := e.svc.Get(ctx, admin(), p.ID); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("删除后应 404")
	}
}

// ---------------- 依赖故障 / 未初始化 ----------------

func TestDependencyFailures(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// 仓储未初始化
	svc := NewService(Deps{Now: func() time.Time { return time.Now() }})
	if _, err := svc.Create(ctx, pm(7), CreateRequest{Title: "x"}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("无仓储应 500")
	}
	if _, _, err := svc.List(ctx, pm(7), ListQuery{}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("无仓储列表应 500")
	}
	if _, err := svc.Get(ctx, pm(7), 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("无仓储 Get 应 500")
	}
	if err := svc.Delete(ctx, admin(), 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("无仓储 Delete 应 500")
	}

	// 事件读取故障：aggregate 时 GetIncidentStatus 报错 -> 500
	e.incidents.failRead = true
	if _, err := e.svc.Create(ctx, pm(7), CreateRequest{Title: "x", Source: SourceAggregate, IncidentIDs: []uint64{1}}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("事件读取故障应 500")
	}
	e.incidents.failRead = false

	// 事件读取组件缺失
	svc2 := NewService(Deps{Repo: newFakeRepo(), Now: func() time.Time { return time.Now() }})
	if _, err := svc2.Create(ctx, pm(7), CreateRequest{Title: "x", Source: SourceAggregate, IncidentIDs: []uint64{1}}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺事件组件应 500")
	}
	if _, err := svc2.AggregateSuggestions(ctx, 1, "", 30); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺事件组件建议聚合应 500")
	}

	// 关联仓储缺失
	svc3 := NewService(Deps{Repo: newFakeRepo(), Now: func() time.Time { return time.Now() }})
	if err := svc3.AddChanges(ctx, pm(7), 1, []uint64{1}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺变更仓储应 500")
	}
}

// TestIllegalTransition409CarriesStatePair 校验问题非法流转 409 消息统一含「当前状态 -> 目标状态」（docs/API.md）。
func TestIllegalTransition409CarriesStatePair(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	p := e.createManual(t, pm(7), "非法流转") // new
	// new 直接 resolve（跳过 triage/investigate）→ 409，消息含 current=new -> target=resolved。
	_, err := e.svc.Transition(ctx, pm(7), p.ID, ActionResolve, TransitionRequest{Action: ActionResolve, NoChangeReason: "无需变更"})
	if appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("非法流转应 409，实际 %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "current="+StatusNew) || !strings.Contains(msg, "target="+StatusResolved) {
		t.Fatalf("非法流转消息应含 current=new -> target=resolved，实际 %q", msg)
	}
}
