package cmdb

import "sort"

// 拓扑展开方向常量。
const (
	// DirectionOut 仅向下游（source -> target）展开。
	DirectionOut = "out"
	// DirectionIn 仅向上游（target -> source）展开。
	DirectionIn = "in"
	// DirectionBoth 双向展开。
	DirectionBoth = "both"
)

// DefaultTopologyDepth 是拓扑展开默认层数。
const DefaultTopologyDepth = 2

// MaxTopologyDepth 是拓扑展开层数上限（防止超大图）。
const MaxTopologyDepth = 5

// TopologyNode 是拓扑图节点。
type TopologyNode struct {
	ID     uint64 `json:"id"`
	Code   string `json:"code"`
	Name   string `json:"name"`
	CIType string `json:"ci_type"`
	Status string `json:"status"`
	// Depth 为相对根节点的层级（根为 0）。
	Depth int `json:"depth"`
}

// TopologyEdge 是拓扑图边（有向关系）。
type TopologyEdge struct {
	ID           uint64 `json:"id"`
	SourceCIID   uint64 `json:"source_ci_id"`
	TargetCIID   uint64 `json:"target_ci_id"`
	RelationType string `json:"relation_type"`
}

// TopologyGraph 是拓扑图（前端按 {nodes, edges} 结构消费）。
type TopologyGraph struct {
	Root      uint64         `json:"root"`
	Depth     int            `json:"depth"`
	Direction string         `json:"direction"`
	Nodes     []TopologyNode `json:"nodes"`
	Edges     []TopologyEdge `json:"edges"`
}

// NormalizeDirection 归一化拓扑方向参数（缺省/非法回退 both）。
func NormalizeDirection(d string) string {
	switch d {
	case DirectionOut, DirectionIn, DirectionBoth:
		return d
	default:
		return DirectionBoth
	}
}

// NormalizeDepth 归一化拓扑层数（<=0 回退默认 2；超过上限截断）。
func NormalizeDepth(d int) int {
	if d <= 0 {
		return DefaultTopologyDepth
	}
	if d > MaxTopologyDepth {
		return MaxTopologyDepth
	}
	return d
}

// BuildTopology 是拓扑展开的纯函数实现：从 rootID 出发，按 direction 做 BFS 展开 depth 层。
//
// 入参为「数据快照」（全部 CI 与全部关系），不依赖任何仓储或 GORM，便于单测。
// 返回节点集合（含相对层级）与两端节点均被展开到的边集合；不存在的悬挂关系将被忽略。
func BuildTopology(rootID uint64, depth int, direction string, cis map[uint64]CI, relations []CIRelation) TopologyGraph {
	depth = NormalizeDepth(depth)
	direction = NormalizeDirection(direction)

	graph := TopologyGraph{
		Root:      rootID,
		Depth:     depth,
		Direction: direction,
		Nodes:     []TopologyNode{},
		Edges:     []TopologyEdge{},
	}

	if _, ok := cis[rootID]; !ok {
		return graph
	}

	level := map[uint64]int{rootID: 0}
	queue := []uint64{rootID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		curLevel := level[cur]
		if curLevel >= depth {
			continue
		}
		for _, rel := range relations {
			var next uint64
			matched := false
			switch direction {
			case DirectionOut:
				if rel.SourceCIID == cur {
					next, matched = rel.TargetCIID, true
				}
			case DirectionIn:
				if rel.TargetCIID == cur {
					next, matched = rel.SourceCIID, true
				}
			default: // both
				if rel.SourceCIID == cur {
					next, matched = rel.TargetCIID, true
				} else if rel.TargetCIID == cur {
					next, matched = rel.SourceCIID, true
				}
			}
			if !matched {
				continue
			}
			if _, exists := cis[next]; !exists {
				continue // 悬挂关系：目标 CI 不存在或被删除
			}
			if _, seen := level[next]; !seen {
				level[next] = curLevel + 1
				queue = append(queue, next)
			}
		}
	}

	ids := make([]uint64, 0, len(level))
	for id := range level {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		ci := cis[id]
		graph.Nodes = append(graph.Nodes, TopologyNode{
			ID:     ci.ID,
			Code:   ci.Code,
			Name:   ci.Name,
			CIType: ci.CIType,
			Status: ci.Status,
			Depth:  level[id],
		})
	}

	for _, rel := range relations {
		_, hasSource := level[rel.SourceCIID]
		_, hasTarget := level[rel.TargetCIID]
		if hasSource && hasTarget {
			graph.Edges = append(graph.Edges, TopologyEdge{
				ID:           rel.ID,
				SourceCIID:   rel.SourceCIID,
				TargetCIID:   rel.TargetCIID,
				RelationType: rel.RelationType,
			})
		}
	}
	sort.Slice(graph.Edges, func(i, j int) bool { return graph.Edges[i].ID < graph.Edges[j].ID })
	return graph
}
