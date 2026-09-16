package cmdb

// CIRequest 是 CI 新建/编辑请求体。
type CIRequest struct {
	Code    string         `json:"code" binding:"required,max=64"`
	Name    string         `json:"name" binding:"required,max=255"`
	CIType  string         `json:"ci_type" binding:"required"`
	Status  string         `json:"status"`
	OwnerID *uint64        `json:"owner_id"`
	Attrs   map[string]any `json:"attrs"`
}

// RelationRequest 是新增 CI 关系请求体（source 取自路径参数）。
type RelationRequest struct {
	TargetCIID   uint64 `json:"target_ci_id" binding:"required,min=1"`
	RelationType string `json:"relation_type" binding:"required"`
}

// CIDetail 是 CI 详情响应体（含直接关系）。
type CIDetail struct {
	CI        CI           `json:"ci"`
	Relations []CIRelation `json:"relations"`
}

// CIListResponse 是 CI 列表项（类型别名，便于文档化）。
type CIListResponse = CI

// TopologyResponse 是拓扑图响应体（对齐前端 {nodes, edges} 结构）。
type TopologyResponse = TopologyGraph
