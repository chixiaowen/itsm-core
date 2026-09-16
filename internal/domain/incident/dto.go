// 本文件定义 incident 域的入参/出参契约。
package incident

import "time"

// ReportRequest 是上报事件请求体。
type ReportRequest struct {
	Title       string     `json:"title" binding:"required,max=255"`
	Description string     `json:"description" binding:"max=20000"`
	Impact      string     `json:"impact" binding:"required,oneof=high medium low"`
	Urgency     string     `json:"urgency" binding:"required,oneof=high medium low"`
	OccurredAt  *time.Time `json:"occurred_at"`
	ReporterID  *uint64    `json:"reporter_id"`
	CIIDs       []uint64   `json:"ci_ids"`
}

// UpdateRequest 是编辑事件请求体（均可选）。
type UpdateRequest struct {
	Title       *string `json:"title" binding:"omitempty,max=255"`
	Description *string `json:"description" binding:"omitempty,max=20000"`
	Impact      *string `json:"impact" binding:"omitempty,oneof=high medium low"`
	Urgency     *string `json:"urgency" binding:"omitempty,oneof=high medium low"`
}

// TransitionRequest 是状态流转请求体。
type TransitionRequest struct {
	Action           string  `json:"action" binding:"required"`
	Solution         string  `json:"solution"`
	Reason           string  `json:"reason"`
	AssigneeID       *uint64 `json:"assignee_id"`
	ToAssigneeID     *uint64 `json:"to_assignee_id"`
	Level            *int    `json:"level"`
	Type             string  `json:"type" binding:"omitempty,oneof=functional hierarchical"`
	ReviewConclusion string  `json:"review_conclusion"`
}

// EscalateRequest 是升级请求体。
type EscalateRequest struct {
	Type         string  `json:"type" binding:"required,oneof=functional hierarchical"`
	Reason       string  `json:"reason" binding:"required,max=2000"`
	ToAssigneeID *uint64 `json:"to_assignee_id"`
	Level        *int    `json:"level"`
}

// PriorityRequest 是人工覆盖优先级请求体。
type PriorityRequest struct {
	Priority string `json:"priority" binding:"required,oneof=P1 P2 P3 P4"`
}

// LinkTicketRequest 是关联已有工单请求体。
type LinkTicketRequest struct {
	TicketID uint64 `json:"ticket_id" binding:"required,min=1"`
}

// AttachCIsRequest 是关联 CI 请求体。
type AttachCIsRequest struct {
	CIIDs []uint64 `json:"ci_ids" binding:"required,min=1"`
}

// ConvertToTicketRequest 是转工单请求体。
type ConvertToTicketRequest struct {
	Title string `json:"title" binding:"max=255"`
}

// Detail 是事件详情响应体。
type Detail struct {
	*Incident
	Escalations []IncidentEscalation `json:"escalations"`
	CIs         []uint64             `json:"ci_ids"`
}
