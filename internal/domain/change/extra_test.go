package change

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
)

func TestRepoHelpersChange(t *testing.T) {
	cases := map[string]string{
		"risk_level": "risk_level", "status": "status", "id": "id",
		"created_at": "created_at", "bogus": "created_at", "": "created_at",
	}
	for in, want := range cases {
		if got := changeSortColumn(in); got != want {
			t.Fatalf("changeSortColumn(%q)=%q want %q", in, got, want)
		}
	}
	if orderDir("asc") != "ASC" || orderDir("") != "DESC" {
		t.Fatalf("orderDir 错误")
	}
	if !errors.Is(mapNotFound(gorm.ErrRecordNotFound), ErrNotFound) {
		t.Fatalf("mapNotFound 错误")
	}
	other := errors.New("x")
	if !errors.Is(mapNotFound(other), other) {
		t.Fatalf("mapNotFound 应透传")
	}
}

func TestTableNamesChange(t *testing.T) {
	if (Change{}).TableName() != "changes" || (ChangeApproval{}).TableName() != "change_approvals" {
		t.Fatalf("表名不符")
	}
}

func TestDetailAndApprovals(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	e.toPendingApproval(t, ch.ID)
	_, _ = e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: DecisionApprove})

	d, err := e.svc.Detail(ctx, cm(7), ch.ID)
	if err != nil || len(d.Approvals) != 1 {
		t.Fatalf("详情审批记录不符: %v len=%d", err, len(d.Approvals))
	}
	list, err := e.svc.ListApprovals(ctx, ch.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("ListApprovals 不符: %v", err)
	}
	if _, err := e.svc.ListApprovals(ctx, 999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404")
	}
}

func TestCancelDraftAndAssessment(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	// draft 取消
	ch := e.create(t, TypeNormal, RiskHigh)
	got, err := e.svc.Transition(ctx, cm(7), ch.ID, ActionCancel, TransitionRequest{Action: ActionCancel})
	if err != nil || got.Status != StatusCancelled {
		t.Fatalf("draft 取消失败: %v", err)
	}
	// assessment 取消
	ch2 := e.create(t, TypeNormal, RiskHigh)
	_, _ = e.svc.Transition(ctx, cm(7), ch2.ID, ActionSubmitAssessment, TransitionRequest{Action: ActionSubmitAssessment})
	got, err = e.svc.Transition(ctx, cm(7), ch2.ID, ActionCancel, TransitionRequest{Action: ActionCancel})
	if err != nil || got.Status != StatusCancelled {
		t.Fatalf("assessment 取消失败: %v", err)
	}
	// 终态再流转 → 409
	if _, err := e.svc.Transition(ctx, cm(7), ch2.ID, ActionCancel, TransitionRequest{Action: ActionCancel}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("终态流转应 409")
	}
}

func TestRejectViaTransitionRequiresComment(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	e.toPendingApproval(t, ch.ID)
	// 直接 transition reject 无 comment → 422
	if _, err := e.svc.Transition(ctx, cm(7), ch.ID, ActionReject, TransitionRequest{Action: ActionReject}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("驳回无意见应 422")
	}
	if _, err := e.svc.Transition(ctx, cm(7), ch.ID, ActionReject, TransitionRequest{Action: ActionReject, Comment: "不行"}); err != nil {
		t.Fatalf("驳回应成功: %v", err)
	}
}

func TestEmergencyOutOfWindowConfirm(t *testing.T) {
	e := newTestEnvCfg(t, true)
	ctx := context.Background()
	ch := e.create(t, TypeEmergency, RiskHigh)
	e.toPendingApproval(t, ch.ID)
	_, _ = e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{Decision: DecisionApprove})

	start := e.clock.Add(2 * time.Hour)
	end := e.clock.Add(4 * time.Hour)
	_, _ = e.svc.Transition(ctx, cm(7), ch.ID, ActionSchedule, TransitionRequest{Action: ActionSchedule, WindowStart: &start, WindowEnd: &end})
	// 窗口外紧急变更未确认 → 422
	if _, err := e.svc.Transition(ctx, resolv(3), ch.ID, ActionStartImplement, TransitionRequest{Action: ActionStartImplement}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("紧急窗口外未确认应 422")
	}
	// 确认后 → 成功
	got, err := e.svc.Transition(ctx, resolv(3), ch.ID, ActionStartImplement, TransitionRequest{Action: ActionStartImplement, ConfirmOutOfWindow: true})
	if err != nil || got.Status != StatusImplementing {
		t.Fatalf("紧急确认后实施应成功: %v", err)
	}
}

func TestMissingReposChange(t *testing.T) {
	svc := NewService(Deps{})
	ctx := context.Background()
	if _, err := svc.Get(ctx, cm(7), 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
	if _, _, err := svc.List(ctx, cm(7), ListQuery{}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
	if _, err := svc.ListApprovals(ctx, 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
	if _, err := svc.RecordApproval(ctx, cm(7), 1, ApprovalRequest{Decision: DecisionApprove}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
	if err := svc.Delete(ctx, cm(7), 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
}

func TestDelete_NotFoundAndUpdateMissing(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	if err := e.svc.Delete(ctx, admin(), 999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("删除不存在应 404")
	}
	if _, err := e.svc.Update(ctx, cm(7), 999, UpdateRequest{}); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("更新不存在应 404")
	}
}

func TestUpdateWindowAndPlan(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh)
	ws := e.clock.Add(time.Hour)
	we := e.clock.Add(2 * time.Hour)
	plan, rb, impact := "p", "r", "影响面"
	got, err := e.svc.Update(ctx, cm(7), ch.ID, UpdateRequest{Plan: &plan, RollbackPlan: &rb, ImpactAnalysis: &impact, WindowStart: &ws, WindowEnd: &we})
	if err != nil || got.Plan != plan || got.WindowEnd == nil {
		t.Fatalf("编辑字段不符: %v", err)
	}
	// 空标题 400
	empty := "  "
	if _, err := e.svc.Update(ctx, cm(7), ch.ID, UpdateRequest{Title: &empty}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("空标题应 400")
	}
	// 非法风险 400
	bad := "x"
	if _, err := e.svc.Update(ctx, cm(7), ch.ID, UpdateRequest{RiskLevel: &bad}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("非法风险应 400")
	}
}

// TestClosedChangeCount 覆盖消费者侧 ClosedChangeCount。
func TestClosedChangeCount(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// 空 ids -> 0
	if n, err := e.svc.ClosedChangeCount(ctx, nil); err != nil || n != 0 {
		t.Fatalf("空 ids 应返回 0，实际 %d err=%v", n, err)
	}
	// 无仓储 -> 500
	svc := NewService(Deps{Now: func() time.Time { return time.Now() }})
	if _, err := svc.ClosedChangeCount(ctx, []uint64{1}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("无仓储应 500")
	}
	// 造两个变更，其中一个置为 closed
	ch1 := e.create(t, TypeNormal, RiskLow)
	ch2 := e.create(t, TypeNormal, RiskLow)
	c2, err := e.repo.Get(ctx, ch2.ID)
	if err != nil {
		t.Fatalf("取变更失败: %v", err)
	}
	c2.Status = StatusClosed
	if err := e.repo.Update(ctx, c2); err != nil {
		t.Fatalf("写变更失败: %v", err)
	}
	n, err := e.svc.ClosedChangeCount(ctx, []uint64{ch1.ID, ch2.ID})
	if err != nil || n != 1 {
		t.Fatalf("应统计 1 个 closed，实际 %d err=%v", n, err)
	}
}

// TestIllegalTransition409CarriesStatePair 校验变更非法流转 409 消息统一含「当前状态 -> 目标状态」（docs/API.md）。
func TestIllegalTransition409CarriesStatePair(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	ch := e.create(t, TypeNormal, RiskHigh) // draft
	// draft 直接 start_implement（跳过审批）→ 409，消息含 current=draft -> target=implementing。
	_, err := e.svc.Transition(ctx, resolv(3), ch.ID, ActionStartImplement, TransitionRequest{Action: ActionStartImplement})
	if appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("非法流转应 409，实际 %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "current="+StatusDraft) || !strings.Contains(msg, "target="+StatusImplementing) {
		t.Fatalf("非法流转消息应含 current=draft -> target=implementing，实际 %q", msg)
	}
}
