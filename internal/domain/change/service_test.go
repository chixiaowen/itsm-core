package change

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
	repo      *fakeRepo
	approvals *fakeApprovals
	users     *fakeUsers
	auditor   *fakeAuditor
	svc       *Service
	clock     *time.Time
}

func newTestEnv(t *testing.T) *testEnv { return newTestEnvCfg(t, false) }

func newTestEnvCfg(t *testing.T, enforce bool) *testEnv {
	t.Helper()
	base := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	e := &testEnv{
		repo: newFakeRepo(), approvals: newFakeApprovals(), users: newFakeUsers(), auditor: newFakeAuditor(),
	}
	cur := base
	e.clock = &cur
	e.svc = NewService(Deps{
		Repo: e.repo, Approvals: e.approvals, Users: e.users, Auditor: e.auditor,
		EnforceWindow: enforce, Now: func() time.Time { return *e.clock },
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

func reqr(id uint64) Actor   { return Actor{UserID: id, Role: role.Requestor} }
func cm(id uint64) Actor     { return Actor{UserID: id, Role: role.ChangeManager} }
func resolv(id uint64) Actor { return Actor{UserID: id, Role: role.Resolver} }
func admin() Actor           { return Actor{UserID: 9, Role: role.Admin} }

// creator 返回一名「真实变更创建者」：持有 perm.change.submit 的 agent。
//
// 现网中变更只能由持 perm.change.submit 的角色（agent/resolver/problem_manager/
// change_manager/cmdb_manager/admin）创建，role.Requestor 到不了「创建者」这一身份；
// 故测试统一用 creator 充当创建者，避免出现「能过状态机但现实中不可达」的假绿用例。
func creator(id uint64) Actor { return Actor{UserID: id, Role: role.Agent} }

func (e *testEnv) create(t *testing.T, typ, risk string) *Change {
	t.Helper()
	ch, err := e.svc.Create(context.Background(), creator(5), CreateRequest{Title: "变更", ChangeType: typ, RiskLevel: risk})
	if err != nil {
		t.Fatalf("创建变更失败: %v", err)
	}
	return ch
}

// ---------------- EvaluateApprovals 表驱动 ----------------

func TestEvaluateApprovals(t *testing.T) {
	// a 构造一组「不同审批人」的投票（ApproverID 依次 1..n），模拟正常会签数据。
	a := func(d ...string) []ChangeApproval {
		var out []ChangeApproval
		for i, x := range d {
			out = append(out, ChangeApproval{ApproverID: uint64(i + 1), Decision: x})
		}
		return out
	}
	cases := []struct {
		name       string
		changeType string
		approvals  []ChangeApproval
		want       string
	}{
		{"空", TypeNormal, a(), ""},
		{"普通1通过待定", TypeNormal, a(DecisionApprove), ""},
		{"普通2通过", TypeNormal, a(DecisionApprove, DecisionApprove), DecisionApprove},
		{"普通1通过1拒绝", TypeNormal, a(DecisionApprove, DecisionReject), DecisionReject},
		{"普通1拒绝", TypeNormal, a(DecisionReject), DecisionReject},
		{"标准2通过", TypeStandard, a(DecisionApprove, DecisionApprove), DecisionApprove},
		{"紧急1通过", TypeEmergency, a(DecisionApprove), DecisionApprove},
		{"紧急拒绝优先", TypeEmergency, a(DecisionApprove, DecisionReject), DecisionReject},
		{"紧急0通过", TypeEmergency, a(""), ""},
		// 脏数据/绕过唯一索引的历史数据：同一审批人两条 approved 只算 1 票，不得通过。
		{"同一人重复通过仅算1票", TypeNormal, []ChangeApproval{
			{ApproverID: 7, Decision: DecisionApprove},
			{ApproverID: 7, Decision: DecisionApprove},
		}, ""},
		// 紧急变更下同一人也只算 1 票（≥1 满足），验证去重口径一致。
		{"紧急同一人重复通过仍视为已满足", TypeEmergency, []ChangeApproval{
			{ApproverID: 7, Decision: DecisionApprove},
			{ApproverID: 7, Decision: DecisionApprove},
		}, DecisionApprove},
	}
	for _, tc := range cases {
		if got := EvaluateApprovals(tc.changeType, tc.approvals); got != tc.want {
			t.Fatalf("%s: 期望 %q 实际 %q", tc.name, tc.want, got)
		}
	}
}

// ---------------- 创建 / 校验 ----------------

func TestCreate_Validation(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	if _, err := e.svc.Create(ctx, creator(5), CreateRequest{Title: "", ChangeType: TypeNormal, RiskLevel: RiskLow}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("空标题应 400")
	}
	if _, err := e.svc.Create(ctx, creator(5), CreateRequest{Title: "x", ChangeType: "bad", RiskLevel: RiskLow}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法类型应 400")
	}
	if _, err := e.svc.Create(ctx, creator(5), CreateRequest{Title: "x", ChangeType: TypeNormal, RiskLevel: "bad"}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法风险应 400")
	}
	ch := e.create(t, TypeNormal, RiskLow)
	if !strings.HasPrefix(ch.Code, "CHG-20260916-") || ch.Status != StatusDraft {
		t.Fatalf("编号/状态不符: %+v", ch)
	}
}

// ---------------- 标准变更预授权 ----------------

func TestStandardPreAuthorize(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// 非标准变更走 pre_authorize → 409
	normal := e.create(t, TypeNormal, RiskLow)
	if _, err := e.svc.Transition(ctx, creator(5), normal.ID, ActionPreAuthorize, TransitionRequest{Action: ActionPreAuthorize}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("非标准预授权应 409，实际 %v", err)
	}

	// 标准变更缺方案 → 422
	std := e.create(t, TypeStandard, RiskLow)
	if _, err := e.svc.Transition(ctx, creator(5), std.ID, ActionPreAuthorize, TransitionRequest{Action: ActionPreAuthorize}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("标准变更缺方案应 422")
	}
	// 补方案后 → approved
	plan := "步骤"
	rb := "回滚"
	if _, err := e.svc.Update(ctx, creator(5), std.ID, UpdateRequest{Plan: &plan, RollbackPlan: &rb}); err != nil {
		t.Fatalf("编辑失败: %v", err)
	}
	got, err := e.svc.Transition(ctx, creator(5), std.ID, ActionPreAuthorize, TransitionRequest{Action: ActionPreAuthorize})
	if err != nil || got.Status != StatusApproved || !got.PreAuthorized {
		t.Fatalf("标准预授权失败: %v", err)
	}
}

// ---------------- CAB 会签 ----------------

func (e *testEnv) toPendingApproval(t *testing.T, id uint64) {
	t.Helper()
	ctx := context.Background()
	// 提交风险评估仅限变更创建者角色（持有 perm.change.submit 的 agent/resolver/change_manager 或 admin）。
	if _, err := e.svc.Transition(ctx, cm(7), id, ActionSubmitAssessment, TransitionRequest{Action: ActionSubmitAssessment}); err != nil {
		t.Fatalf("submit_assessment 失败: %v", err)
	}
	plan, rb := "p", "r"
	if _, err := e.svc.Update(ctx, creator(5), id, UpdateRequest{Plan: &plan, RollbackPlan: &rb}); err != nil {
		t.Fatalf("补方案失败: %v", err)
	}
	if _, err := e.svc.Transition(ctx, creator(5), id, ActionSubmitApproval, TransitionRequest{Action: ActionSubmitApproval}); err != nil {
		t.Fatalf("submit_approval 失败: %v", err)
	}
}

func TestCAB_ConsensusNormal(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	e.toPendingApproval(t, ch.ID)

	// 第 1 票通过 → 仍 pending_approval
	got, err := e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: DecisionApprove})
	if err != nil || got.Status != StatusPendingApproval {
		t.Fatalf("1 票后应仍 pending: %v status=%s", err, got.Status)
	}
	// 第 2 票通过 → approved
	got, err = e.svc.RecordApproval(ctx, cm(8), ch.ID, ApprovalRequest{Decision: DecisionApprove})
	if err != nil || got.Status != StatusApproved {
		t.Fatalf("2 票后应 approved: %v", err)
	}
}

func TestCAB_Reject(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	e.toPendingApproval(t, ch.ID)

	got, err := e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: DecisionReject, Comment: "风险太高"})
	if err != nil || got.Status != StatusRejected {
		t.Fatalf("驳回应 rejected: %v", err)
	}
	// rejected → revise → draft
	got, err = e.svc.Transition(ctx, creator(5), ch.ID, ActionRevise, TransitionRequest{Action: ActionRevise})
	if err != nil || got.Status != StatusDraft {
		t.Fatalf("修订应回 draft: %v", err)
	}
}

func TestECAB_EmergencyOneApproval(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeEmergency, RiskHigh)
	e.toPendingApproval(t, ch.ID)
	got, err := e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: DecisionApprove})
	if err != nil || got.Status != StatusApproved {
		t.Fatalf("紧急变更 1 票应 approved: %v", err)
	}
}

func TestRecordApproval_Guards(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	// 非 pending_approval → 409
	if _, err := e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: DecisionApprove}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("非待审审批应 409")
	}
	e.toPendingApproval(t, ch.ID)
	if _, err := e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: "bad"}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法决定应 400")
	}
}

// TestRecordApproval_NoDoubleVote 同一审批人对同一变更只能投一票（四眼原则，防单人凑票绕过 CAB）。
func TestRecordApproval_NoDoubleVote(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	e.toPendingApproval(t, ch.ID)

	// 第 1 票通过 → 成功，仍 pending
	if _, err := e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: DecisionApprove}); err != nil {
		t.Fatalf("第 1 票应成功: %v", err)
	}
	// 同一人再次 approve → 409（不得凑票）
	if _, err := e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: DecisionApprove}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("重复投票应 409")
	}
	// 同一人先 approved 再 rejected（改票）→ 409
	if _, err := e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: DecisionReject, Comment: "改票"}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("改票应 409")
	}
	// 该审批人仅 1 条记录，且变更仍为 pending_approval（未通过）
	apvs, err := e.svc.ListApprovals(ctx, ch.ID)
	if err != nil {
		t.Fatalf("查询审批记录失败: %v", err)
	}
	if len(apvs) != 1 || apvs[0].ApproverID != 7 {
		t.Fatalf("应仅 1 条审批记录，实际 %d", len(apvs))
	}
	cur, err := e.svc.Get(ctx, cm(7), ch.ID)
	if err != nil || cur.Status != StatusPendingApproval {
		t.Fatalf("单人凑票不应通过，实际 status=%v err=%v", cur.Status, err)
	}
	// 另一名审批人投票 → 正常通过（保证修复没有把正常会签打死）
	got, err := e.svc.RecordApproval(ctx, cm(8), ch.ID, ApprovalRequest{Decision: DecisionApprove})
	if err != nil || got.Status != StatusApproved {
		t.Fatalf("第二位不同审批人投票应 approved: %v status=%v", err, got.Status)
	}
}

// TestCAB_ResubmitFreshRound 重提审批开启新一轮：旧审批记录清空，同一审批人可重新投票。
func TestCAB_ResubmitFreshRound(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	e.toPendingApproval(t, ch.ID)

	// 第 1 轮：7 通过、8 驳回 → rejected
	if _, err := e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: DecisionApprove}); err != nil {
		t.Fatalf("7 通过失败: %v", err)
	}
	if _, err := e.svc.RecordApproval(ctx, cm(8), ch.ID, ApprovalRequest{Decision: DecisionReject, Comment: "否"}); err != nil {
		t.Fatalf("8 驳回失败: %v", err)
	}
	// revise → draft，再重提审批
	if _, err := e.svc.Transition(ctx, creator(5), ch.ID, ActionRevise, TransitionRequest{Action: ActionRevise}); err != nil {
		t.Fatalf("revise 失败: %v", err)
	}
	e.toPendingApproval(t, ch.ID)

	apvs, err := e.svc.ListApprovals(ctx, ch.ID)
	if err != nil {
		t.Fatalf("查询审批记录失败: %v", err)
	}
	if len(apvs) != 0 {
		t.Fatalf("新一轮应清空旧审批记录，实际 %d 条", len(apvs))
	}
	// 同一审批人可重新投票
	if _, err := e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: DecisionApprove}); err != nil {
		t.Fatalf("新一轮 7 通过应成功: %v", err)
	}
}

// ---------------- 提交审批前置条件 ----------------

func TestSubmitApproval_RequiresPlan(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	_, _ = e.svc.Transition(ctx, cm(7), ch.ID, ActionSubmitAssessment, TransitionRequest{Action: ActionSubmitAssessment})
	// 无 plan → 422
	if _, err := e.svc.Transition(ctx, creator(5), ch.ID, ActionSubmitApproval, TransitionRequest{Action: ActionSubmitApproval}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("无 plan 提交审批应 422")
	}
}

// ---------------- 绕过审批 ----------------

func TestBypassApproval_Conflict(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	// draft → start_implement（无表项）→ 409
	if _, err := e.svc.Transition(ctx, resolv(3), ch.ID, ActionStartImplement, TransitionRequest{Action: ActionStartImplement}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("绕过审批实施应 409")
	}
}

// ---------------- 窗口校验 ----------------

func TestWindowOrderValidation(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	start := e.clock.Add(2 * time.Hour)
	end := e.clock.Add(1 * time.Hour) // end < start
	if _, err := e.svc.Update(ctx, creator(5), ch.ID, UpdateRequest{WindowStart: &start, WindowEnd: &end}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("窗口顺序非法应 422")
	}
}

func TestEnforceWindow_OutOfWindowConflict(t *testing.T) {
	e := newTestEnvCfg(t, true)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	e.toPendingApproval(t, ch.ID)
	_, _ = e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: DecisionApprove})
	_, _ = e.svc.RecordApproval(ctx, cm(8), ch.ID, ApprovalRequest{Decision: DecisionApprove})

	// 设置窗口在未来
	start := e.clock.Add(2 * time.Hour)
	end := e.clock.Add(4 * time.Hour)
	if _, err := e.svc.Transition(ctx, cm(7), ch.ID, ActionSchedule, TransitionRequest{Action: ActionSchedule, WindowStart: &start, WindowEnd: &end}); err != nil {
		t.Fatalf("排期失败: %v", err)
	}
	// now 不在窗口内 → 409
	if _, err := e.svc.Transition(ctx, resolv(3), ch.ID, ActionStartImplement, TransitionRequest{Action: ActionStartImplement}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("窗口外实施应 409")
	}
	// 进入窗口 → 成功
	e.advance(3 * time.Hour)
	if _, err := e.svc.Transition(ctx, resolv(3), ch.ID, ActionStartImplement, TransitionRequest{Action: ActionStartImplement}); err != nil {
		t.Fatalf("窗口内实施应成功: %v", err)
	}
}

// ---------------- 类型锁定 ----------------

func TestTypeLock(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	// assessment 后改类型 → 409
	_, _ = e.svc.Transition(ctx, cm(7), ch.ID, ActionSubmitAssessment, TransitionRequest{Action: ActionSubmitAssessment})
	std := TypeStandard
	if _, err := e.svc.Update(ctx, creator(5), ch.ID, UpdateRequest{ChangeType: &std}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("assessment 改类型应 409")
	}
	// draft 阶段可改
	ch2 := e.create(t, TypeNormal, RiskHigh)
	if _, err := e.svc.Update(ctx, creator(5), ch2.ID, UpdateRequest{ChangeType: &std}); err != nil {
		t.Fatalf("draft 改类型应成功: %v", err)
	}
	// 非 draft/assessment 编辑 → 409
	ch3 := e.create(t, TypeStandard, RiskLow)
	plan, rb := "p", "r"
	_, _ = e.svc.Update(ctx, creator(5), ch3.ID, UpdateRequest{Plan: &plan, RollbackPlan: &rb})
	_, _ = e.svc.Transition(ctx, creator(5), ch3.ID, ActionPreAuthorize, TransitionRequest{Action: ActionPreAuthorize})
	if _, err := e.svc.Update(ctx, creator(5), ch3.ID, UpdateRequest{Title: strptr("x")}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("approved 编辑应 409")
	}
}

// ---------------- 全生命周期 ----------------

func TestFullLifecycle(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	e.toPendingApproval(t, ch.ID)
	_, _ = e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: DecisionApprove})
	_, _ = e.svc.RecordApproval(ctx, cm(8), ch.ID, ApprovalRequest{Decision: DecisionApprove})

	start := e.clock.Add(-time.Hour)
	end := e.clock.Add(time.Hour)
	if _, err := e.svc.Transition(ctx, cm(7), ch.ID, ActionSchedule, TransitionRequest{Action: ActionSchedule, WindowStart: &start, WindowEnd: &end}); err != nil {
		t.Fatalf("排期失败: %v", err)
	}
	if _, err := e.svc.Transition(ctx, resolv(3), ch.ID, ActionStartImplement, TransitionRequest{Action: ActionStartImplement}); err != nil {
		t.Fatalf("开始实施失败: %v", err)
	}
	// complete 无 result → 422
	if _, err := e.svc.Transition(ctx, resolv(3), ch.ID, ActionComplete, TransitionRequest{Action: ActionComplete}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("无结论实施成功应 422")
	}
	if _, err := e.svc.Transition(ctx, resolv(3), ch.ID, ActionComplete, TransitionRequest{Action: ActionComplete, Result: "已上线"}); err != nil {
		t.Fatalf("实施成功失败: %v", err)
	}
	if _, err := e.svc.Transition(ctx, cm(7), ch.ID, ActionReview, TransitionRequest{Action: ActionReview}); err != nil {
		t.Fatalf("回顾失败: %v", err)
	}
	// close 无结论 → 422
	if _, err := e.svc.Transition(ctx, cm(7), ch.ID, ActionClose, TransitionRequest{Action: ActionClose}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("无结论关闭应 422")
	}
	got, err := e.svc.Transition(ctx, cm(7), ch.ID, ActionClose, TransitionRequest{Action: ActionClose, Conclusion: "验证通过"})
	if err != nil || got.Status != StatusClosed || got.ClosedAt == nil {
		t.Fatalf("关闭失败: %v", err)
	}
}

func TestRollbackAndResubmit(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	e.toPendingApproval(t, ch.ID)
	_, _ = e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: DecisionApprove})
	_, _ = e.svc.RecordApproval(ctx, cm(8), ch.ID, ApprovalRequest{Decision: DecisionApprove})
	start := e.clock.Add(-time.Hour)
	end := e.clock.Add(time.Hour)
	_, _ = e.svc.Transition(ctx, cm(7), ch.ID, ActionSchedule, TransitionRequest{Action: ActionSchedule, WindowStart: &start, WindowEnd: &end})
	_, _ = e.svc.Transition(ctx, resolv(3), ch.ID, ActionStartImplement, TransitionRequest{Action: ActionStartImplement})

	// 回滚无原因 → 422
	if _, err := e.svc.Transition(ctx, resolv(3), ch.ID, ActionRollback, TransitionRequest{Action: ActionRollback}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("回滚无原因应 422")
	}
	got, err := e.svc.Transition(ctx, resolv(3), ch.ID, ActionRollback, TransitionRequest{Action: ActionRollback, Reason: "服务异常"})
	if err != nil || got.Status != StatusRolledBack {
		t.Fatalf("回滚失败: %v", err)
	}
	// resubmit → draft
	got, err = e.svc.Transition(ctx, creator(5), ch.ID, ActionResubmit, TransitionRequest{Action: ActionResubmit})
	if err != nil || got.Status != StatusDraft {
		t.Fatalf("重新申请应回 draft: %v", err)
	}
}

func TestDeleteAndListAndGet(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	e.create(t, TypeEmergency, RiskHigh)

	items, total, err := e.svc.List(ctx, cm(7), ListQuery{Offset: 0, Limit: 10})
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("列表不符: total=%d err=%v", total, err)
	}
	if _, err := e.svc.Get(ctx, cm(7), 999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404")
	}
	if err := e.svc.Delete(ctx, admin(), ch.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if err := e.svc.Delete(ctx, admin(), ch.ID); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("重复删除应 404")
	}
}

func TestServiceDefaults(t *testing.T) {
	svc := NewService(Deps{})
	if svc.now == nil || svc.gen == nil || svc.log == nil {
		t.Fatalf("默认依赖未回填")
	}
	if _, err := svc.Create(context.Background(), creator(5), CreateRequest{Title: "x", ChangeType: TypeNormal, RiskLevel: RiskLow}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
}

// TestEarlyLifecycle_NonCreatorRejected 早期生命周期动作经 guardCreatorOrManager 资源级收窄：
// 非创建者的同角色 agent、以及 role.Requestor（change 域的死条目）推进
// pre_authorize / submit_approval / revise / resubmit 一律 403。
func TestEarlyLifecycle_NonCreatorRejected(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	plan, rb := "p", "r"

	// --- pre_authorize：创建者 100 的标准变更 ---
	std, _ := e.svc.Create(ctx, creator(100), CreateRequest{Title: "std", ChangeType: TypeStandard, RiskLevel: RiskLow})
	_, _ = e.svc.Update(ctx, creator(100), std.ID, UpdateRequest{Plan: &plan, RollbackPlan: &rb})
	if _, err := e.svc.Transition(ctx, creator(999), std.ID, ActionPreAuthorize, TransitionRequest{Action: ActionPreAuthorize}); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("非创建者 agent 推进 pre_authorize 应 403，实际 %v", err)
	}
	if _, err := e.svc.Transition(ctx, reqr(100), std.ID, ActionPreAuthorize, TransitionRequest{Action: ActionPreAuthorize}); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("role.Requestor（不可建单）推进 pre_authorize 应 403，实际 %v", err)
	}
	// 创建者本人可推进（正向对照）
	if _, err := e.svc.Transition(ctx, creator(100), std.ID, ActionPreAuthorize, TransitionRequest{Action: ActionPreAuthorize}); err != nil {
		t.Fatalf("创建者本人推进 pre_authorize 应成功: %v", err)
	}

	// --- submit_approval / revise：创建者 100 的普通变更 ---
	ch, _ := e.svc.Create(ctx, creator(100), CreateRequest{Title: "nrm", ChangeType: TypeNormal, RiskLevel: RiskLow})
	_, _ = e.svc.Transition(ctx, creator(100), ch.ID, ActionSubmitAssessment, TransitionRequest{Action: ActionSubmitAssessment})
	_, _ = e.svc.Update(ctx, creator(100), ch.ID, UpdateRequest{Plan: &plan, RollbackPlan: &rb})
	if _, err := e.svc.Transition(ctx, creator(999), ch.ID, ActionSubmitApproval, TransitionRequest{Action: ActionSubmitApproval}); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("非创建者 agent 推进 submit_approval 应 403，实际 %v", err)
	}
	// 创建者本人提交后由 CAB 驳回，再验证非创建者 revise → 403
	if _, err := e.svc.Transition(ctx, creator(100), ch.ID, ActionSubmitApproval, TransitionRequest{Action: ActionSubmitApproval}); err != nil {
		t.Fatalf("创建者本人 submit_approval 应成功: %v", err)
	}
	if _, err := e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: DecisionReject, Comment: "否"}); err != nil {
		t.Fatalf("CAB 驳回应成功: %v", err)
	}
	if _, err := e.svc.Transition(ctx, creator(999), ch.ID, ActionRevise, TransitionRequest{Action: ActionRevise}); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("非创建者 agent 推进 revise 应 403，实际 %v", err)
	}

	// --- resubmit：创建者 100 的变更走完 提交→会签→排期→实施→回滚 ---
	ch2, _ := e.svc.Create(ctx, creator(100), CreateRequest{Title: "rb", ChangeType: TypeNormal, RiskLevel: RiskLow})
	_, _ = e.svc.Transition(ctx, creator(100), ch2.ID, ActionSubmitAssessment, TransitionRequest{Action: ActionSubmitAssessment})
	_, _ = e.svc.Update(ctx, creator(100), ch2.ID, UpdateRequest{Plan: &plan, RollbackPlan: &rb})
	_, _ = e.svc.Transition(ctx, creator(100), ch2.ID, ActionSubmitApproval, TransitionRequest{Action: ActionSubmitApproval})
	_, _ = e.svc.RecordApproval(ctx, cm(7), ch2.ID, ApprovalRequest{Decision: DecisionApprove})
	_, _ = e.svc.RecordApproval(ctx, cm(8), ch2.ID, ApprovalRequest{Decision: DecisionApprove})
	start := e.clock.Add(-time.Hour)
	end := e.clock.Add(time.Hour)
	_, _ = e.svc.Transition(ctx, cm(7), ch2.ID, ActionSchedule, TransitionRequest{Action: ActionSchedule, WindowStart: &start, WindowEnd: &end})
	_, _ = e.svc.Transition(ctx, resolv(3), ch2.ID, ActionStartImplement, TransitionRequest{Action: ActionStartImplement})
	_, _ = e.svc.Transition(ctx, resolv(3), ch2.ID, ActionRollback, TransitionRequest{Action: ActionRollback, Reason: "实施异常"})
	if _, err := e.svc.Transition(ctx, creator(999), ch2.ID, ActionResubmit, TransitionRequest{Action: ActionResubmit}); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("非创建者 agent 推进 resubmit 应 403，实际 %v", err)
	}
}

func strptr(s string) *string { return &s }
