package incident

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

func TestRepoHelpersIncident(t *testing.T) {
	cases := map[string]string{
		"priority": "priority", "status": "status", "escalation_level": "escalation_level",
		"id": "id", "created_at": "created_at", "bogus": "created_at", "": "created_at",
	}
	for in, want := range cases {
		if got := incidentSortColumn(in); got != want {
			t.Fatalf("incidentSortColumn(%q)=%q want %q", in, got, want)
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

func TestTableNamesIncident(t *testing.T) {
	if (Incident{}).TableName() != "incidents" || (IncidentEscalation{}).TableName() != "incident_escalations" || (IncidentCI{}).TableName() != "incident_cis" {
		t.Fatalf("表名不符")
	}
}

func TestFalsePositiveAndPending(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// triage → false_positive 需 solution
	it := e.report(t, reqr(20), ImpactLow, UrgencyLow)
	_, _ = e.svc.Transition(ctx, agent(7), it.ID, ActionTriage, TransitionRequest{Action: ActionTriage})
	if _, err := e.svc.Transition(ctx, agent(7), it.ID, ActionFalsePos, TransitionRequest{Action: ActionFalsePos}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("误报无 solution 应 422")
	}
	got, err := e.svc.Transition(ctx, agent(7), it.ID, ActionFalsePos, TransitionRequest{Action: ActionFalsePos, Solution: "误报"})
	if err != nil || got.Status != StatusResolved {
		t.Fatalf("误报应 resolved: %v", err)
	}

	// in_progress → pending (需原因) → resume
	it2 := e.report(t, reqr(20), ImpactMedium, UrgencyMed)
	e.toInProgress(t, it2.ID)
	if _, err := e.svc.Transition(ctx, agent(7), it2.ID, ActionPending, TransitionRequest{Action: ActionPending}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("挂起无原因应 422")
	}
	if _, err := e.svc.Transition(ctx, agent(7), it2.ID, ActionPending, TransitionRequest{Action: ActionPending, Reason: "等待供应商"}); err != nil {
		t.Fatalf("挂起失败: %v", err)
	}
	got2, err := e.svc.Transition(ctx, agent(7), it2.ID, ActionResume, TransitionRequest{Action: ActionResume})
	if err != nil || got2.Status != StatusInProgress {
		t.Fatalf("恢复失败: %v", err)
	}
}

func TestCancelTriageRequiresReason(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactLow, UrgencyLow)
	_, _ = e.svc.Transition(ctx, agent(7), it.ID, ActionTriage, TransitionRequest{Action: ActionTriage})
	if _, err := e.svc.Transition(ctx, agent(7), it.ID, ActionCancel, TransitionRequest{Action: ActionCancel}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("triage 取消无原因应 422")
	}
	if _, err := e.svc.Transition(ctx, agent(7), it.ID, ActionCancel, TransitionRequest{Action: ActionCancel, Reason: "非事件"}); err != nil {
		t.Fatalf("triage 取消应成功: %v", err)
	}
}

func TestUpdate_DescriptionOnly(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactLow, UrgencyLow)
	desc := "补充描述"
	got, err := e.svc.Update(ctx, agent(7), it.ID, UpdateRequest{Description: &desc})
	if err != nil || got.Description != desc {
		t.Fatalf("编辑描述失败: %v", err)
	}
	empty := "  "
	if _, err := e.svc.Update(ctx, agent(7), it.ID, UpdateRequest{Title: &empty}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("空标题应 400")
	}
}

func TestList_Filters(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactHigh, UrgencyHigh)
	from := e.clock.Add(-time.Hour)
	to := e.clock.Add(time.Hour)
	items, total, err := e.svc.List(ctx, agent(7), ListQuery{
		Impact: ImpactHigh, Urgency: UrgencyHigh, Keyword: "服务",
		From: &from, To: &to, Priority: PriorityP1, Status: StatusReported, Offset: 0, Limit: 10,
	})
	if err != nil || total != 1 || items[0].ID != it.ID {
		t.Fatalf("过滤不符: total=%d err=%v", total, err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	e := newTestEnv(t)
	if err := e.svc.Delete(context.Background(), admin(), 999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("删除不存在应 404")
	}
}

func TestMissingReposIncident(t *testing.T) {
	svc := NewService(Deps{})
	ctx := context.Background()
	if _, err := svc.Get(ctx, agent(7), 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
	if _, _, err := svc.List(ctx, agent(7), ListQuery{}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
	if _, _, err := svc.Stats(ctx); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
	if err := svc.AddCIs(ctx, agent(7), 1, []uint64{1}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
	if err := svc.RemoveCI(ctx, agent(7), 1, 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
	if _, _, _, _, err := svc.GetIncidentStatus(ctx, 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
	if err := svc.AttachIncidentsToProblem(ctx, []uint64{1}, 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
	if _, err := svc.CountIncidentsByCI(ctx, 1, time.Now()); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
	if _, err := svc.CountIncidentsByKeyword(ctx, "k", time.Now()); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
	if err := svc.Delete(ctx, agent(7), 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("缺仓储应 500")
	}
}

func TestEscalateWithAssigneeAndTakeOver(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactHigh, UrgencyHigh)
	e.toInProgress(t, it.ID)
	to := uint64(7)
	got, err := e.svc.Escalate(ctx, agent(7), it.ID, EscalateRequest{Type: EscHier, Reason: "上报管理层", ToAssigneeID: &to})
	if err != nil {
		t.Fatalf("升级失败: %v", err)
	}
	if got.EscalationLevel != 1 || got.AssigneeID == nil || *got.AssigneeID != 7 {
		t.Fatalf("升级带指派人字段不符: %+v", got)
	}
	// take_over 带新指派人
	newA := uint64(8)
	got, err = e.svc.Transition(ctx, agent(7), it.ID, ActionTakeOver, TransitionRequest{Action: ActionTakeOver, AssigneeID: &newA})
	if err != nil || got.Status != StatusInProgress || got.AssigneeID == nil || *got.AssigneeID != 8 {
		t.Fatalf("take_over 带指派人失败: %v", err)
	}
}

func TestHandlerQueryAndRBACIncident(t *testing.T) {
	r, jwt, _ := newIncidentEngine(t)
	agentTok := incToken(t, jwt, 7, role.Agent)
	reqTok := incToken(t, jwt, 20, role.Requestor)
	// requestor 上报（有 perm.incident.report）
	incReq(r, http.MethodPost, "/api/v1/incidents", `{"title":"t","impact":"medium","urgency":"medium"}`, reqTok)
	// 带过滤参数列表
	url := "/api/v1/incidents?status=reported&priority=P3&impact=medium&urgency=medium&keyword=abc&from=2026-09-01T00:00:00Z&to=2026-09-30T00:00:00Z&sort_by=priority&order=asc&page=1&page_size=5"
	if w := incReq(r, http.MethodGet, url, "", agentTok); w.Code != http.StatusOK {
		t.Fatalf("带参列表应 200，实际 %d", w.Code)
	}
	// requestor 无升级权限 -> 403
	if w := incReq(r, http.MethodPost, "/api/v1/incidents/1/escalate", `{"type":"functional","reason":"r"}`, reqTok); w.Code != http.StatusForbidden {
		t.Fatalf("requestor 升级应 403，实际 %d", w.Code)
	}
	// 非法 ciId -> 400
	if w := incReq(r, http.MethodDelete, "/api/v1/incidents/1/cis/abc", "", agentTok); w.Code != http.StatusBadRequest {
		t.Fatalf("非法 ciId 应 400，实际 %d", w.Code)
	}
}

// TestListIncidentIDsByProblemPublic 覆盖消费者侧 ListIncidentIDsByProblem。
func TestListIncidentIDsByProblemPublic(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	it := e.report(t, reqr(20), ImpactHigh, UrgencyHigh)

	if err := e.svc.AttachIncidentsToProblem(ctx, []uint64{it.ID}, 88); err != nil {
		t.Fatalf("挂载问题失败: %v", err)
	}
	ids, err := e.svc.ListIncidentIDsByProblem(ctx, 88)
	if err != nil || len(ids) != 1 || ids[0] != it.ID {
		t.Fatalf("应返回 1 个事件，实际 %v err=%v", ids, err)
	}
	// 无仓储 -> 500
	svc := NewService(Deps{})
	if _, err := svc.ListIncidentIDsByProblem(ctx, 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("无仓储应 500")
	}
}
