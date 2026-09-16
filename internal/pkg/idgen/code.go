// Package idgen 提供业务编号生成：PREFIX-YYYYMMDD-NNNN（如 TKT-20260916-0001）。
//
// 并发安全：进程内按「前缀 + 日期」维护自增计数器（加锁）；
// 跨进程/重启的重复防护依靠唯一索引兜底（先查库内当日最大序号后 +1）。
package idgen

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 业务前缀常量。
const (
	PrefixTicket   = "TKT"
	PrefixIncident = "INC"
	PrefixProblem  = "PRB"
	PrefixChange   = "CHG"
)

const (
	dateLayout = "20060102"
	seqWidth   = 4
)

// Format 生成编号：PREFIX-YYYYMMDD-NNNN（seq 至少 4 位，不足左补 0）。
func Format(prefix string, date time.Time, seq int) string {
	if seq < 1 {
		seq = 1
	}
	return fmt.Sprintf("%s-%s-%0*d", prefix, date.Format(dateLayout), seqWidth, seq)
}

// ParseSeq 从编号解析「当日序号」。
//
// date 必须与编号中的日期一致，否则返回 false。用于「先查最大序号 +1」的实现。
func ParseSeq(code, prefix string, date time.Time) (int, bool) {
	want := prefix + "-" + date.Format(dateLayout) + "-"
	if !strings.HasPrefix(code, want) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(code, want))
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// Generator 是并发安全的编号生成器。
type Generator struct {
	mu       sync.Mutex
	counters map[string]int
}

// NewGenerator 创建一个空计数器的生成器。
func NewGenerator() *Generator {
	return &Generator{counters: make(map[string]int)}
}

// Next 返回下一个编号。
//
// existingMax 为该（前缀 + 日期）在库中已有的最大序号（无则传 0）；
// 生成器会取 max(内存计数, existingMax) 后 +1，避免重启后重复。
func (g *Generator) Next(prefix string, date time.Time, existingMax int) string {
	g.mu.Lock()
	defer g.mu.Unlock()

	key := counterKey(prefix, date)
	if existingMax > g.counters[key] {
		g.counters[key] = existingMax
	}
	g.counters[key]++
	seq := g.counters[key]

	return Format(prefix, date, seq)
}

func counterKey(prefix string, date time.Time) string {
	return prefix + "-" + date.Format(dateLayout)
}
