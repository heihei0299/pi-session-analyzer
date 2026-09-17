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
	"database/sql"
	"strings"
	"time"

	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
	"github.com/heihei0299/token-analyzer/internal/timerange"
)

const requestColumns = `model, session_id, physical_rollout_id, created_at, timestamp_text, cwd, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, reasoning_tokens, COALESCE(CAST(total_cost_usd AS REAL), 0)`

func scanRequests(rows *sql.Rows) ([]ledgerRequest, error) {
	var out []ledgerRequest
	for rows.Next() {
		var r ledgerRequest
		if err := rows.Scan(&r.model, &r.sessionID, &r.physicalID, &r.createdAt, &r.tsText, &r.cwd,
			&r.input, &r.output, &r.cacheRead, &r.cacheWrite, &r.reasoning, &r.cost); err != nil {
			return nil, err
		}
		r.requestCount = 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// loadRequests 按源 + 消息级过滤加载请求行；sessionIDs 为空表示无会话作用域（空结果）。
func loadRequests(database *db.Database, appType, dataSource string, sessionIDs []string, physical bool, f sessiondata.Filter) ([]ledgerRequest, error) {
	if sessionIDs != nil && len(sessionIDs) == 0 {
		return nil, nil
	}
	since, until := messageBounds(f)
	var conds []string
	var args []any
	conds = append(conds, `app_type = ? AND data_source = ?`)
	args = append(args, appType, dataSource)
	if f.Model != "" {
		conds = append(conds, `model = ?`)
		args = append(args, f.Model)
	}
	if since != nil {
		conds = append(conds, `created_at >= ?`)
		args = append(args, *since)
	}
	if until != nil {
		conds = append(conds, `created_at <= ?`)
		args = append(args, *until)
	}
	if sessionIDs != nil {
		key := `session_id`
		if physical {
			key = `physical_rollout_id`
		}
		placeholders := make([]string, len(sessionIDs))
		for i, id := range sessionIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		conds = append(conds, key+` IN (`+strings.Join(placeholders, ",")+`)`)
	}
	rows, err := database.DB.Query(`SELECT `+requestColumns+` FROM proxy_request_logs WHERE `+
		strings.Join(conds, " AND ")+` ORDER BY created_at, request_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRequests(rows)
}

// rollupsAllowed describes the information that a daily rollup can safely answer.
// Rollups have no session or cwd identity, so those scopes remain raw-ledger only.
func rollupsAllowed(f sessiondata.Filter, v sessiondata.View) bool {
	if f.Cwd != "" || (f.TimeRange != nil && f.TimeRange.Kind == timerange.KindSession) {
		return false
	}
	switch v.Kind {
	case sessiondata.ViewTotals, sessiondata.ViewPeriod:
		return true
	case sessiondata.ViewGroups:
		return v.By == "" || v.By == domain.GroupByModel
	default:
		return false
	}
}

type rollupDateWindow struct {
	since        string
	until        string
	sincePartial bool
	untilPartial bool
}

func rollupDateWindowFor(f sessiondata.Filter) rollupDateWindow {
	var window rollupDateWindow
	tr := f.TimeRange
	if tr == nil || tr.Kind != timerange.KindMessage {
		return window
	}
	toDate := func(ms int64) string {
		return time.UnixMilli(ms).In(time.Local).Format("2006-01-02")
	}
	isStartOfDay := func(ms int64) bool {
		value := time.UnixMilli(ms).In(time.Local)
		return value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0
	}
	isEndOfDay := func(ms int64) bool {
		value := time.UnixMilli(ms).In(time.Local).Add(time.Millisecond)
		return value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0
	}
	if tr.SinceMs != nil {
		window.since = toDate(*tr.SinceMs)
		window.sincePartial = !isStartOfDay(*tr.SinceMs)
	}
	if tr.UntilMs != nil {
		window.until = toDate(*tr.UntilMs)
		window.untilPartial = !isEndOfDay(*tr.UntilMs)
	}
	return window
}

func rollupTimestamp(date string) (string, int64, bool) {
	// date is a local-calendar key; use UTC midnight only as a stable timestamp
	// because PeriodKey normalizes timestamps to UTC before extracting its key.
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "", 0, false
	}
	return date + "T00:00:00Z", t.Unix(), true
}

// rollupReasoningExpression keeps read-only Query compatible with databases
// created before reasoning_tokens was added to usage_daily_rollups.
func rollupReasoningExpression(database *db.Database) string {
	rows, err := database.DB.Query(`PRAGMA table_info(usage_daily_rollups)`)
	if err != nil {
		return "0"
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt interface{}
		var pk int
		if rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk) == nil && name == "reasoning_tokens" {
			return "reasoning_tokens"
		}
	}
	return "0"
}

// loadRollups loads persisted daily aggregates. A valid rollup is additive with
// raw rows: the prune writer removes the contributing raw rows (a cutoff can
// leave disjoint raw and rollup rows on the same calendar day).
func loadRollups(database *db.Database, appType string, f sessiondata.Filter) ([]ledgerRequest, error) {
	if f.Cwd != "" || (f.TimeRange != nil && f.TimeRange.Kind == timerange.KindSession) {
		return nil, nil
	}
	conds := []string{"app_type = ?"}
	args := []any{appType}
	if f.Model != "" {
		conds = append(conds, "model = ?")
		args = append(args, f.Model)
	}
	window := rollupDateWindowFor(f)
	if window.since != "" {
		op := ">="
		if window.sincePartial {
			op = ">"
		}
		conds = append(conds, "date "+op+" ?")
		args = append(args, window.since)
	}
	if window.until != "" {
		op := "<="
		if window.untilPartial {
			op = "<"
		}
		conds = append(conds, "date "+op+" ?")
		args = append(args, window.until)
	}
	reasoningExpr := rollupReasoningExpression(database)
	rows, err := database.DB.Query(`SELECT date, request_count, model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, `+reasoningExpr+`, CAST(COALESCE(total_cost_usd, '0') AS REAL) FROM usage_daily_rollups WHERE `+strings.Join(conds, " AND ")+` ORDER BY date, model`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ledgerRequest
	for rows.Next() {
		var date, model string
		var requestCount int64
		var input, output, cacheRead, cacheWrite, reasoning, cost float64
		if err := rows.Scan(&date, &requestCount, &model, &input, &output, &cacheRead, &cacheWrite, &reasoning, &cost); err != nil {
			return nil, err
		}
		tsText, createdAt, ok := rollupTimestamp(date)
		if !ok {
			continue
		}
		out = append(out, ledgerRequest{
			model:        model,
			createdAt:    createdAt,
			tsText:       tsText,
			input:        input,
			output:       output,
			cacheRead:    cacheRead,
			cacheWrite:   cacheWrite,
			reasoning:    reasoning,
			cost:         cost,
			requestCount: int(requestCount),
		})
	}
	return out, rows.Err()
}

const partialRollupCoverageWarning = "MessageTimeRange 含小时/分钟级边界；边界日 daily rollup 未计入，历史 raw 已 prune 时结果为 partial coverage"

func hasPartialRollupCoverage(database *db.Database, appType string, f sessiondata.Filter) (bool, error) {
	// ponytail: any boundary-day rollup is conservatively partial; exact
	// coverage needs per-event timestamps in the rollup schema.
	window := rollupDateWindowFor(f)
	if !window.sincePartial && !window.untilPartial {
		return false, nil
	}
	dates := make([]string, 0, 2)
	if window.sincePartial {
		dates = append(dates, window.since)
	}
	if window.untilPartial && window.until != window.since {
		dates = append(dates, window.until)
	}
	if len(dates) == 0 {
		return false, nil
	}
	conds := []string{"app_type = ?"}
	args := []any{appType}
	if f.Model != "" {
		conds = append(conds, "model = ?")
		args = append(args, f.Model)
	}
	placeholders := make([]string, len(dates))
	for i, date := range dates {
		placeholders[i] = "?"
		args = append(args, date)
	}
	conds = append(conds, "date IN ("+strings.Join(placeholders, ",")+")")
	var exists int
	if err := database.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM usage_daily_rollups WHERE `+strings.Join(conds, " AND ")+` LIMIT 1)`, args...).Scan(&exists); err != nil {
		return false, err
	}
	return exists == 1, nil
}

func markPartialRollupCoverage(result *sessiondata.QueryResult) {
	if result.Meta == nil {
		result.Meta = &sessiondata.QueryMeta{Sources: SupportedSources()}
	}
	result.Meta.CoverageStatus = "partial"
	result.Meta.Warnings = append(result.Meta.Warnings, partialRollupCoverageWarning)
}
