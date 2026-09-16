package statemachine

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testTable() Table {
	return Table{
		"new": {
			"assign": {
				To:    "assigned",
				Roles: []string{"agent", "admin"},
				Guard: func(in GuardInput) error {
					if v, _ := in.Params["assignee_id"].(uint64); v == 0 {
						return errors.New("指派对象无效")
					}
					return nil
				},
			},
			"cancel": {To: "cancelled", Roles: []string{"requestor"}},
		},
		"assigned": {
			"start": {To: "in_progress", Roles: []string{"agent", "resolver", "admin"}},
		},
		"open": {
			// 无角色限制
			"touch": {To: "open"},
		},
	}
}

func TestResolve_Hit(t *testing.T) {
	tb := testTable()
	tr, ok := tb.Resolve("new", "assign")
	if !ok {
		t.Fatalf("期望命中 new->assign")
	}
	if tr.To != "assigned" {
		t.Fatalf("期望 To=assigned，实际 %q", tr.To)
	}
	if tr.Guard == nil {
		t.Fatalf("期望 assign 带 Guard")
	}
}

func TestResolve_UnknownFrom(t *testing.T) {
	if _, ok := testTable().Resolve("nonexistent", "assign"); ok {
		t.Fatalf("未知 from 不应命中")
	}
}

func TestResolve_UnknownAction(t *testing.T) {
	if _, ok := testTable().Resolve("new", "start"); ok {
		t.Fatalf("当前状态下不允许的 action 不应命中")
	}
}

func TestAllows_EmptyRolesMeansAllow(t *testing.T) {
	tr := Transition{To: "open"}
	if !tr.Allows("anyone") {
		t.Fatalf("Roles 为空应放行任意角色")
	}
}

func TestAllows_MatchAndMiss(t *testing.T) {
	tr := Transition{To: "assigned", Roles: []string{"agent", "admin"}}
	if !tr.Allows("agent") {
		t.Fatalf("agent 应被允许")
	}
	if !tr.Allows("admin") {
		t.Fatalf("admin 应被允许")
	}
	if tr.Allows("requestor") {
		t.Fatalf("requestor 不应被允许")
	}
}

func TestGuard_Invocation(t *testing.T) {
	tb := testTable()
	tr, _ := tb.Resolve("new", "assign")

	in := GuardInput{
		Ctx:    context.Background(),
		Actor:  Actor{UserID: 1, Role: "agent"},
		Entity: nil,
		Params: map[string]any{"assignee_id": uint64(2)},
		Now:    time.Now(),
	}
	if err := tr.Guard(in); err != nil {
		t.Fatalf("有效 assignee 应通过 guard，实际 %v", err)
	}

	in.Params["assignee_id"] = uint64(0)
	if err := tr.Guard(in); err == nil {
		t.Fatalf("无效 assignee 应被 guard 拒绝")
	}
}

func TestActions(t *testing.T) {
	actions := testTable().Actions("new")
	if len(actions) != 2 {
		t.Fatalf("期望 2 个 action，实际 %d", len(actions))
	}
	if got := testTable().Actions("unknown"); got != nil {
		t.Fatalf("未知状态应返回 nil，实际 %v", got)
	}
}
