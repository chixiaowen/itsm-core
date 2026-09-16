// 本文件实现事件优先级矩阵（影响度 × 紧急度 3×3 → P1~P4），为纯函数，零依赖。
//
// 映射与 PRD §5.3.1 完全一致：
//
//	影响度\紧急度   高        中        低
//	高            P1        P2        P3
//	中            P2        P3        P4
//	低            P3        P4        P4
package incident

import "github.com/chixiaowen/itsm-core/internal/pkg/httpx"

// priorityMatrix 是影响度（行：高/中/低）× 紧急度（列：高/中/低）→ 优先级 的权威映射。
var priorityMatrix = [3][3]string{
	{PriorityP1, PriorityP2, PriorityP3}, // 高影响
	{PriorityP2, PriorityP3, PriorityP4}, // 中影响
	{PriorityP3, PriorityP4, PriorityP4}, // 低影响
}

// PriorityFromImpactUrgency 按影响度与紧急度计算优先级；取值非法返回 400。
func PriorityFromImpactUrgency(impact, urgency string) (string, error) {
	i, ok := impactRank(impact)
	if !ok {
		return "", httpx.ErrBadRequest("非法影响度: " + impact)
	}
	u, ok := urgencyRank(urgency)
	if !ok {
		return "", httpx.ErrBadRequest("非法紧急度: " + urgency)
	}
	return priorityMatrix[i][u], nil
}

// impactRank 返回影响度秩（0=高,1=中,2=低）。
func impactRank(s string) (int, bool) {
	switch s {
	case ImpactHigh:
		return 0, true
	case ImpactMedium:
		return 1, true
	case ImpactLow:
		return 2, true
	default:
		return 0, false
	}
}

// urgencyRank 返回紧急度秩（0=高,1=中,2=低）。
func urgencyRank(s string) (int, bool) {
	switch s {
	case UrgencyHigh:
		return 0, true
	case UrgencyMed:
		return 1, true
	case UrgencyLow:
		return 2, true
	default:
		return 0, false
	}
}

// ValidImpact 校验影响度取值。
func ValidImpact(s string) bool {
	_, ok := impactRank(s)
	return ok
}

// ValidUrgency 校验紧急度取值。
func ValidUrgency(s string) bool {
	_, ok := urgencyRank(s)
	return ok
}

// validPriority 校验优先级取值。
func validPriority(p string) bool {
	switch p {
	case PriorityP1, PriorityP2, PriorityP3, PriorityP4:
		return true
	default:
		return false
	}
}

// Matrix 返回 9 宫格矩阵（行=影响度 高/中/低，列=紧急度 高/中/低），供前端展示。
func Matrix() [][]string {
	impacts := []string{ImpactHigh, ImpactMedium, ImpactLow}
	urgencies := []string{UrgencyHigh, UrgencyMed, UrgencyLow}
	out := make([][]string, 0, len(impacts))
	for _, im := range impacts {
		row := make([]string, 0, len(urgencies))
		for _, ur := range urgencies {
			p, _ := PriorityFromImpactUrgency(im, ur)
			row = append(row, p)
		}
		out = append(out, row)
	}
	return out
}
