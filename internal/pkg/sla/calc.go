// Package sla 提供 SLA 计时的纯函数实现（零依赖、可独立单测）。
//
// 计时口径（PRD §5.2.1 / ARCHITECTURE §7）：
//   - 响应 Response：created_at -> 首次响应；不暂停。
//   - 解决 Resolution：created_at -> resolved_at；pending 期间暂停（paused_minutes 回补）。
//   - due_at = 起点 + 目标分钟（7×24 自然时间）。
//   - 三档：normal（剩余 > 20%）/ warning（剩余 <= 20%）/ breached（now > due_at 未达成）。
package sla

import "time"

// Policy 是按优先级定义的 SLA 策略。
type Policy struct {
	Priority        string
	ResponseMinutes int
	ResolveMinutes  int
	PauseOnPending  bool
}

// Status 是 SLA 三档状态。
type Status string

// SLA 三档取值。
const (
	Normal   Status = "normal"
	Warning  Status = "warning"
	Breached Status = "breached"
)

// warningRatio 是预警阈值：剩余时间 <= 目标时长的 20% 时进入 warning。
const warningRatio = 0.2

// Due 计算截止时间：start + minutes 分钟。
func Due(start time.Time, minutes int) time.Time {
	return start.Add(time.Duration(minutes) * time.Minute)
}

// Elapsed 计算净耗时 = end - start - pausedMinutes（end 为 nil 时以 now 为终点）。
//
// 净耗时不会为负：paused 超过总时长时归零。
func Elapsed(start time.Time, end *time.Time, now time.Time, pausedMinutes int) time.Duration {
	ref := now
	if end != nil {
		ref = *end
	}
	d := ref.Sub(start) - time.Duration(pausedMinutes)*time.Minute
	if d < 0 {
		return 0
	}
	return d
}

// StatusOf 判定 SLA 三档。
//
//   - 已达成（end != nil）：end > dueAt -> breached，否则 normal。
//   - 未达成（end == nil）：now > dueAt -> breached；剩余 <= 20% 目标 -> warning；否则 normal。
func StatusOf(start time.Time, targetMinutes int, end *time.Time, now time.Time, pausedMinutes int) Status {
	// 回补暂停时间：等价于把起点后移 pausedMinutes（暂停期间不消耗 SLA）。
	effectiveStart := start.Add(time.Duration(pausedMinutes) * time.Minute)
	dueAt := Due(effectiveStart, targetMinutes)

	if end != nil {
		if end.After(dueAt) {
			return Breached
		}
		return Normal
	}
	if now.After(dueAt) {
		return Breached
	}
	remaining := dueAt.Sub(now)
	threshold := time.Duration(float64(targetMinutes)*warningRatio) * time.Minute
	if remaining <= threshold {
		return Warning
	}
	return Normal
}

// View 汇总工单/事件的 SLA 展示数据（service 与前端共用，不落库）。
type View struct {
	ResponseDueAt  *time.Time    `json:"response_due_at"`
	ResolveDueAt   *time.Time    `json:"resolve_due_at"`
	ResponseStatus Status        `json:"response_status"`
	ResolveStatus  Status        `json:"resolve_status"`
	SLAStatus      Status        `json:"sla_status"`
	Remaining      time.Duration `json:"remaining"`
}

// Evaluate 计算 SLA 视图。
//
//   - firstRespondedAt 非空表示响应已达成；resolvedAt 非空表示解决已达成。
//   - 响应不暂停；解决按 pausedMinutes 回补。
//   - SLAStatus 取响应/解决中更严重者（breached > warning > normal）。
func Evaluate(p Policy, createdAt, now time.Time, firstRespondedAt, resolvedAt *time.Time, pausedMinutes int) View {
	// 响应不暂停；解决按 pausedMinutes 回补（等价于起点后移）。
	respDue := Due(createdAt, p.ResponseMinutes)
	resolveDue := Due(createdAt.Add(time.Duration(pausedMinutes)*time.Minute), p.ResolveMinutes)

	respStatus := StatusOf(createdAt, p.ResponseMinutes, firstRespondedAt, now, 0)
	resolveStatus := StatusOf(createdAt, p.ResolveMinutes, resolvedAt, now, pausedMinutes)

	remaining := resolveDue.Sub(now)
	if remaining < 0 {
		remaining = 0
	}

	return View{
		ResponseDueAt:  &respDue,
		ResolveDueAt:   &resolveDue,
		ResponseStatus: respStatus,
		ResolveStatus:  resolveStatus,
		SLAStatus:      worse(respStatus, resolveStatus),
		Remaining:      remaining,
	}
}

// worse 返回更严重的状态。
func worse(a, b Status) Status {
	if rank(a) >= rank(b) {
		return a
	}
	return b
}

func rank(s Status) int {
	switch s {
	case Breached:
		return 2
	case Warning:
		return 1
	default:
		return 0
	}
}

// DefaultPolicies 返回 §7.1 定义的四条默认 SLA 策略（P1~P4）。
func DefaultPolicies() []Policy {
	return []Policy{
		{Priority: "P1", ResponseMinutes: 15, ResolveMinutes: 240, PauseOnPending: true},
		{Priority: "P2", ResponseMinutes: 30, ResolveMinutes: 480, PauseOnPending: true},
		{Priority: "P3", ResponseMinutes: 120, ResolveMinutes: 1440, PauseOnPending: true},
		{Priority: "P4", ResponseMinutes: 480, ResolveMinutes: 4320, PauseOnPending: true},
	}
}
