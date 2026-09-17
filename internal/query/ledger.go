package query

// Ledger-backed query engine: reads the normalized SQLite ledger only.
//
// The engine never touches source files, never runs discovery/parse/sync,
// never writes to the DB, and never imports source file formats — it only
// understands ledger rows. Refresh (source → ledger) lives in
// internal/refresh (pi) and internal/codex.SyncRollouts, and must run
// before Query in production (server/CLI call refresh.Refresh first).
//
// Only codex path resolution/containment and codex.Diagnostics (stored summary
// shape) are reused from the adapter; no adapter file I/O runs here.

import (
	"time"

	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
	"github.com/heihei0299/token-analyzer/internal/timerange"
)

type ledgerRequest struct {
	model        string
	sessionID    string
	physicalID   string
	requestCount int
	createdAt    int64
	tsText       string
	cwd          string
	input        float64
	output       float64
	cacheRead    float64
	cacheWrite   float64
	reasoning    float64
	cost         float64
}

func (r ledgerRequest) timestamp() string {
	if r.tsText != "" {
		return r.tsText
	}
	return time.Unix(r.createdAt, 0).UTC().Format(time.RFC3339)
}

func (r ledgerRequest) usage() domain.Usage {
	return domain.Usage{
		Input:       r.input,
		Output:      r.output,
		CacheRead:   r.cacheRead,
		CacheWrite:  r.cacheWrite,
		Reasoning:   r.reasoning,
		TotalTokens: r.input + r.cacheRead + r.output,
	}
}

// messageBounds 返回消息级时间过滤（秒）；会话级 TimeRange 不过滤请求行。
func messageBounds(f sessiondata.Filter) (since, until *int64) {
	tr := f.TimeRange
	if tr == nil || tr.Kind != timerange.KindMessage {
		return nil, nil
	}
	if tr.SinceMs != nil {
		v := *tr.SinceMs / 1000
		since = &v
	}
	if tr.UntilMs != nil {
		v := *tr.UntilMs / 1000
		until = &v
	}
	return since, until
}

// headerInRange 会话级 TimeRange 按 header 时间戳过滤；非法时间戳保留（与文件扫描一致）。
func headerInRange(headerTs string, f sessiondata.Filter) bool {
	tr := f.TimeRange
	if tr == nil || tr.Kind != timerange.KindSession {
		return true
	}
	ms, err := timerange.ParseUtcTimestamp(headerTs)
	if err != nil {
		return true
	}
	if tr.SinceMs != nil && ms < *tr.SinceMs {
		return false
	}
	if tr.UntilMs != nil && ms > *tr.UntilMs {
		return false
	}
	return true
}

func cwdKept(sessionCwd, filterCwd string) bool {
	if filterCwd == "" {
		return true
	}
	return sessiondata.NormalizeCwd(sessionCwd) == sessiondata.NormalizeCwd(filterCwd)
}

// hasRequestFilter 决定无匹配请求的会话是否列出（与 TS oracle 一致）：
// model/消息时间过滤会筛掉空会话，会话级时间只定会话作用域。
func hasRequestFilter(f sessiondata.Filter) bool {
	if f.Model != "" {
		return true
	}
	since, until := messageBounds(f)
	return since != nil || until != nil
}

func modelLabel(models map[string]bool) string {
	if len(models) == 0 {
		return "-"
	}
	if len(models) == 1 {
		for m := range models {
			return m
		}
	}
	return "mixed"
}

func sumInto(t *domain.Totals, r ledgerRequest) {
	t.Requests += r.requestCount
	t.Input += r.input
	t.Output += r.output
	t.CacheRead += r.cacheRead
	t.CacheWrite += r.cacheWrite
	t.Reasoning += r.reasoning
	t.Cost += r.cost
}
