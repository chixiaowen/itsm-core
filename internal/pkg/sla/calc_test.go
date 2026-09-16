package sla

import (
	"testing"
	"time"
)

func tp(min int) time.Time {
	return time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC).Add(time.Duration(min) * time.Minute)
}

func ptr(t time.Time) *time.Time { return &t }

func TestDue_FourPriorities(t *testing.T) {
	start := tp(0)
	cases := []struct {
		p        Policy
		wantResp time.Duration
		wantRes  time.Duration
	}{
		{Policy{Priority: "P1", ResponseMinutes: 15, ResolveMinutes: 240}, 15 * time.Minute, 240 * time.Minute},
		{Policy{Priority: "P2", ResponseMinutes: 30, ResolveMinutes: 480}, 30 * time.Minute, 480 * time.Minute},
		{Policy{Priority: "P3", ResponseMinutes: 120, ResolveMinutes: 1440}, 120 * time.Minute, 1440 * time.Minute},
		{Policy{Priority: "P4", ResponseMinutes: 480, ResolveMinutes: 4320}, 480 * time.Minute, 4320 * time.Minute},
	}
	for _, c := range cases {
		if got := Due(start, c.p.ResponseMinutes); !got.Equal(start.Add(c.wantResp)) {
			t.Fatalf("%s 响应截止错误: got %v", c.p.Priority, got)
		}
		if got := Due(start, c.p.ResolveMinutes); !got.Equal(start.Add(c.wantRes)) {
			t.Fatalf("%s 解决截止错误: got %v", c.p.Priority, got)
		}
	}
}

func TestStatusOf_Boundaries(t *testing.T) {
	start := tp(0)
	const target = 100 // 100 分钟，阈值 20 分钟

	// 剩余 50 分钟（> 20%）-> normal
	if got := StatusOf(start, target, nil, start.Add(50*time.Minute), 0); got != Normal {
		t.Fatalf("期望 normal，实际 %s", got)
	}
	// 剩余恰好 20 分钟（== 20%）-> warning
	if got := StatusOf(start, target, nil, start.Add(80*time.Minute), 0); got != Warning {
		t.Fatalf("期望 warning，实际 %s", got)
	}
	// 剩余 10 分钟（< 20%）-> warning
	if got := StatusOf(start, target, nil, start.Add(90*time.Minute), 0); got != Warning {
		t.Fatalf("期望 warning，实际 %s", got)
	}
	// 已超期未解决 -> breached
	if got := StatusOf(start, target, nil, start.Add(101*time.Minute), 0); got != Breached {
		t.Fatalf("期望 breached，实际 %s", got)
	}
	// 达成且按期 -> normal
	if got := StatusOf(start, target, ptr(start.Add(99*time.Minute)), start.Add(150*time.Minute), 0); got != Normal {
		t.Fatalf("按期解决期望 normal，实际 %s", got)
	}
	// 达成但超期 -> breached
	if got := StatusOf(start, target, ptr(start.Add(101*time.Minute)), start.Add(150*time.Minute), 0); got != Breached {
		t.Fatalf("超期解决期望 breached，实际 %s", got)
	}
}

func TestStatusOf_PauseRebate(t *testing.T) {
	start := tp(0)
	const target = 100

	// 已暂停 30 分钟：now=start+120 分钟，回补后等价于现在只是 start+90 -> warning 而非 breached
	got := StatusOf(start, target, nil, start.Add(120*time.Minute), 30)
	if got != Warning {
		t.Fatalf("暂停回补后期望 warning，实际 %s", got)
	}

	// now=start+100 分钟，暂停 25 -> 有效已用 75，剩余 25 -> normal
	if got := StatusOf(start, target, nil, start.Add(100*time.Minute), 25); got != Normal {
		t.Fatalf("暂停回补后期望 normal，实际 %s", got)
	}
}

func TestElapsed(t *testing.T) {
	start := tp(0)
	// 无暂停
	if got := Elapsed(start, nil, start.Add(10*time.Minute), 0); got != 10*time.Minute {
		t.Fatalf("期望 10m，实际 %v", got)
	}
	// 暂停回补
	if got := Elapsed(start, ptr(start.Add(60*time.Minute)), start, 20); got != 40*time.Minute {
		t.Fatalf("期望 40m，实际 %v", got)
	}
	// 暂停超过总时长 -> 归零
	if got := Elapsed(start, nil, start.Add(10*time.Minute), 999); got != 0 {
		t.Fatalf("暂停超时应归零，实际 %v", got)
	}
}

func TestEvaluate(t *testing.T) {
	start := tp(0)
	now := start.Add(10 * time.Minute)
	p := Policy{Priority: "P1", ResponseMinutes: 15, ResolveMinutes: 240, PauseOnPending: true}

	v := Evaluate(p, start, now, nil, nil, 0)
	if v.ResponseDueAt == nil || !v.ResponseDueAt.Equal(start.Add(15*time.Minute)) {
		t.Fatalf("响应截止错误: %v", v.ResponseDueAt)
	}
	if v.ResolveDueAt == nil || !v.ResolveDueAt.Equal(start.Add(240*time.Minute)) {
		t.Fatalf("解决截止错误: %v", v.ResolveDueAt)
	}
	if v.ResponseStatus != Normal || v.ResolveStatus != Normal || v.SLAStatus != Normal {
		t.Fatalf("期望全部 normal，实际 %+v", v)
	}
	if v.Remaining != 230*time.Minute {
		t.Fatalf("剩余时间错误: %v", v.Remaining)
	}

	// 响应已超期（25 分钟未响应）-> 综合取 breached
	v2 := Evaluate(p, start, start.Add(25*time.Minute), nil, nil, 0)
	if v2.ResponseStatus != Breached || v2.SLAStatus != Breached {
		t.Fatalf("期望响应 breached 且综合 breached，实际 %+v", v2)
	}

	// 响应已达成按期，解决未达成且剩余超期点后 -> breached 综合
	fr := start.Add(5 * time.Minute)
	v3 := Evaluate(p, start, start.Add(300*time.Minute), &fr, nil, 0)
	if v3.ResponseStatus != Normal || v3.ResolveStatus != Breached || v3.SLAStatus != Breached {
		t.Fatalf("期望综合 breached，实际 %+v", v3)
	}

	// 解决已达成，remaining 归零
	rs := start.Add(100 * time.Minute)
	v4 := Evaluate(p, start, start.Add(1000*time.Minute), &fr, &rs, 0)
	if v4.Remaining != 0 {
		t.Fatalf("已达成时 remaining 应为 0，实际 %d", v4.Remaining)
	}
}

func TestWorseRank(t *testing.T) {
	if worse(Warning, Breached) != Breached {
		t.Fatalf("breached 更严重")
	}
	if worse(Normal, Warning) != Warning {
		t.Fatalf("warning 更严重")
	}
	if worse(Breached, Normal) != Breached {
		t.Fatalf("breached 更严重")
	}
	if worse(Normal, Normal) != Normal {
		t.Fatalf("相等应返回自身")
	}
}

func TestDefaultPolicies(t *testing.T) {
	ps := DefaultPolicies()
	if len(ps) != 4 {
		t.Fatalf("期望 4 条默认策略，实际 %d", len(ps))
	}
	if ps[0].Priority != "P1" || ps[0].ResponseMinutes != 15 || ps[0].ResolveMinutes != 240 || !ps[0].PauseOnPending {
		t.Fatalf("P1 策略不符：%+v", ps[0])
	}
	if ps[3].Priority != "P4" || ps[3].ResponseMinutes != 480 || ps[3].ResolveMinutes != 4320 {
		t.Fatalf("P4 策略不符：%+v", ps[3])
	}
}
