// 本文件定义 ticket 域的入参/出参契约（纯结构体 + 校验标签，禁止 GORM）。
package ticket

import "github.com/chixiaowen/itsm-core/internal/domain/platform"

// CreateTicketRequest 是新建工单请求体。
type CreateTicketRequest struct {
	Title            string  `json:"title" binding:"required,max=255"`
	Description      string  `json:"description" binding:"max=20000"`
	CategoryID       *uint64 `json:"category_id"`
	Priority         string  `json:"priority" binding:"omitempty,oneof=P1 P2 P3 P4"`
	RequesterID      *uint64 `json:"requester_id"`
	Type             string  `json:"type" binding:"omitempty,oneof=manual service incident"`
	SourceIncidentID *uint64 `json:"source_incident_id"`
	ServiceItemID    *uint64 `json:"service_item_id"`
	FormData         string  `json:"form_data"`
}

// UpdateTicketRequest 是编辑工单请求体（字段均可选）。
type UpdateTicketRequest struct {
	Title       *string `json:"title" binding:"omitempty,max=255"`
	Description *string `json:"description" binding:"omitempty,max=20000"`
	CategoryID  *uint64 `json:"category_id"`
	Priority    *string `json:"priority" binding:"omitempty,oneof=P1 P2 P3 P4"`
}

// TransitionRequest 是统一状态流转请求体。
type TransitionRequest struct {
	Action     string  `json:"action" binding:"required"`
	Solution   string  `json:"solution"`
	Reason     string  `json:"reason"`
	AssigneeID *uint64 `json:"assignee_id"`
	Priority   string  `json:"priority" binding:"omitempty,oneof=P1 P2 P3 P4"`
}

// AssignRequest 是快捷指派请求体。
type AssignRequest struct {
	AssigneeID uint64 `json:"assignee_id" binding:"required,min=1"`
}

// RatingRequest 是满意度评价请求体。
type RatingRequest struct {
	Rating  int    `json:"rating" binding:"required,min=1,max=5"`
	Comment string `json:"comment" binding:"max=500"`
}

// TicketCommentRequest 是工单公开回复请求体。
type TicketCommentRequest struct {
	Content    string `json:"content" binding:"required,max=5000"`
	IsInternal bool   `json:"is_internal"`
}

// CategoryRequest 是工单分类新建/编辑请求体。
type CategoryRequest struct {
	Name      string  `json:"name" binding:"required,max=128"`
	ParentID  *uint64 `json:"parent_id"`
	SortOrder int     `json:"sort_order"`
}

// AttachCIsRequest 是关联 CI 请求体。
type AttachCIsRequest struct {
	CIIDs []uint64 `json:"ci_ids" binding:"required,min=1"`
}

// Detail 是工单详情响应体（含关联 CI 与评论时间线）。
type Detail struct {
	*Ticket
	CIs      []uint64           `json:"ci_ids"`
	Comments []platform.Comment `json:"comments"`
}
