package incident

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
	repo        *fakeRepo
	escalations *fakeEscalations
	users       *fakeUsers
	slas        *fakeSLAs
	auditor     *fakeAuditor
	tickets     *fakeTicketCreator
	svc         *Service
	clock       *time.Time
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	base := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	e := &testEnv{
		repo:        newFakeRepo(),
		escalations: newFakeEscalations(),
		users:       newFakeUsers(),
		slas:        newFakeSLAs(),
		auditor:     newFakeAuditor(),
		tickets:     newFakeTicketCreator(),
	}
	cur := base
	e.clock = &cur
	e.svc = NewService(Deps{
		Repo: e.repo, Escalations: e.escalations, Users: e.users, SLAs: e.slas,
		Auditor: e.auditor, Tickets: e.tickets, Now: func() time.Time { return *e.clock },
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
func reqr(id uint64) Actor  { return Actor{UserID: id, Role: role.Requestor} }

func (e *testEnv) report(t *testing.T, actor Actor, impact, urgency string) *Incident {
	t.Helper()
	it, err := e.svc.Report(context.Background(), actor, ReportRequest{Title: "服务不可用", Impact: impact, Urgency: urgency})
	if err != nil {
		t.Fatalf("上报失败: %v", err)
	}
	return it
}

// toInProgress 推进到 in_progress。
func (e *testEnv) toInProgress(t *testing.T, id uint64) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.svc.Transition(ctx, agent(7), id, ActionTriage, TransitionRequest{Action: ActionTriage}); err != nil {
		t.Fatalf("triage 失败: %v", err)
	}
	aid := uint64(7)
	if _, err := e.svc.Transition(ctx, agent(7), id, ActionConfirm, TransitionRequest{Action: ActionConfirm, AssigneeID: &aid}); err != nil {
		t.Fatalf("confirm 失败: %v", err)
	}
}

// ---------------- 上报 ----------------

func TestReport_MatrixAndSLA(t *testing.T) {
	e := newTestEnv(t)
	it := e.report(t, reqr(20), ImpactHigh, UrgencyHigh)
	if it.Priority != PriorityP1 {
		t.Fatalf("高/高 应 P1，实际 %s", it.Priority)
	}
	if !strings.HasPrefix(it.Code, "INC-20260916-") {
		t.Fatalf("编号格式不符: %s", it.Code)
	}
	if it.Status != StatusReported || it.SLAStatus == "" {
		t.Fatalf("初始状态/SLA 不符: %+v", it)
	}
}

func TestReport_Validation(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	if _, err := e.svc.Report(ctx, reqr(20), ReportRequest{Title: "", Impact: ImpactHigh, Urgency: UrgencyHigh}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("空标题应 400")
	}
	if _, err := e.svc.Report(ctx, reqr(20), ReportRequest{Title: "x", Impact: "bad", Urgency: UrgencyHigh}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法影响度应 400")
	}
}

// ---------------- 优先级覆盖 ----------------

func TestOverridePriority(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactLow, UrgencyLow) // P4
	got, err := e.svc.OverridePriority(ctx, agent(7), it.ID, PriorityP1)
	if err != nil {
		t.Fatalf("覆盖失败: %v", err)
	}
	if got.Priority != PriorityP1 || !got.PriorityOverridden {
		t.Fatalf("覆盖字段不符: %+v", got)
	}
	if _, err := e.svc.OverridePriority(ctx, agent(7), it.ID, "P9"); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法优先级应 400")
	}
}

// ---------------- 升级 ----------------

func TestEscalate_LevelPlusOne(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactHigh, UrgencyHigh)
	e.toInProgress(t, it.ID)

	got, err := e.svc.Escalate(ctx, agent(7), it.ID, EscalateRequest{Type: EscFunc, Reason: "需要专家"})
	if err != nil {
		t.Fatalf("升级失败: %v", err)
	}
	if got.EscalationLevel != 1 || got.Status != StatusEscalated {
		t.Fatalf("升级后不符: level=%d status=%s", got.EscalationLevel, got.Status)
	}
	if e.escalations.count() != 1 {
		t.Fatalf("应写 1 条升级历史，实际 %d", e.escalations.count())
	}
}

func TestEscalate_SkipLevelConflict(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactHigh, UrgencyHigh)
	e.toInProgress(t, it.ID)

	// 越级：直接指定 level=2 → 409
	lv := 2
	if _, err := e.svc.Escalate(ctx, agent(7), it.ID, EscalateRequest{Type: EscFunc, Reason: "r", Level: &lv}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("越级升级应 409，实际 %v", err)
	}
	// 无原因 → 422
	if _, err := e.svc.Escalate(ctx, agent(7), it.ID, EscalateRequest{Type: EscFunc}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("无原因升级应 422")
	}
}

func TestEscalate_MaxLevel(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactHigh, UrgencyHigh)
	e.toInProgress(t, it.ID)
	for i := 0; i < MaxEscalationLevel; i++ {
		if _, err := e.svc.Escalate(ctx, agent(7), it.ID, EscalateRequest{Type: EscFunc, Reason: "r"}); err != nil {
			t.Fatalf("第 %d 次升级失败: %v", i+1, err)
		}
		if _, err := e.svc.Transition(ctx, agent(7), it.ID, ActionTakeOver, TransitionRequest{Action: ActionTakeOver}); err != nil {
			t.Fatalf("take_over 失败: %v", err)
		}
	}
	// 已达上限 → 409
	if _, err := e.svc.Escalate(ctx, agent(7), it.ID, EscalateRequest{Type: EscFunc, Reason: "r"}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("超过最大级别应 409")
	}
}

// ---------------- 转工单 ----------------

func TestConvertToTicket_AndDuplicate(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactHigh, UrgencyHigh)

	// reported 状态不可转单 → 409
	if _, _, err := e.svc.ConvertToTicket(ctx, agent(7), it.ID, ""); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("reported 转单应 409")
	}
	e.toInProgress(t, it.ID)
	tkID, code, err := e.svc.ConvertToTicket(ctx, agent(7), it.ID, "工单标题")
	if err != nil || tkID == 0 || code == "" {
		t.Fatalf("转单失败: %v", err)
	}
	// 重复转单 → 409
	if _, _, err := e.svc.ConvertToTicket(ctx, agent(7), it.ID, ""); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("重复转单应 409")
	}
}

func TestConvertToTicket_CreatorError(t *testing.T) {
	e := newTestEnv(t)
	e.tickets.failWith = errors.New("boom")
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactHigh, UrgencyHigh)
	e.toInProgress(t, it.ID)
	if _, _, err := e.svc.ConvertToTicket(ctx, agent(7), it.ID, ""); err == nil {
		t.Fatalf("创建失败应返回错误")
	}
}

// ---------------- 解决 / 关闭 / 回退 ----------------

func TestResolve_RequiresSolutionAndP1Reason(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactHigh, UrgencyHigh) // P1
	e.toInProgress(t, it.ID)

	// 无 solution → 422
	if _, err := e.svc.Transition(ctx, agent(7), it.ID, ActionResolve, TransitionRequest{Action: ActionResolve}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("无 solution 应 422")
	}
	// P1 缺恢复说明 → 422
	if _, err := e.svc.Transition(ctx, agent(7), it.ID, ActionResolve, TransitionRequest{Action: ActionResolve, Solution: "已恢复"}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("P1 缺恢复说明应 422")
	}
	// 完整 → 成功
	got, err := e.svc.Transition(ctx, agent(7), it.ID, ActionResolve, TransitionRequest{Action: ActionResolve, Solution: "已恢复", Reason: "影响 2 台，已回滚"})
	if err != nil || got.Status != StatusResolved {
		t.Fatalf("解决失败: %v", err)
	}
}

func TestClose_P1RequiresReview(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactHigh, UrgencyHigh)
	e.toInProgress(t, it.ID)
	_, _ = e.svc.Transition(ctx, agent(7), it.ID, ActionResolve, TransitionRequest{Action: ActionResolve, Solution: "s", Reason: "r"})

	if _, err := e.svc.Transition(ctx, agent(7), it.ID, ActionClose, TransitionRequest{Action: ActionClose}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("P1 无复盘应 422")
	}
	got, err := e.svc.Transition(ctx, agent(7), it.ID, ActionClose, TransitionRequest{Action: ActionClose, ReviewConclusion: "根因已消除"})
	if err != nil || got.Status != StatusClosed || got.ClosedAt == nil {
		t.Fatalf("关闭失败: %v", err)
	}
}

func TestRevert_Window(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactLow, UrgencyLow) // P4（关闭无需复盘）
	e.toInProgress(t, it.ID)
	_, _ = e.svc.Transition(ctx, agent(7), it.ID, ActionResolve, TransitionRequest{Action: ActionResolve, Solution: "s"})

	got, err := e.svc.Transition(ctx, agent(7), it.ID, ActionRevert, TransitionRequest{Action: ActionRevert})
	if err != nil || got.Status != StatusInProgress {
		t.Fatalf("24h 内回退应成功: %v", err)
	}
	// 再解决后超 24h 回退 → 409
	_, _ = e.svc.Transition(ctx, agent(7), it.ID, ActionResolve, TransitionRequest{Action: ActionResolve, Solution: "s"})
	e.advance(25 * time.Hour)
	if _, err := e.svc.Transition(ctx, agent(7), it.ID, ActionRevert, TransitionRequest{Action: ActionRevert}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("超 24h 回退应 409")
	}
}

func TestCancel_OnlyReporter(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactLow, UrgencyLow)
	if _, err := e.svc.Transition(ctx, reqr(21), it.ID, ActionCancel, TransitionRequest{Action: ActionCancel}); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("非本人撤销应 403")
	}
	if _, err := e.svc.Transition(ctx, reqr(20), it.ID, ActionCancel, TransitionRequest{Action: ActionCancel}); err != nil {
		t.Fatalf("本人撤销应成功: %v", err)
	}
}

// ---------------- 关联工单 / CI ----------------

func TestLinkTicketAndCIs(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactLow, UrgencyLow)

	if _, err := e.svc.LinkTicket(ctx, agent(7), it.ID, 0); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("ticket_id=0 应 400")
	}
	got, err := e.svc.LinkTicket(ctx, agent(7), it.ID, 88)
	if err != nil || got.TicketID == nil || *got.TicketID != 88 {
		t.Fatalf("关联工单失败: %v", err)
	}
	if err := e.svc.AddCIs(ctx, agent(7), it.ID, []uint64{1, 2}); err != nil {
		t.Fatalf("关联 CI 失败: %v", err)
	}
	if err := e.svc.AddCIs(ctx, agent(7), it.ID, nil); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("空 ci_ids 应 400")
	}
	d, _ := e.svc.Detail(ctx, agent(7), it.ID)
	if len(d.CIs) != 2 || d.Escalations == nil {
		t.Fatalf("详情 CI 不符: %+v", d.CIs)
	}
	if err := e.svc.RemoveCI(ctx, agent(7), it.ID, 1); err != nil {
		t.Fatalf("解除 CI 失败: %v", err)
	}
}

// ---------------- 编辑 / 删除 / 列表 / 看板 ----------------

func TestUpdate_RecalcPriority(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactLow, UrgencyLow) // P4
	hi := ImpactHigh
	urg := UrgencyHigh
	got, err := e.svc.Update(ctx, agent(7), it.ID, UpdateRequest{Impact: &hi, Urgency: &urg})
	if err != nil || got.Priority != PriorityP1 {
		t.Fatalf("编辑后应重算为 P1: %v", err)
	}
	bad := "x"
	if _, err := e.svc.Update(ctx, agent(7), it.ID, UpdateRequest{Impact: &bad}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法影响度应 400")
	}
	if _, err := e.svc.Update(ctx, agent(7), 999, UpdateRequest{}); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404")
	}
}

func TestDeleteAndListAndStats(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactHigh, UrgencyHigh)
	_, _ = e.svc.Report(ctx, reqr(20), ReportRequest{Title: "低影响", Impact: ImpactLow, Urgency: UrgencyLow})

	items, total, err := e.svc.List(ctx, agent(7), ListQuery{Offset: 0, Limit: 10})
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("列表不符: total=%d err=%v", total, err)
	}
	byStatus, byPriority, err := e.svc.Stats(ctx)
	if err != nil || byStatus[StatusReported] != 2 || byPriority[PriorityP1] != 1 {
		t.Fatalf("统计不符: %v %v %v", byStatus, byPriority, err)
	}
	if err := e.svc.Delete(ctx, admin(), it.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if err := e.svc.Delete(ctx, admin(), it.ID); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("重复删除应 404")
	}
}

// ---------------- 消费者侧实现 ----------------

func TestConsumerImpls(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactLow, UrgencyLow)

	status, pid, has, found, err := e.svc.GetIncidentStatus(ctx, it.ID)
	if err != nil || !found || has || pid != 0 || status != StatusReported {
		t.Fatalf("GetIncidentStatus 不符: %v %v %v %v %v", status, pid, has, found, err)
	}
	// 不存在 → found=false，无错误
	_, _, _, found, err = e.svc.GetIncidentStatus(ctx, 999)
	if err != nil || found {
		t.Fatalf("不存在事件应 found=false")
	}
	// 挂载到问题
	if err := e.svc.AttachIncidentsToProblem(ctx, []uint64{it.ID}, 55); err != nil {
		t.Fatalf("挂载失败: %v", err)
	}
	_, pid, has, _, _ = e.svc.GetIncidentStatus(ctx, it.ID)
	if !has || pid != 55 {
		t.Fatalf("挂载后应返回 problem_id=55: %d %v", pid, has)
	}
	// 建议聚合计数
	_ = e.svc.AddCIs(ctx, agent(7), it.ID, []uint64{9})
	if n, err := e.svc.CountIncidentsByCI(ctx, 9, e.clock.Add(-time.Hour)); err != nil || n != 1 {
		t.Fatalf("CountIncidentsByCI 不符: %d %v", n, err)
	}
	if n, err := e.svc.CountIncidentsByKeyword(ctx, "服务", e.clock.Add(-time.Hour)); err != nil || n != 1 {
		t.Fatalf("CountIncidentsByKeyword 不符: %d %v", n, err)
	}
}

func TestDetail_NotFound(t *testing.T) {
	e := newTestEnv(t)
	if _, err := e.svc.Detail(context.Background(), agent(7), 999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404")
	}
}

func TestServiceDefaults(t *testing.T) {
	svc := NewService(Deps{})
	if svc.now == nil || svc.gen == nil || svc.log == nil {
		t.Fatalf("默认依赖未回填")
	}
	if _, err := svc.Report(context.Background(), agent(7), ReportRequest{Title: "x", Impact: ImpactHigh, Urgency: UrgencyHigh}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
}

// TestIllegalTransition409CarriesStatePair 校验事件非法流转 409 消息统一含「当前状态 -> 目标状态」（docs/API.md）。
func TestIllegalTransition409CarriesStatePair(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactHigh, UrgencyHigh) // reported
	// reported 直接 resolve（跳过 triage/confirm）→ 409，消息含 current=reported -> target=resolved。
	_, err := e.svc.Transition(ctx, agent(7), it.ID, ActionResolve, TransitionRequest{Action: ActionResolve})
	if appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("非法流转应 409，实际 %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "current="+StatusReported) || !strings.Contains(msg, "target="+StatusResolved) {
		t.Fatalf("非法流转消息应含 current=reported -> target=resolved，实际 %q", msg)
	}
}
