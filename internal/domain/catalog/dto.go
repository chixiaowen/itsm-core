package catalog

// CategoryRequest 是服务分类新建/编辑请求体。
type CategoryRequest struct {
	Name      string  `json:"name" binding:"required,max=128"`
	ParentID  *uint64 `json:"parent_id"`
	SortOrder int     `json:"sort_order"`
}

// CategoryNode 是服务分类树节点。
type CategoryNode struct {
	ID        uint64         `json:"id"`
	Name      string         `json:"name"`
	ParentID  *uint64        `json:"parent_id"`
	SortOrder int            `json:"sort_order"`
	ItemCount int            `json:"item_count"`
	Children  []CategoryNode `json:"children"`
}

// ItemRequest 是服务项新建/编辑请求体。
type ItemRequest struct {
	Name             string  `json:"name" binding:"required,max=128"`
	Description      string  `json:"description"`
	CategoryID       uint64  `json:"category_id" binding:"required,min=1"`
	SLAPolicyID      *uint64 `json:"sla_policy_id"`
	DefaultPriority  string  `json:"default_priority" binding:"omitempty,oneof=P1 P2 P3 P4"`
	RequiresApproval bool    `json:"requires_approval"`
	FormSchema       string  `json:"form_schema"`
}

// ItemTransitionRequest 是服务项状态流转请求体（如驳回）。
type ItemTransitionRequest struct {
	Action string `json:"action" binding:"required"`
	Reason string `json:"reason"`
}

// OrderRequest 是用户下单请求体。
type OrderRequest struct {
	// FormData 是用户填写的动态表单数据。
	FormData map[string]any `json:"form_data"`
	// Title 可选；缺省时由服务项名称生成。
	Title string `json:"title" binding:"max=255"`
}

// OrderResult 是下单结果（返回生成的工单标识）。
type OrderResult struct {
	TicketID uint64 `json:"ticket_id"`
}

// ItemResponse 是服务项响应体（类型别名，便于文档化）。
type ItemResponse = ServiceItem

// CategoryResponse 是分类响应体（类型别名）。
type CategoryResponse = ServiceCategory
