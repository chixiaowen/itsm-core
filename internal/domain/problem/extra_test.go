package problem

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
)

// TestModelAndGormHelpers 覆盖模型命名、构造器与纯函数。
func TestModelAndGormHelpers(t *testing.T) {
	if (Problem{}).TableName() != "problems" {
		t.Fatalf("Problem 表名错误")
	}
	if (ProblemChange{}).TableName() != "problem_changes" {
		t.Fatalf("ProblemChange 表名错误")
	}
	// 构造器（不触碰 DB）
	_ = NewRepository((*gorm.DB)(nil))
	_ = NewProblemChangeRepository((*gorm.DB)(nil))

	if problemSortColumn("status") != "status" || problemSortColumn("id") != "id" || problemSortColumn("x") != "created_at" {
		t.Fatalf("problemSortColumn 错误")
	}
	if orderDir("asc") != "ASC" || orderDir("desc") != "DESC" || orderDir("") != "DESC" {
		t.Fatalf("orderDir 错误")
	}
	if !errors.Is(mapNotFound(gorm.ErrRecordNotFound), ErrNotFound) {
		t.Fatalf("mapNotFound 应映射 ErrNotFound")
	}
	if errors.Is(mapNotFound(errors.New("boom")), ErrNotFound) {
		t.Fatalf("mapNotFound 不应映射普通错误")
	}
	if sb, od := normalizeSort("status", "asc"); sb != "status" || od != "asc" {
		t.Fatalf("normalizeSort 合法值应保留")
	}
	if sb, od := normalizeSort("", ""); sb != "created_at" || od != "desc" {
		t.Fatalf("normalizeSort 默认值错误: %s %s", sb, od)
	}
}

// TestDetailFailures 覆盖详情查询中事件/变更读取故障。
func TestDetailFailures(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	p := e.createManual(t, pm(7), "d1")

	e.incidents.failRead = true
	if _, err := e.svc.Detail(ctx, pm(7), p.ID); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("事件读取故障应 500")
	}
	e.incidents.failRead = false
	e.changes.fail = true
	if _, err := e.svc.Detail(ctx, pm(7), p.ID); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("变更读取故障应 500")
	}
	e.changes.fail = false
	// 正常详情
	if _, err := e.svc.Detail(ctx, pm(7), p.ID); err != nil {
		t.Fatalf("正常详情失败: %v", err)
	}
}

// TestChangeLinkFailures 覆盖关联/解除变更的仓储故障。
func TestChangeLinkFailures(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	p := e.createManual(t, pm(7), "c1")
	e.changes.fail = true
	if err := e.svc.AddChanges(ctx, pm(7), p.ID, []uint64{1}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("关联变更故障应 500")
	}
	if err := e.svc.RemoveChange(ctx, pm(7), p.ID, 1); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("解除变更故障应 500")
	}
	e.changes.fail = false
	// 关联不存在的问题 -> 404
	if err := e.svc.AddChanges(ctx, pm(7), 9999, []uint64{1}); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("关联不存在问题应 404")
	}
	if err := e.svc.RemoveChange(ctx, pm(7), 9999, 1); appErrCode(t, err) != httpx.CodeNotFound {
		t.Fatalf("解除不存在问题应 404")
	}
}

// TestAssigneeAndResolveReaders 覆盖指派人校验与解决约束的读取分支。
func TestAssigneeAndResolveReaders(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	p := e.createManual(t, pm(7), "a1")
	e.advanceTo(t, p.ID, StatusTriage)

	// investigate 指派不存在的用户 -> 422
	bad := uint64(999)
	if _, err := e.svc.Transition(ctx, pm(7), p.ID, ActionInvestigate, TransitionRequest{Action: ActionInvestigate, AssigneeID: &bad}); appErrCode(t, err) != httpx.CodePrecondition {
		t.Fatalf("指派不存在用户应 422")
	}
	// users 组件缺失时跳过校验，指派有效 id 成功
	svcNoUsers := NewService(Deps{
		Repo: e.repo, Changes: e.changes, Incidents: e.incidents, ChangeRead: e.changeRead,
		Now: func() time.Time { return *e.clock },
	})
	good := uint64(7)
	if _, err := svcNoUsers.Transition(ctx, pm(7), p.ID, ActionInvestigate, TransitionRequest{Action: ActionInvestigate, AssigneeID: &good}); err != nil {
		t.Fatalf("无用户组件时应放行: %v", err)
	}
	// 解决：changeRead 读取故障 -> 500
	_ = e.changes.Add(ctx, p.ID, []uint64{1})
	e.changeRead.fail = true
	if _, err := e.svc.Transition(ctx, pm(7), p.ID, ActionResolve, TransitionRequest{Action: ActionResolve, NoChangeReason: "x"}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("变更读取故障应 500")
	}
	e.changeRead.fail = false
	// changeRead 缺失时用 no_change_reason 放行
	svcNoChangeRead := NewService(Deps{Repo: e.repo, Changes: e.changes, Incidents: e.incidents, Now: func() time.Time { return *e.clock }})
	if _, err := svcNoChangeRead.Transition(ctx, pm(7), p.ID, ActionResolve, TransitionRequest{Action: ActionResolve, NoChangeReason: "热修复"}); err != nil {
		t.Fatalf("无变更读组件应可用 no_change_reason 解决: %v", err)
	}
}

// TestWriteFailures 覆盖仓储写故障与编号生成故障。
func TestWriteFailures(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	e.repo.failSeq = true
	if _, err := e.svc.Create(ctx, pm(7), CreateRequest{Title: "x"}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("编号生成故障应 500")
	}
	e.repo.failSeq = false

	e.repo.failWrite = true
	if _, err := e.svc.Create(ctx, pm(7), CreateRequest{Title: "x"}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("创建写故障应 500")
	}
	e.repo.failWrite = false

	// 创建一张正常问题后注入写故障，覆盖 Update/Delete/Transition 故障分支
	p := e.createManual(t, pm(7), "w1")
	e.repo.failWrite = true
	if _, err := e.svc.Update(ctx, pm(7), p.ID, UpdateRequest{}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("更新写故障应 500")
	}
	if err := e.svc.Delete(ctx, admin(), p.ID); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("删除写故障应 500")
	}
	if _, err := e.svc.Transition(ctx, pm(7), p.ID, ActionTriage, TransitionRequest{Action: ActionTriage}); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("流转写故障应 500")
	}
	e.repo.failWrite = false
}

// TestAggregateSuggestionsFailures 覆盖建议聚合统计故障。
func TestAggregateSuggestionsFailures(t *testing.T) {
	e := newTestEnv(t)
	e.incidents.failRead = true
	if _, err := e.svc.AggregateSuggestions(context.Background(), 1, "db", 30); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("CI 统计故障应 500")
	}
}

// TestSuggestionsKeywordFailure 覆盖关键词统计故障。
func TestSuggestionsKeywordFailure(t *testing.T) {
	e := newTestEnv(t)
	e.incidents.failRead = true
	if _, err := e.svc.AggregateSuggestions(context.Background(), 0, "db", 30); appErrCode(t, err) != httpx.CodeInternal {
		t.Fatalf("关键词统计故障应 500")
	}
}
