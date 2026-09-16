package ticket

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

// TestRepoHelpers 覆盖 repository_gorm.go 中的纯函数（无需 DB）。
func TestRepoHelpers(t *testing.T) {
	cases := map[string]string{
		"priority": "priority", "status": "status", "id": "id",
		"created_at": "created_at", "bogus": "created_at", "": "created_at",
	}
	for in, want := range cases {
		if got := sortColumn(in); got != want {
			t.Fatalf("sortColumn(%q)=%q want %q", in, got, want)
		}
	}
	if orderDir("asc") != "ASC" || orderDir("ASC") != "ASC" || orderDir("desc") != "DESC" || orderDir("") != "DESC" {
		t.Fatalf("orderDir 归一化错误")
	}
	if !errors.Is(mapNotFound(gorm.ErrRecordNotFound), ErrNotFound) {
		t.Fatalf("mapNotFound 应归一化 not found")
	}
	other := errors.New("x")
	if !errors.Is(mapNotFound(other), other) {
		t.Fatalf("mapNotFound 应透传其它错误")
	}
}

// TestTableNames 覆盖实体表名。
func TestTableNames(t *testing.T) {
	if (Ticket{}).TableName() != "tickets" || (TicketCategory{}).TableName() != "ticket_categories" || (TicketCI{}).TableName() != "ticket_cis" {
		t.Fatalf("表名不符")
	}
}

// TestDraftSubmitFlow 覆盖 guardRequiredFields 与 draft 流转。
func TestDraftSubmitFlow(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	draft := &Ticket{Code: "TKT-20260916-6001", Title: "草稿工单", Status: StatusDraft, Priority: PriorityP4, RequesterID: 20}
	if err := e.repo.Create(ctx, draft); err != nil {
		t.Fatalf("准备草稿失败: %v", err)
	}
	got, err := e.svc.Transition(ctx, agent(7), draft.ID, ActionSubmit, TransitionRequest{Action: ActionSubmit})
	if err != nil || got.Status != StatusNew {
		t.Fatalf("draft submit 应转 new: %v", err)
	}

	// 缺必填（requester=0）→ guardRequiredFields 422
	bad := &Ticket{Code: "TKT-20260916-6002", Title: "", Status: StatusDraft, Priority: PriorityP4}
	_ = e.repo.Create(ctx, bad)
	if _, err := e.svc.Transition(ctx, agent(7), bad.ID, ActionSubmit, TransitionRequest{Action: ActionSubmit}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("草稿缺字段提交应 422，实际 %v", err)
	}
}

// TestUpdateFieldsAndForbidden 覆盖编辑更多字段与权限。
func TestUpdateFieldsAndForbidden(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	desc := "新描述"
	cat := uint64(5)
	title := "新标题"
	got, err := e.svc.Update(ctx, agent(7), tk.ID, UpdateTicketRequest{Title: &title, Description: &desc, CategoryID: &cat})
	if err != nil {
		t.Fatalf("编辑失败: %v", err)
	}
	if got.Title != title || got.Description != desc || got.CategoryID == nil || *got.CategoryID != cat {
		t.Fatalf("编辑字段不符: %+v", got)
	}
	empty := "   "
	if _, err := e.svc.Update(ctx, agent(7), tk.ID, UpdateTicketRequest{Title: &empty}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("空标题应 400")
	}
	// 非本人 requestor 编辑 → 403
	if _, err := e.svc.Update(ctx, requestor(21), tk.ID, UpdateTicketRequest{Title: &title}); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("非本人编辑应 403，实际 %v", err)
	}
	// 不存在 → 404
	if _, err := e.svc.Update(ctx, agent(7), 999, UpdateTicketRequest{Title: &title}); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("不存在应 404")
	}
}

// TestServiceDefaults 覆盖 NewService 默认值。
func TestServiceDefaults(t *testing.T) {
	svc := NewService(Deps{Now: nil, IDGen: nil, Logger: nil})
	if svc.now == nil || svc.gen == nil || svc.log == nil {
		t.Fatalf("默认依赖未回填")
	}
}

// TestCreateFromIncident_DuplicateCodeIsolated 覆盖编号递增。
func TestNextCodeIncrements(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	a, _ := e.svc.Create(ctx, requestor(20), CreateTicketRequest{Title: "1"})
	b, _ := e.svc.Create(ctx, requestor(20), CreateTicketRequest{Title: "2"})
	if a.Code == b.Code {
		t.Fatalf("编号不应重复")
	}
	if a.Code != "TKT-20260916-0001" || b.Code != "TKT-20260916-0002" {
		t.Fatalf("编号序列不符: %s %s", a.Code, b.Code)
	}
}

// TestHandlerQueryParams 覆盖 handler 各 query 参数解析分支。
func TestHandlerQueryParams(t *testing.T) {
	r, jwt, _ := newTicketEngine(t)
	agentTok := tkToken(t, jwt, 7, role.Agent)
	url := "/api/v1/tickets?status=new&priority=P3&type=manual&category_id=1&assignee_id=7&requester_id=20" +
		"&sla_status=normal&keyword=abc&from=2026-09-01T00:00:00Z&to=2026-09-30T00:00:00Z&sort_by=priority&order=asc&page=1&page_size=5"
	if w := tkReq(r, http.MethodGet, url, "", agentTok); w.Code != http.StatusOK {
		t.Fatalf("带参列表应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
}

// TestTransitionUnknownAction 覆盖未知 action → 409。
func TestTransitionUnknownAction(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	if _, err := e.svc.Transition(ctx, agent(7), tk.ID, "no_such_action", TransitionRequest{Action: "no_such_action"}); appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("未知 action 应 409，实际 %v", err)
	}
}

// TestRating_Bounds 覆盖评分越界校验与「admin 不可评价」（PRD §2.2：满意度评价 admin=✗）。
func TestRating_Bounds(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20))
	aid := uint64(7)
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionAssign, TransitionRequest{Action: ActionAssign, AssigneeID: &aid})
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionStart, TransitionRequest{Action: ActionStart})
	_, _ = e.svc.Transition(ctx, agent(7), tk.ID, ActionResolve, TransitionRequest{Action: ActionResolve, Solution: "ok"})
	if _, err := e.svc.Rate(ctx, requestor(20), tk.ID, RatingRequest{Rating: 9}); appErrCode(t, err) != httpx.CodeInvalidParam {
		t.Fatalf("越界评分应 400，实际 %v", err)
	}
	// admin 亦不得评价（PRD §2.2：满意度评价 admin=✗）。
	if _, err := e.svc.Rate(ctx, admin(), tk.ID, RatingRequest{Rating: 4}); appErrCode(t, err) != httpx.CodeForbidden {
		t.Fatalf("admin 评价应 403，实际 %v", err)
	}
	// 请求人本人评价成功。
	if _, err := e.svc.Rate(ctx, requestor(20), tk.ID, RatingRequest{Rating: 4}); err != nil {
		t.Fatalf("请求人评价应成功: %v", err)
	}
}

// TestDeleteCategory_NotFound 覆盖删除不存在分类。
func TestDeleteCategory_NotFound(t *testing.T) {
	e := newTestEnv(t)
	if err := e.svc.DeleteCategory(context.Background(), admin(), 999); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("删除不存在分类应 404")
	}
}

// TestSLAView_Breached 覆盖 decorate 的 breached 分支。
func TestSLAView_Breached(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk, _ := e.svc.Create(ctx, requestor(20), CreateTicketRequest{Title: "紧急", Priority: PriorityP1})
	e.advance(5 * time.Hour) // 超过 P1 4h
	got, _ := e.svc.Get(ctx, agent(7), tk.ID)
	if got.SLAStatus != "breached" {
		t.Fatalf("应 breached，实际 %s", got.SLAStatus)
	}
}

// TestIllegalTransition409CarriesStatePair 校验工单非法流转 409 消息统一含「当前状态 -> 目标状态」（docs/API.md）。
func TestIllegalTransition409CarriesStatePair(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	tk := e.createTicket(t, requestor(20)) // new
	// new 直接 start（跳过分派）→ 409，消息含 current=new -> target=in_progress。
	_, err := e.svc.Transition(ctx, agent(7), tk.ID, ActionStart, TransitionRequest{Action: ActionStart})
	if appErrCode(t, err) != httpx.CodeConflict {
		t.Fatalf("非法流转应 409，实际 %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "current="+StatusNew) || !strings.Contains(msg, "target="+StatusInProgress) {
		t.Fatalf("非法流转消息应含 current=new -> target=in_progress，实际 %q", msg)
	}
}
