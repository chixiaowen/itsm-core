package idgen

import (
	"sync"
	"testing"
	"time"
)

var refDate = time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)

func TestFormat(t *testing.T) {
	cases := []struct {
		prefix string
		seq    int
		want   string
	}{
		{PrefixTicket, 1, "TKT-20260916-0001"},
		{PrefixIncident, 1, "INC-20260916-0001"},
		{PrefixProblem, 42, "PRB-20260916-0042"},
		{PrefixChange, 9999, "CHG-20260916-9999"},
		{PrefixTicket, 0, "TKT-20260916-0001"}, // 非法序号归 1
		{PrefixTicket, 12345, "TKT-20260916-12345"},
	}
	for _, c := range cases {
		if got := Format(c.prefix, refDate, c.seq); got != c.want {
			t.Fatalf("Format(%s,%d)=%s，期望 %s", c.prefix, c.seq, got, c.want)
		}
	}
}

func TestParseSeq(t *testing.T) {
	if n, ok := ParseSeq("TKT-20260916-0007", PrefixTicket, refDate); !ok || n != 7 {
		t.Fatalf("期望解析出 7，实际 %d ok=%v", n, ok)
	}
	// 前缀不匹配
	if _, ok := ParseSeq("INC-20260916-0007", PrefixTicket, refDate); ok {
		t.Fatalf("前缀不匹配不应解析成功")
	}
	// 日期不匹配
	if _, ok := ParseSeq("TKT-20260915-0007", PrefixTicket, refDate); ok {
		t.Fatalf("日期不匹配不应解析成功")
	}
	// 序号非法
	if _, ok := ParseSeq("TKT-20260916-abcd", PrefixTicket, refDate); ok {
		t.Fatalf("非法序号不应解析成功")
	}
	if _, ok := ParseSeq("TKT-20260916-0000", PrefixTicket, refDate); ok {
		t.Fatalf("序号 0 不应解析成功")
	}
}

func TestGenerator_Sequential(t *testing.T) {
	g := NewGenerator()
	if got := g.Next(PrefixTicket, refDate, 0); got != "TKT-20260916-0001" {
		t.Fatalf("期望 0001，实际 %s", got)
	}
	if got := g.Next(PrefixTicket, refDate, 0); got != "TKT-20260916-0002" {
		t.Fatalf("期望 0002，实际 %s", got)
	}
	// 不同前缀互不影响
	if got := g.Next(PrefixIncident, refDate, 0); got != "INC-20260916-0001" {
		t.Fatalf("期望 INC 0001，实际 %s", got)
	}
}

func TestGenerator_SeedFromExisting(t *testing.T) {
	g := NewGenerator()
	// 库中已有 0009，则下一个应为 0010
	if got := g.Next(PrefixTicket, refDate, 9); got != "TKT-20260916-0010" {
		t.Fatalf("期望 0010，实际 %s", got)
	}
	// 内存计数已为 10，传入更小的 existingMax 不应回退
	if got := g.Next(PrefixTicket, refDate, 3); got != "TKT-20260916-0011" {
		t.Fatalf("期望 0011，实际 %s", got)
	}
}

func TestGenerator_ConcurrentUnique(t *testing.T) {
	g := NewGenerator()
	const n = 200

	var wg sync.WaitGroup
	results := make([]string, n)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			results[idx] = g.Next(PrefixChange, refDate, 0)
		}(i)
	}
	close(start)
	wg.Wait()

	seen := make(map[string]bool, n)
	for _, code := range results {
		if seen[code] {
			t.Fatalf("并发生成出现重复编号: %s", code)
		}
		seen[code] = true
		if _, ok := ParseSeq(code, PrefixChange, refDate); !ok {
			t.Fatalf("并发生成编号格式非法: %s", code)
		}
	}
	if len(seen) != n {
		t.Fatalf("期望 %d 个唯一编号，实际 %d", n, len(seen))
	}
}
