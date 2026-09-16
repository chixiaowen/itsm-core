// 本文件定义 problem 域的入参/出参契约。
package problem

// CreateRequest 是新建/聚合问题请求体。
//
// source=aggregate 时必须提供 incident_ids（≥1）；source=manual 时忽略。
type CreateRequest struct {
	Title       string   `json:"title" binding:"required,max=255"`
	Description string   `json:"description" binding:"max=20000"`
	Source      string   `json:"source" binding:"omitempty,oneof=aggregate manual"`
	IncidentIDs []uint64 `json:"incident_ids"`
}

// UpdateRequest 是编辑问题/RCA 请求体（均可选）。
type UpdateRequest struct {
	Title       *string `json:"title" binding:"omitempty,max=255"`
	Description *string `json:"description" binding:"omitempty,max=20000"`
	Symptom     *string `json:"symptom" binding:"omitempty,max=20000"`
	Analysis    *string `json:"analysis" binding:"omitempty,max=20000"`
	RootCause   *string `json:"root_cause" binding:"omitempty,max=20000"`
	Workaround  *string `json:"workaround" binding:"omitempty,max=20000"`
}

// TransitionRequest 是状态流转请求体。
type TransitionRequest struct {
	Action         string  `json:"action" binding:"required"`
	Reason         string  `json:"reason" binding:"max=2000"`
	AssigneeID     *uint64 `json:"assignee_id"`
	NoChangeReason string  `json:"no_change_reason" binding:"max=20000"`
	RootCause      string  `json:"root_cause" binding:"max=20000"`
	Workaround     string  `json:"workaround" binding:"max=20000"`
}

// KnownErrorRequest 是标记/更新已知错误请求体。
//
// 空值不在此处拦截，由 service 统一返回 422（对齐 ARCHITECTURE §5.4）。
type KnownErrorRequest struct {
	RootCause  string `json:"root_cause" binding:"max=20000"`
	Workaround string `json:"workaround" binding:"max=20000"`
}

// AttachChangesRequest 是关联变更请求体。
type AttachChangesRequest struct {
	ChangeIDs []uint64 `json:"change_ids" binding:"required,min=1"`
}

// Detail 是问题详情响应体（含关联事件/变更 id）。
type Detail struct {
	*Problem
	IncidentIDs []uint64 `json:"incident_ids"`
	ChangeIDs   []uint64 `json:"change_ids"`
}
