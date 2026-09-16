package cmdb

import "testing"

// buildChain 构造 a->b->c->d 链式关系与对应 CI 映射。
func buildChain() (map[uint64]CI, []CIRelation) {
	cis := map[uint64]CI{
		1: {ID: 1, Code: "a", Name: "A", CIType: CITypeServer, Status: StatusInUse},
		2: {ID: 2, Code: "b", Name: "B", CIType: CITypeDatabase, Status: StatusInUse},
		3: {ID: 3, Code: "c", Name: "C", CIType: CITypeApp, Status: StatusInUse},
		4: {ID: 4, Code: "d", Name: "D", CIType: CITypeOther, Status: StatusInUse},
	}
	rels := []CIRelation{
		{ID: 11, SourceCIID: 1, TargetCIID: 2, RelationType: RelationDependsOn},
		{ID: 12, SourceCIID: 2, TargetCIID: 3, RelationType: RelationDependsOn},
		{ID: 13, SourceCIID: 3, TargetCIID: 4, RelationType: RelationDependsOn},
	}
	return cis, rels
}

func nodeIDs(g TopologyGraph) []uint64 {
	out := make([]uint64, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		out = append(out, n.ID)
	}
	return out
}

func TestBuildTopology_DefaultDepthTwo(t *testing.T) {
	cis, rels := buildChain()
	g := BuildTopology(1, 0, "", cis, rels)

	if g.Depth != DefaultTopologyDepth || g.Direction != DirectionBoth {
		t.Fatalf("默认应为 2 层 both，实际 depth=%d dir=%s", g.Depth, g.Direction)
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("默认 2 层应为 3 节点，实际 %v", nodeIDs(g))
	}
	// 根节点 depth=0，其余递增
	depths := map[uint64]int{}
	for _, n := range g.Nodes {
		depths[n.ID] = n.Depth
	}
	if depths[1] != 0 || depths[2] != 1 || depths[3] != 2 {
		t.Fatalf("层级错误: %+v", depths)
	}
	if len(g.Edges) != 2 {
		t.Fatalf("应包含 2 条边，实际 %d", len(g.Edges))
	}
}

func TestBuildTopology_DepthAndDirection(t *testing.T) {
	cis, rels := buildChain()

	// 向外 3 层 → 全部 4 节点
	g := BuildTopology(1, 3, DirectionOut, cis, rels)
	if len(g.Nodes) != 4 {
		t.Fatalf("out 3 层应为 4 节点，实际 %v", nodeIDs(g))
	}

	// 从 d 向上游 2 层 → d(0), c(1), b(2) = 3 节点
	g = BuildTopology(4, 2, DirectionIn, cis, rels)
	if len(g.Nodes) != 3 {
		t.Fatalf("in 2 层应为 3 节点，实际 %v", nodeIDs(g))
	}
	for _, n := range g.Nodes {
		if n.ID == 1 {
			t.Fatalf("向上游 2 层不应包含 a")
		}
	}

	// 从 b 双向 1 层 → a,b,c = 3 节点
	g = BuildTopology(2, 1, DirectionBoth, cis, rels)
	if len(g.Nodes) != 3 {
		t.Fatalf("both 1 层应为 3 节点，实际 %v", nodeIDs(g))
	}
}

func TestBuildTopology_CycleAndDanglingAndMissingRoot(t *testing.T) {
	cis := map[uint64]CI{
		1: {ID: 1, Code: "a", Name: "A", CIType: CITypeServer, Status: StatusInUse},
		2: {ID: 2, Code: "b", Name: "B", CIType: CITypeServer, Status: StatusInUse},
	}
	// 含环（1->2, 2->1）与悬挂关系（2->99，99 不存在）
	rels := []CIRelation{
		{ID: 1, SourceCIID: 1, TargetCIID: 2, RelationType: RelationConnectsTo},
		{ID: 2, SourceCIID: 2, TargetCIID: 1, RelationType: RelationConnectsTo},
		{ID: 3, SourceCIID: 2, TargetCIID: 99, RelationType: RelationConnectsTo},
	}
	g := BuildTopology(1, 5, DirectionBoth, cis, rels)
	if len(g.Nodes) != 2 {
		t.Fatalf("环不应导致重复节点，实际 %v", nodeIDs(g))
	}
	for _, e := range g.Edges {
		if e.TargetCIID == 99 {
			t.Fatalf("悬挂关系不应出现在边集合")
		}
	}

	// 根不存在 → 空图
	g = BuildTopology(123, 2, "", cis, rels)
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Fatalf("根不存在应返回空图，实际 %+v", g)
	}
}

func TestNormalizeDepthDirection(t *testing.T) {
	if NormalizeDepth(-1) != DefaultTopologyDepth {
		t.Fatalf("非法层数应回退默认")
	}
	if NormalizeDepth(99) != MaxTopologyDepth {
		t.Fatalf("超限层数应截断")
	}
	if NormalizeDirection("sideways") != DirectionBoth {
		t.Fatalf("非法方向应回退 both")
	}
}
