package incident

import "testing"

// TestPriorityMatrix_AllNine 覆盖 9 种组合，逐条对齐 PRD §5.3.1。
func TestPriorityMatrix_AllNine(t *testing.T) {
	cases := []struct {
		impact, urgency, want string
	}{
		{ImpactHigh, UrgencyHigh, PriorityP1},
		{ImpactHigh, UrgencyMed, PriorityP2},
		{ImpactHigh, UrgencyLow, PriorityP3},
		{ImpactMedium, UrgencyHigh, PriorityP2},
		{ImpactMedium, UrgencyMed, PriorityP3},
		{ImpactMedium, UrgencyLow, PriorityP4},
		{ImpactLow, UrgencyHigh, PriorityP3},
		{ImpactLow, UrgencyMed, PriorityP4},
		{ImpactLow, UrgencyLow, PriorityP4},
	}
	for _, tc := range cases {
		got, err := PriorityFromImpactUrgency(tc.impact, tc.urgency)
		if err != nil {
			t.Fatalf("(%s,%s) 计算失败: %v", tc.impact, tc.urgency, err)
		}
		if got != tc.want {
			t.Fatalf("(%s,%s) = %s，期望 %s", tc.impact, tc.urgency, got, tc.want)
		}
	}
}

func TestPriorityMatrix_InvalidInputs(t *testing.T) {
	if _, err := PriorityFromImpactUrgency("bad", UrgencyHigh); err == nil {
		t.Fatalf("非法影响度应报错")
	}
	if _, err := PriorityFromImpactUrgency(ImpactHigh, "bad"); err == nil {
		t.Fatalf("非法紧急度应报错")
	}
	if !ValidImpact(ImpactHigh) || ValidImpact("x") || !ValidUrgency(UrgencyLow) || ValidUrgency("x") {
		t.Fatalf("取值校验错误")
	}
}

func TestMatrix_Shape(t *testing.T) {
	m := Matrix()
	if len(m) != 3 {
		t.Fatalf("矩阵应为 3 行，实际 %d", len(m))
	}
	for i, row := range m {
		if len(row) != 3 {
			t.Fatalf("第 %d 行应为 3 列", i)
		}
	}
	if m[0][0] != PriorityP1 || m[2][2] != PriorityP4 {
		t.Fatalf("矩阵端点不符: %v", m)
	}
}

func TestValidPriority(t *testing.T) {
	for _, p := range []string{PriorityP1, PriorityP2, PriorityP3, PriorityP4} {
		if !validPriority(p) {
			t.Fatalf("%s 应合法", p)
		}
	}
	if validPriority("P9") {
		t.Fatalf("P9 应非法")
	}
}
