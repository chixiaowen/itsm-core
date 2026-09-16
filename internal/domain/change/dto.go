// 本文件定义 change 域的入参/出参契约。
package change

import "time"

// CreateRequest 是提交变更申请请求体。
type CreateRequest struct {
	Title       string `json:"title" binding:"required,max=255"`
	Description string `json:"description" binding:"max=20000"`
	ChangeType  string `json:"change_type" binding:"required,oneof=standard normal emergency"`
	RiskLevel   string `json:"risk_level" binding:"required,oneof=high medium low"`
}

// UpdateRequest 是编辑变更请求体（均可选）。
type UpdateRequest struct {
	Title          *string    `json:"title" binding:"omitempty,max=255"`
	Description    *string    `json:"description" binding:"omitempty,max=20000"`
	ChangeType     *string    `json:"change_type" binding:"omitempty,oneof=standard normal emergency"`
	RiskLevel      *string    `json:"risk_level" binding:"omitempty,oneof=high medium low"`
	ImpactAnalysis *string    `json:"impact_analysis" binding:"omitempty,max=20000"`
	Plan           *string    `json:"plan" binding:"omitempty,max=20000"`
	RollbackPlan   *string    `json:"rollback_plan" binding:"omitempty,max=20000"`
	WindowStart    *time.Time `json:"window_start"`
	WindowEnd      *time.Time `json:"window_end"`
}

// TransitionRequest 是状态流转请求体。
type TransitionRequest struct {
	Action             string     `json:"action" binding:"required"`
	Result             string     `json:"result"`
	Reason             string     `json:"reason"`
	Comment            string     `json:"comment"`
	Conclusion         string     `json:"conclusion"`
	WindowStart        *time.Time `json:"window_start"`
	WindowEnd          *time.Time `json:"window_end"`
	ConfirmOutOfWindow bool       `json:"confirm_out_of_window"`
}

// ApprovalRequest 是 CAB 审批请求体。
type ApprovalRequest struct {
	Decision string `json:"decision" binding:"required,oneof=approved rejected"`
	Comment  string `json:"comment" binding:"max=2000"`
}

// Detail 是变更详情响应体。
type Detail struct {
	*Change
	Approvals []ChangeApproval `json:"approvals"`
}
