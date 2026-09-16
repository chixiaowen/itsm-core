// 本文件是交付总监在 QA 因配额限制未能完成第 2 轮回归时，补做的独立验证用例。
//
// 目的：证实 change 域存在与 P1-2 同类的残留缺陷——
// 「变更的创建者本人被状态机拒之门外」。
//
// 背景（为什么现有测试没抓到）：
//
//	现有 service_test.go 用 reqr(5)（role.Requestor）充当"变更创建者"，
//	但 role.requestor 并不持有 perm.change.submit、现实中无法创建变更；
//	而真实创建者（agent/resolver/change_manager/cmdb_manager）走 HTTP 时
//	会在状态机 Roles 处被拒。测试用了一个"能过状态机但现实中到不了该状态"
//	的角色，因此是"假绿"。
//
// 本文件用【能真实创建变更的角色】复现真实路径。
package change

import (
	"context"
	"testing"
	"time"

	"github.com/chixiaowen/itsm-core/internal/pkg/httpx"
	"github.com/chixiaowen/itsm-core/internal/pkg/role"
)

// 真实创建者：持有 perm.change.submit 的 agent。
func creatorAgent(id uint64) Actor { return Actor{UserID: id, Role: role.Agent} }

// 另一名 agent（非创建者），用于负向对照。
func otherAgent(id uint64) Actor { return Actor{UserID: id, Role: role.Agent} }

// TestVerify_StandardPreAuthorize_ByRealCreator 验证「标准变更免审直通」可由创建者本人推进。
//
// PRD §5.5：标准变更为预授权（免 CAB 审批直通 approved），由变更申请人执行。
// 变更申请人 = Change.RequesterID，其角色必为持有 perm.change.submit 者。
func TestVerify_StandardPreAuthorize_ByRealCreator(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// 用 agent 创建标准变更（现实中唯一可达的路径）
	std, err := e.svc.Create(ctx, creatorAgent(100), CreateRequest{
		Title: "标准变更", ChangeType: TypeStandard, RiskLevel: RiskLow,
	})
	if err != nil {
		t.Fatalf("agent 创建标准变更失败: %v", err)
	}
	if std.RequesterID != 100 {
		t.Fatalf("RequesterID 应为 100，实际 %d", std.RequesterID)
	}

	// 创建者本人补齐方案
	plan, rb := "实施步骤", "回滚步骤"
	if _, err := e.svc.Update(ctx, creatorAgent(100), std.ID, UpdateRequest{Plan: &plan, RollbackPlan: &rb}); err != nil {
		t.Fatalf("创建者补方案失败: %v", err)
	}

	// 创建者本人执行免审直通 —— 这是缺陷点
	got, err := e.svc.Transition(ctx, creatorAgent(100), std.ID, ActionPreAuthorize,
		TransitionRequest{Action: ActionPreAuthorize})
	if err != nil {
		t.Fatalf("【缺陷】创建者本人推进标准变更免审直通被拒: %v（期望 200/approved）", err)
	}
	if got.Status != StatusApproved {
		t.Fatalf("期望状态 %s，实际 %s", StatusApproved, got.Status)
	}

	// 负向对照：非创建者的 agent 不应能推进他人工单
	std2, err := e.svc.Create(ctx, creatorAgent(101), CreateRequest{
		Title: "标准变更2", ChangeType: TypeStandard, RiskLevel: RiskLow,
	})
	if err != nil {
		t.Fatalf("创建第二条标准变更失败: %v", err)
	}
	if _, err := e.svc.Update(ctx, creatorAgent(101), std2.ID, UpdateRequest{Plan: &plan, RollbackPlan: &rb}); err != nil {
		t.Fatalf("补方案失败: %v", err)
	}
	if _, err := e.svc.Transition(ctx, otherAgent(999), std2.ID, ActionPreAuthorize,
		TransitionRequest{Action: ActionPreAuthorize}); err == nil {
		t.Fatalf("非创建者 agent 不应能推进他人的标准变更免审直通")
	}
}

// TestVerify_SubmitApproval_ByRealCreator 验证「提交 CAB 审批」可由创建者本人推进。
func TestVerify_SubmitApproval_ByRealCreator(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	ch, err := e.svc.Create(ctx, creatorAgent(100), CreateRequest{
		Title: "普通变更", ChangeType: TypeNormal, RiskLevel: RiskLow,
	})
	if err != nil {
		t.Fatalf("agent 创建普通变更失败: %v", err)
	}

	// 创建者本人提交风险评估
	if _, err := e.svc.Transition(ctx, creatorAgent(100), ch.ID, ActionSubmitAssessment,
		TransitionRequest{Action: ActionSubmitAssessment}); err != nil {
		t.Fatalf("创建者提交风险评估失败: %v", err)
	}

	plan, rb := "p", "r"
	if _, err := e.svc.Update(ctx, creatorAgent(100), ch.ID, UpdateRequest{Plan: &plan, RollbackPlan: &rb}); err != nil {
		t.Fatalf("补方案失败: %v", err)
	}

	// 创建者本人提交审批 —— 缺陷点
	if _, err := e.svc.Transition(ctx, creatorAgent(100), ch.ID, ActionSubmitApproval,
		TransitionRequest{Action: ActionSubmitApproval}); err != nil {
		t.Fatalf("【缺陷】创建者本人提交 CAB 审批被拒: %v（期望 200）", err)
	}
}

// TestVerify_ReviseResubmit_ByRealCreator 验证「驳回后修改 / 回滚后重提」可由创建者本人推进。
func TestVerify_ReviseResubmit_ByRealCreator(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// --- 驳回后修改（rejected -> draft）---
	ch, err := e.svc.Create(ctx, creatorAgent(100), CreateRequest{
		Title: "被驳回的变更", ChangeType: TypeNormal, RiskLevel: RiskLow,
	})
	if err != nil {
		t.Fatalf("创建变更失败: %v", err)
	}
	if _, err := e.svc.Transition(ctx, creatorAgent(100), ch.ID, ActionSubmitAssessment,
		TransitionRequest{Action: ActionSubmitAssessment}); err != nil {
		t.Fatalf("submit_assessment 失败: %v", err)
	}
	plan, rb := "p", "r"
	if _, err := e.svc.Update(ctx, creatorAgent(100), ch.ID, UpdateRequest{Plan: &plan, RollbackPlan: &rb}); err != nil {
		t.Fatalf("补方案失败: %v", err)
	}
	if _, err := e.svc.Transition(ctx, creatorAgent(100), ch.ID, ActionSubmitApproval,
		TransitionRequest{Action: ActionSubmitApproval}); err != nil {
		t.Fatalf("submit_approval 失败: %v", err)
	}
	if _, err := e.svc.RecordApproval(ctx, cm(7), ch.ID, ApprovalRequest{
		Decision: DecisionReject, Comment: "方案不足",
	}); err != nil {
		t.Fatalf("记录驳回失败: %v", err)
	}

	if _, err := e.svc.Transition(ctx, creatorAgent(100), ch.ID, ActionRevise,
		TransitionRequest{Action: ActionRevise}); err != nil {
		t.Fatalf("【缺陷】创建者本人对驳回变更执行 revise 被拒: %v（期望 200）", err)
	}

	// --- 回滚后重提（rolled_back -> draft）---
	ch2, err := e.svc.Create(ctx, creatorAgent(100), CreateRequest{
		Title: "回滚的变更", ChangeType: TypeNormal, RiskLevel: RiskLow,
	})
	if err != nil {
		t.Fatalf("创建变更2失败: %v", err)
	}
	if _, err := e.svc.Transition(ctx, creatorAgent(100), ch2.ID, ActionSubmitAssessment,
		TransitionRequest{Action: ActionSubmitAssessment}); err != nil {
		t.Fatalf("submit_assessment2 失败: %v", err)
	}
	if _, err := e.svc.Update(ctx, creatorAgent(100), ch2.ID, UpdateRequest{Plan: &plan, RollbackPlan: &rb}); err != nil {
		t.Fatalf("补方案2失败: %v", err)
	}
	if _, err := e.svc.Transition(ctx, creatorAgent(100), ch2.ID, ActionSubmitApproval,
		TransitionRequest{Action: ActionSubmitApproval}); err != nil {
		t.Fatalf("submit_approval2 失败: %v", err)
	}
	// 两名变更经理会签通过
	for _, id := range []uint64{7, 8} {
		if _, err := e.svc.RecordApproval(ctx, cm(id), ch2.ID, ApprovalRequest{Decision: DecisionApprove}); err != nil {
			if appErrCode(t, err) != httpx.CodeConflict {
				t.Fatalf("会签记录失败: %v", err)
			}
		}
	}
	// 排期：guardWindowOrder 要求 window_start/window_end 齐备（PRD §5.5），此处补齐前置条件。
	start := e.clock.Add(-time.Hour)
	end := e.clock.Add(time.Hour)
	if _, err := e.svc.Transition(ctx, cm(7), ch2.ID, ActionSchedule, TransitionRequest{Action: ActionSchedule, WindowStart: &start, WindowEnd: &end}); err != nil {
		t.Fatalf("schedule 失败: %v", err)
	}
	if _, err := e.svc.Transition(ctx, Actor{UserID: 9, Role: role.Resolver}, ch2.ID, ActionStartImplement,
		TransitionRequest{Action: ActionStartImplement}); err != nil {
		t.Fatalf("start_implement 失败: %v", err)
	}
	if _, err := e.svc.Transition(ctx, Actor{UserID: 9, Role: role.Resolver}, ch2.ID, ActionRollback,
		TransitionRequest{Action: ActionRollback, Reason: "实施异常"}); err != nil {
		t.Fatalf("rollback 失败: %v", err)
	}
	if _, err := e.svc.Transition(ctx, creatorAgent(100), ch2.ID, ActionResubmit,
		TransitionRequest{Action: ActionResubmit}); err != nil {
		t.Fatalf("【缺陷】创建者本人对回滚变更执行 resubmit 被拒: %v（期望 200）", err)
	}
}
