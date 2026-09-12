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
	"sort"
	"strings"
	"time"

	"github.com/heihei0299/token-analyzer/internal/codex"
	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
	"github.com/heihei0299/token-analyzer/internal/timerange"
)

// ledgerRequest 是一条已提交的 normalized usage 记录（raw 消息或 daily rollup）。
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

// ---------- Pi ----------

type piSessionMeta struct {
	sessionID       string
	headerTs        string
	cwd             string
	fileName        string
	displayName     string
	isTask          bool
	parentSessionID string
}

func loadPiMetas(database *db.Database) ([]piSessionMeta, error) {
	rows, err := database.DB.Query(`SELECT session_id, header_ts, cwd, file_name, display_name, is_task, parent_session_id FROM pi_sessions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []piSessionMeta
	for rows.Next() {
		var m piSessionMeta
		var isTask int
		var parent sql.NullString
		if err := rows.Scan(&m.sessionID, &m.headerTs, &m.cwd, &m.fileName, &m.displayName, &isTask, &parent); err != nil {
			return nil, err
		}
		m.isTask = isTask == 1
		m.parentSessionID = parent.String
		out = append(out, m)
	}
	return out, rows.Err()
}

// scopePiSessions 会话作用域：detail 白名单 → cwd 会话级 → 会话级时间。
func scopePiSessions(metas []piSessionMeta, f sessiondata.Filter, whitelist map[string]bool) []piSessionMeta {
	var out []piSessionMeta
	for _, m := range metas {
		if whitelist != nil && !whitelist[m.sessionID] {
			continue
		}
		if !cwdKept(m.cwd, f.Cwd) {
			continue
		}
		if !headerInRange(m.headerTs, f) {
			continue
		}
		out = append(out, m)
	}
	return out
}

func piSessionIDs(scoped []piSessionMeta) []string {
	ids := make([]string, 0, len(scoped))
	for _, m := range scoped {
		ids = append(ids, m.sessionID)
	}
	return ids
}

func sessionTotals(reqs []ledgerRequest) domain.Totals {
	tot := domain.EmptyTotals()
	for _, r := range reqs {
		sumInto(&tot, r)
	}
	domain.FinalizeTotals(&tot)
	return tot
}

func buildGroups(reqs []ledgerRequest, by domain.GroupBy, unpriced bool) []domain.GroupRow {
	byModel := by == domain.GroupByModel || by == domain.GroupByModelCwd
	byCwd := by == domain.GroupByCwd || by == domain.GroupByModelCwd
	type key struct{ model, cwd string }
	groups := map[key]*domain.GroupRow{}
	var order []key
	for _, r := range reqs {
		var k key
		row := &domain.GroupRow{Totals: domain.EmptyTotals()}
		if byModel {
			k.model = r.model
			row.Model = r.model
		}
		if byCwd {
			k.cwd = sessiondata.NormalizeCwd(r.cwd)
			row.Cwd = k.cwd
		}
		g, ok := groups[k]
		if !ok {
			g = row
			groups[k] = g
			order = append(order, k)
		}
		sumInto(&g.Totals, r)
	}
	rows := make([]domain.GroupRow, 0, len(order))
	for _, k := range order {
		g := groups[k]
		if unpriced {
			g.CostStatus = "unpriced"
		}
		domain.FinalizeTotals(&g.Totals)
		rows = append(rows, *g)
	}
	return rows
}

func buildPeriod(reqs []ledgerRequest, p domain.Period, unpriced bool) []domain.PeriodRow {
	groups := map[string]*domain.Totals{}
	var order []string
	for _, r := range reqs {
		k, err := sessiondata.PeriodKey(r.timestamp(), p)
		if err != nil {
			continue
		}
		g, ok := groups[k]
		if !ok {
			g = &domain.Totals{}
			*g = domain.EmptyTotals()
			groups[k] = g
			order = append(order, k)
		}
		sumInto(g, r)
	}
	sort.Strings(order)
	rows := make([]domain.PeriodRow, 0, len(order))
	for _, k := range order {
		g := groups[k]
		if unpriced {
			g.CostStatus = "unpriced"
		}
		domain.FinalizeTotals(g)
		rows = append(rows, domain.PeriodRow{Period: k, Totals: *g})
	}
	return rows
}

func buildPiSessionRows(scoped []piSessionMeta, bySession map[string][]ledgerRequest, source string) []domain.SessionRow {
	rows := make([]domain.SessionRow, 0, len(scoped))
	for _, m := range scoped {
		reqs := bySession[m.sessionID]
		tot := sessionTotals(reqs)
		models := map[string]bool{}
		for _, r := range reqs {
			models[r.model] = true
		}
		rows = append(rows, domain.SessionRow{
			Totals:          tot,
			SessionId:       m.sessionID,
			Timestamp:       m.headerTs,
			Cwd:             m.cwd,
			Model:           modelLabel(models),
			FileName:        m.fileName,
			DisplayName:     m.displayName,
			CwdNorm:         sessiondata.NormalizeCwd(m.cwd),
			IsTask:          m.isTask,
			ParentSessionId: m.parentSessionID,
			Source:          source,
		})
	}
	return rows
}

func groupBySession(reqs []ledgerRequest, key func(ledgerRequest) string) map[string][]ledgerRequest {
	out := map[string][]ledgerRequest{}
	for _, r := range reqs {
		k := key(r)
		out[k] = append(out[k], r)
	}
	return out
}

func queryPi(database *db.Database, dir string, f sessiondata.Filter, v sessiondata.View, whitelist map[string]bool) (*sessiondata.QueryResult, error) {
	metas, err := loadPiMetas(database)
	if err != nil {
		return nil, err
	}
	if v.Kind == sessiondata.ViewMeta {
		return &sessiondata.QueryResult{Window: "meta", Meta: piMetaOf(dir, metas)}, nil
	}
	scoped := scopePiSessions(metas, f, whitelist)
	reqs, err := loadRequests(database, "pi", "pi_session", piSessionIDs(scoped), false, f)
	if err != nil {
		return nil, err
	}
	queryReqs := reqs
	partialCoverage := false
	if rollupsAllowed(f, v) {
		rollups, err := loadRollups(database, "pi", f)
		if err != nil {
			return nil, err
		}
		queryReqs = append(queryReqs, rollups...)
		partialCoverage, err = hasPartialRollupCoverage(database, "pi", f)
		if err != nil {
			return nil, err
		}
	}
	bySession := groupBySession(reqs, func(r ledgerRequest) string { return r.sessionID })
	if hasRequestFilter(f) {
		kept := scoped[:0]
		for _, m := range scoped {
			if len(bySession[m.sessionID]) > 0 {
				kept = append(kept, m)
			}
		}
		scoped = kept
	}
	switch v.Kind {
	case sessiondata.ViewTotals:
		tot := sessionTotals(queryReqs)
		result := &sessiondata.QueryResult{Window: "totals", Totals: &tot}
		if partialCoverage {
			markPartialRollupCoverage(result)
		}
		return result, nil
	case sessiondata.ViewGroups:
		result := &sessiondata.QueryResult{Window: "totals", By: v.By, Rows: buildGroups(queryReqs, v.By, false)}
		if partialCoverage {
			markPartialRollupCoverage(result)
		}
		return result, nil
	case sessiondata.ViewPeriod:
		result := &sessiondata.QueryResult{Window: "totals", Period: v.Period, Rows: buildPeriod(queryReqs, v.Period, false)}
		if partialCoverage {
			markPartialRollupCoverage(result)
		}
		return result, nil
	case sessiondata.ViewSessions:
		rows := buildPiSessionRows(scoped, bySession, "")
		tot := domain.EmptyTotals()
		for i := range rows {
			tot.Requests += rows[i].Requests
			tot.Input += rows[i].Input
			tot.Output += rows[i].Output
			tot.CacheRead += rows[i].CacheRead
			tot.CacheWrite += rows[i].CacheWrite
			tot.Reasoning += rows[i].Reasoning
			tot.Cost += rows[i].Cost
		}
		domain.FinalizeTotals(&tot)
		total := len(rows)
		if v.SortKey != "" {
			sessiondata.SortSessionRows(rows, v.SortKey, v.SortDir == "desc")
		}
		pageRows := paginateSessionRows(rows, v)
		return &sessiondata.QueryResult{Window: "sessions", Rows: pageRows, Total: total, Page: v.Page, Size: v.Size, Totals: &tot}, nil
	case sessiondata.ViewRequests:
		rows := buildPiRequestRows(scoped, bySession)
		total := len(rows)
		if v.SortKey != "" {
			sessiondata.SortRequestRows(rows, v.SortKey, v.SortDir == "desc")
		}
		pageRows := paginateRequestRows(rows, v)
		return &sessiondata.QueryResult{Window: "requests", Rows: pageRows, Total: total, Page: v.Page, Size: v.Size}, nil
	}
	return nil, errUnknownView(string(v.Kind))
}

func buildPiRequestRows(scoped []piSessionMeta, bySession map[string][]ledgerRequest) []domain.RequestRow {
	rows := make([]domain.RequestRow, 0)
	nameBySession := map[string]string{}
	for _, m := range scoped {
		nameBySession[m.sessionID] = m.displayName
	}
	for _, m := range scoped {
		for _, r := range bySession[m.sessionID] {
			tot := domain.EmptyTotals()
			sumInto(&tot, r)
			domain.FinalizeTotals(&tot)
			rows = append(rows, domain.RequestRow{
				Totals:      tot,
				SessionId:   m.sessionID,
				Timestamp:   r.timestamp(),
				Model:       r.model,
				DisplayName: nameBySession[m.sessionID],
			})
		}
	}
	return rows
}

func piMetaOf(dir string, metas []piSessionMeta) *sessiondata.QueryMeta {
	var minTs, maxTs *string
	for _, m := range metas {
		if _, err := timerange.ParseUtcTimestamp(m.headerTs); err != nil {
			continue
		}
		if minTs == nil || m.headerTs < *minTs {
			v := m.headerTs
			minTs = &v
		}
		if maxTs == nil || m.headerTs > *maxTs {
			v := m.headerTs
			maxTs = &v
		}
	}
	return &sessiondata.QueryMeta{
		Dir:          dir,
		SessionCount: len(metas),
		DataRange:    sessiondata.DataRangeValue{Since: minTs, Until: maxTs},
		Sources:      SupportedSources(),
	}
}

func paginateSessionRows(rows []domain.SessionRow, v sessiondata.View) []domain.SessionRow {
	if v.Page <= 0 || v.Size <= 0 {
		if rows == nil {
			return make([]domain.SessionRow, 0)
		}
		return rows
	}
	start := (v.Page - 1) * v.Size
	if start > len(rows) {
		start = len(rows)
	}
	end := start + v.Size
	if end > len(rows) {
		end = len(rows)
	}
	out := rows[start:end]
	if out == nil {
		return make([]domain.SessionRow, 0)
	}
	return out
}

func paginateRequestRows(rows []domain.RequestRow, v sessiondata.View) []domain.RequestRow {
	if v.Page <= 0 || v.Size <= 0 {
		if rows == nil {
			return make([]domain.RequestRow, 0)
		}
		return rows
	}
	start := (v.Page - 1) * v.Size
	if start > len(rows) {
		start = len(rows)
	}
	end := start + v.Size
	if end > len(rows) {
		end = len(rows)
	}
	out := rows[start:end]
	if out == nil {
		return make([]domain.RequestRow, 0)
	}
	return out
}

// QueryPiDetail 会话详情（Pi 专属）：主会话 + parentSessionId 指向它的子会话。
func queryPiDetail(database *db.Database, sessionID string) (*sessiondata.SessionDetailResult, error) {
	metas, err := loadPiMetas(database)
	if err != nil {
		return nil, err
	}
	var parent *piSessionMeta
	var children []piSessionMeta
	for i := range metas {
		if metas[i].sessionID == sessionID {
			v := metas[i]
			parent = &v
		} else if metas[i].parentSessionID == sessionID {
			children = append(children, metas[i])
		}
	}
	if parent == nil {
		return nil, errSessionNotFound(sessionID)
	}
	ids := []string{parent.sessionID}
	for _, c := range children {
		ids = append(ids, c.sessionID)
	}
	reqs, err := loadRequests(database, "pi", "pi_session", ids, false, sessiondata.Filter{})
	if err != nil {
		return nil, err
	}
	bySession := groupBySession(reqs, func(r ledgerRequest) string { return r.sessionID })
	mainTotals := sessionTotals(bySession[parent.sessionID])
	mergedTotals := sessionTotals(reqs)
	parentRows := buildPiSessionRows([]piSessionMeta{*parent}, bySession, "")
	childRows := buildPiSessionRows(children, bySession, "")
	nameOf := map[string]string{parent.sessionID: parent.displayName}
	for _, c := range children {
		nameOf[c.sessionID] = c.displayName
	}
	childIDs := map[string]bool{}
	for _, c := range children {
		childIDs[c.sessionID] = true
	}
	var requests []domain.RequestRow
	for _, r := range bySession[parent.sessionID] {
		tot := domain.EmptyTotals()
		sumInto(&tot, r)
		domain.FinalizeTotals(&tot)
		requests = append(requests, domain.RequestRow{
			Totals: tot, SessionId: parent.sessionID, Timestamp: r.timestamp(),
			Model: r.model, DisplayName: nameOf[parent.sessionID],
			Source: "main", SourceSessionId: parent.sessionID,
		})
	}
	for _, c := range children {
		for _, r := range bySession[c.sessionID] {
			tot := domain.EmptyTotals()
			sumInto(&tot, r)
			domain.FinalizeTotals(&tot)
			requests = append(requests, domain.RequestRow{
				Totals: tot, SessionId: c.sessionID, Timestamp: r.timestamp(),
				Model: r.model, DisplayName: nameOf[c.sessionID],
				Source: "child", SourceSessionId: c.sessionID,
			})
		}
	}
	sessiondata.SortRequestRows(requests, "timestamp", false)
	if requests == nil {
		requests = make([]domain.RequestRow, 0)
	}
	if childRows == nil {
		childRows = make([]domain.SessionRow, 0)
	}
	return &sessiondata.SessionDetailResult{
		Session:  parentRows[0],
		Children: childRows,
		Totals: sessiondata.SessionDetailTotals{
			Main: mainTotals, Merged: mergedTotals, ChildrenCount: len(children),
		},
		Requests: requests,
		Meta:     sessiondata.SessionDetailMeta{HasChildren: len(children) > 0},
	}, nil
}

// ---------- Codex ----------

type codexSessionMeta struct {
	physicalID     string
	sessionID      string
	threadID       string
	tsText         string
	cwd            string
	filePath       string
	fileName       string
	parentThreadID string
}

func (m codexSessionMeta) id() string {
	if m.threadID != "" {
		return m.threadID
	}
	if m.sessionID != "" {
		return m.sessionID
	}
	return m.physicalID
}

func loadCodexMetas(database *db.Database, home string) ([]codexSessionMeta, error) {
	rows, err := database.DB.Query(`SELECT physical_id, session_id, thread_id, timestamp_text, cwd, file_path, file_name, parent_thread_id FROM source_sessions WHERE data_source = 'codex'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []codexSessionMeta
	for rows.Next() {
		var m codexSessionMeta
		if err := rows.Scan(&m.physicalID, &m.sessionID, &m.threadID, &m.tsText, &m.cwd, &m.filePath, &m.fileName, &m.parentThreadID); err != nil {
			return nil, err
		}
		// 作用域归属当前 home：换目录查询不复用旧账本行。
		if !codex.PathWithin(home, m.filePath) {
			continue
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].filePath < out[j].filePath })
	return out, nil
}

// codexUsageSet 返回有 durable usage 行的 physical 集合。
// 只有累计快照、没有 usage record 的 rollout 只产生 diagnostics，不进入 sessions/meta。
func codexUsageSet(database *db.Database, physicalIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(physicalIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(physicalIDs))
	args := make([]any, 0, len(physicalIDs)+1)
	args = append(args, "codex")
	for i, id := range physicalIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	rows, err := database.DB.Query(`SELECT DISTINCT physical_rollout_id FROM proxy_request_logs WHERE data_source = ? AND physical_rollout_id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// countedCodexMetas 只保留有 usage 的 physical（sessions/meta 口径）。
func countedCodexMetas(database *db.Database, metas []codexSessionMeta) ([]codexSessionMeta, error) {
	ids := make([]string, 0, len(metas))
	for _, m := range metas {
		ids = append(ids, m.physicalID)
	}
	used, err := codexUsageSet(database, ids)
	if err != nil {
		return nil, err
	}
	var out []codexSessionMeta
	for _, m := range metas {
		if used[m.physicalID] {
			out = append(out, m)
		}
	}
	return out, nil
}

func scopeCodexSessions(metas []codexSessionMeta, f sessiondata.Filter) []codexSessionMeta {
	var out []codexSessionMeta
	for _, m := range metas {
		if !cwdKept(m.cwd, f.Cwd) {
			continue
		}
		if !headerInRange(m.tsText, f) {
			continue
		}
		out = append(out, m)
	}
	return out
}

func codexPhysicalIDs(scoped []codexSessionMeta) []string {
	ids := make([]string, 0, len(scoped))
	for _, m := range scoped {
		ids = append(ids, m.physicalID)
	}
	return ids
}

// loadCodexDiagnostics 经 adapter 取数：query 不直接读诊断表结构。
func loadCodexDiagnostics(database *db.Database, home string) codex.Diagnostics {
	return codex.LoadDiagnostics(database, home)
}

func buildCodexSessionRows(scoped []codexSessionMeta, byPhysical map[string][]ledgerRequest) []domain.SessionRow {
	rows := make([]domain.SessionRow, 0, len(scoped))
	for _, m := range scoped {
		reqs := byPhysical[m.physicalID]
		tot := sessionTotals(reqs)
		tot.CostStatus = "unpriced"
		models := map[string]bool{}
		for _, r := range reqs {
			models[r.model] = true
		}
		ts := m.tsText
		cwd := m.cwd
		if len(reqs) > 0 {
			if ts == "" {
				ts = reqs[0].timestamp()
			}
			if cwd == "" {
				cwd = reqs[0].cwd
			}
		}
		rows = append(rows, domain.SessionRow{
			Totals:          tot,
			SessionId:       m.id(),
			Timestamp:       ts,
			Cwd:             cwd,
			Model:           modelLabel(models),
			FileName:        m.fileName,
			DisplayName:     sessiondata.DisplayNameOf(m.fileName, ""),
			CwdNorm:         sessiondata.NormalizeCwd(cwd),
			ParentSessionId: m.parentThreadID,
			Source:          "codex",
		})
	}
	return rows
}

func buildCodexRequestRows(scoped []codexSessionMeta, byPhysical map[string][]ledgerRequest) []domain.RequestRow {
	rows := make([]domain.RequestRow, 0)
	byID := map[string]codexSessionMeta{}
	for _, m := range scoped {
		byID[m.physicalID] = m
	}
	for _, m := range scoped {
		for _, r := range byPhysical[m.physicalID] {
			tot := domain.EmptyTotals()
			sumInto(&tot, r)
			domain.FinalizeTotals(&tot)
			rows = append(rows, domain.RequestRow{
				Totals:      tot,
				SessionId:   m.id(),
				Timestamp:   r.timestamp(),
				Model:       r.model,
				DisplayName: sessiondata.DisplayNameOf(m.fileName, ""),
			})
		}
	}
	return rows
}

func queryCodex(database *db.Database, home string, f sessiondata.Filter, v sessiondata.View) (*sessiondata.QueryResult, error) {
	metas, err := loadCodexMetas(database, home)
	if err != nil {
		return nil, err
	}
	metas, err = countedCodexMetas(database, metas)
	if err != nil {
		return nil, err
	}
	diagnostics := loadCodexDiagnostics(database, home)
	if v.Kind == sessiondata.ViewMeta {
		meta := codexMetaOf(home, metas, diagnostics)
		return &sessiondata.QueryResult{Window: "meta", Meta: meta}, nil
	}
	scoped := scopeCodexSessions(metas, f)
	reqs, err := loadRequests(database, "codex", "codex", codexPhysicalIDs(scoped), true, f)
	if err != nil {
		return nil, err
	}
	queryReqs := reqs
	partialCoverage := false
	if rollupsAllowed(f, v) {
		rollups, err := loadRollups(database, "codex", f)
		if err != nil {
			return nil, err
		}
		queryReqs = append(queryReqs, rollups...)
		partialCoverage, err = hasPartialRollupCoverage(database, "codex", f)
		if err != nil {
			return nil, err
		}
	}
	byPhysical := groupBySession(reqs, func(r ledgerRequest) string { return r.physicalID })
	if hasRequestFilter(f) {
		kept := scoped[:0]
		for _, m := range scoped {
			if len(byPhysical[m.physicalID]) > 0 {
				kept = append(kept, m)
			}
		}
		scoped = kept
	}
	contributed := len(queryReqs) > 0
	if !contributed && f.TimeRange != nil && f.TimeRange.Kind == timerange.KindSession && len(scoped) > 0 {
		contributed = true
	}
	markUnpriced := func(t *domain.Totals) {
		if contributed {
			t.CostStatus = "unpriced"
		}
	}
	switch v.Kind {
	case sessiondata.ViewTotals:
		tot := sessionTotals(queryReqs)
		markUnpriced(&tot)
		res := &sessiondata.QueryResult{Window: "totals", Totals: &tot}
		attachCodexDiagnostics(res, home, metas, diagnostics)
		if partialCoverage {
			markPartialRollupCoverage(res)
		}
		return res, nil
	case sessiondata.ViewGroups:
		rows := buildGroups(queryReqs, v.By, contributed)
		res := &sessiondata.QueryResult{Window: "totals", By: v.By, Rows: rows}
		attachCodexDiagnostics(res, home, metas, diagnostics)
		if partialCoverage {
			markPartialRollupCoverage(res)
		}
		return res, nil
	case sessiondata.ViewPeriod:
		rows := buildPeriod(queryReqs, v.Period, contributed)
		res := &sessiondata.QueryResult{Window: "totals", Period: v.Period, Rows: rows}
		attachCodexDiagnostics(res, home, metas, diagnostics)
		if partialCoverage {
			markPartialRollupCoverage(res)
		}
		return res, nil
	case sessiondata.ViewSessions:
		rows := buildCodexSessionRows(scoped, byPhysical)
		tot := domain.EmptyTotals()
		for i := range rows {
			tot.Requests += rows[i].Requests
			tot.Input += rows[i].Input
			tot.Output += rows[i].Output
			tot.CacheRead += rows[i].CacheRead
			tot.CacheWrite += rows[i].CacheWrite
			tot.Reasoning += rows[i].Reasoning
			tot.Cost += rows[i].Cost
		}
		domain.FinalizeTotals(&tot)
		markUnpriced(&tot)
		total := len(rows)
		if v.SortKey != "" {
			sessiondata.SortSessionRows(rows, v.SortKey, v.SortDir == "desc")
		}
		pageRows := paginateSessionRows(rows, v)
		res := &sessiondata.QueryResult{Window: "sessions", Rows: pageRows, Total: total, Page: v.Page, Size: v.Size, Totals: &tot}
		attachCodexDiagnostics(res, home, metas, diagnostics)
		return res, nil
	}
	return nil, errUnknownView(string(v.Kind))
}

func codexMetaOf(home string, metas []codexSessionMeta, diagnostics codex.Diagnostics) *sessiondata.QueryMeta {
	var minTs, maxTs *string
	for _, m := range metas {
		if _, err := timerange.ParseUtcTimestamp(m.tsText); err != nil {
			continue
		}
		if minTs == nil || m.tsText < *minTs {
			v := m.tsText
			minTs = &v
		}
		if maxTs == nil || m.tsText > *maxTs {
			v := m.tsText
			maxTs = &v
		}
	}
	return &sessiondata.QueryMeta{
		Dir:                home,
		SessionCount:       len(metas),
		DataRange:          sessiondata.DataRangeValue{Since: minTs, Until: maxTs},
		Sources:            SupportedSources(),
		Warnings:           diagnostics.Warnings,
		UncountedSnapshots: diagnostics.UncountedSnapshots,
	}
}

// attachCodexDiagnostics 给非 meta 视图补 meta（会话计数/范围/覆盖率诊断），
// 与旧回绕路径的 attachMeta 行为一致。
func attachCodexDiagnostics(res *sessiondata.QueryResult, home string, metas []codexSessionMeta, diagnostics codex.Diagnostics) {
	res.Meta = codexMetaOf(home, metas, diagnostics)
}

// ---------- All ----------

// queryAll 只是两侧 normalized 记录的组合查询，不存在独立的内存 merge 统计实现。
func queryAll(database *db.Database, cfg Config, f sessiondata.Filter, v sessiondata.View) (*sessiondata.QueryResult, error) {
	home := codex.ResolveHome(cfg.CodexDir)
	piMetas, err := loadPiMetas(database)
	if err != nil {
		return nil, err
	}
	codexMetas, err := loadCodexMetas(database, home)
	if err != nil {
		return nil, err
	}
	codexMetas, err = countedCodexMetas(database, codexMetas)
	if err != nil {
		return nil, err
	}
	diagnostics := loadCodexDiagnostics(database, home)
	if v.Kind == sessiondata.ViewMeta {
		return &sessiondata.QueryResult{Window: "meta", Meta: allMetaOf(cfg.PiDir, piMetas, codexMetas, diagnostics)}, nil
	}
	piScoped := scopePiSessions(piMetas, f, nil)
	piReqs, err := loadRequests(database, "pi", "pi_session", piSessionIDs(piScoped), false, f)
	if err != nil {
		return nil, err
	}
	piQueryReqs := piReqs
	piPartialCoverage := false
	if rollupsAllowed(f, v) {
		rollups, err := loadRollups(database, "pi", f)
		if err != nil {
			return nil, err
		}
		piQueryReqs = append(piQueryReqs, rollups...)
		piPartialCoverage, err = hasPartialRollupCoverage(database, "pi", f)
		if err != nil {
			return nil, err
		}
	}
	piBySession := groupBySession(piReqs, func(r ledgerRequest) string { return r.sessionID })
	if hasRequestFilter(f) {
		kept := piScoped[:0]
		for _, m := range piScoped {
			if len(piBySession[m.sessionID]) > 0 {
				kept = append(kept, m)
			}
		}
		piScoped = kept
	}
	codexScoped := scopeCodexSessions(codexMetas, f)
	codexReqs, err := loadRequests(database, "codex", "codex", codexPhysicalIDs(codexScoped), true, f)
	if err != nil {
		return nil, err
	}
	codexQueryReqs := codexReqs
	codexPartialCoverage := false
	if rollupsAllowed(f, v) {
		rollups, err := loadRollups(database, "codex", f)
		if err != nil {
			return nil, err
		}
		codexQueryReqs = append(codexQueryReqs, rollups...)
		codexPartialCoverage, err = hasPartialRollupCoverage(database, "codex", f)
		if err != nil {
			return nil, err
		}
	}
	codexByPhysical := groupBySession(codexReqs, func(r ledgerRequest) string { return r.physicalID })
	if hasRequestFilter(f) {
		kept := codexScoped[:0]
		for _, m := range codexScoped {
			if len(codexByPhysical[m.physicalID]) > 0 {
				kept = append(kept, m)
			}
		}
		codexScoped = kept
	}
	codexContributed := len(codexQueryReqs) > 0
	switch v.Kind {
	case sessiondata.ViewTotals:
		tot := sessionTotals(piQueryReqs)
		for _, r := range codexQueryReqs {
			sumInto(&tot, r)
		}
		domain.FinalizeTotals(&tot)
		// All 保留已知 Pi 成本，同时标注含未定价源。
		if codexContributed {
			tot.CostStatus = "unpriced"
		}
		res := &sessiondata.QueryResult{Window: "totals", Totals: &tot}
		res.Meta = allMetaOf(cfg.PiDir, piMetas, codexMetas, diagnostics)
		if piPartialCoverage || codexPartialCoverage {
			markPartialRollupCoverage(res)
		}
		return res, nil
	case sessiondata.ViewGroups:
		rows := mergeGroupRows(buildGroups(piQueryReqs, v.By, false), buildGroups(codexQueryReqs, v.By, true))
		res := &sessiondata.QueryResult{Window: "totals", By: v.By, Rows: rows}
		res.Meta = allMetaOf(cfg.PiDir, piMetas, codexMetas, diagnostics)
		if piPartialCoverage || codexPartialCoverage {
			markPartialRollupCoverage(res)
		}
		return res, nil
	case sessiondata.ViewPeriod:
		rows := mergePeriodRows(buildPeriod(piQueryReqs, v.Period, false), buildPeriod(codexQueryReqs, v.Period, true))
		res := &sessiondata.QueryResult{Window: "totals", Period: v.Period, Rows: rows}
		res.Meta = allMetaOf(cfg.PiDir, piMetas, codexMetas, diagnostics)
		if piPartialCoverage || codexPartialCoverage {
			markPartialRollupCoverage(res)
		}
		return res, nil
	case sessiondata.ViewSessions:
		piRows := buildPiSessionRows(piScoped, piBySession, "pi")
		codexRows := buildCodexSessionRows(codexScoped, codexByPhysical)
		rows := append(piRows, codexRows...)
		tot := domain.EmptyTotals()
		for i := range rows {
			tot.Requests += rows[i].Requests
			tot.Input += rows[i].Input
			tot.Output += rows[i].Output
			tot.CacheRead += rows[i].CacheRead
			tot.CacheWrite += rows[i].CacheWrite
			tot.Reasoning += rows[i].Reasoning
			tot.Cost += rows[i].Cost
		}
		domain.FinalizeTotals(&tot)
		if codexContributed {
			tot.CostStatus = "unpriced"
		}
		total := len(rows)
		if v.SortKey != "" {
			sessiondata.SortSessionRows(rows, v.SortKey, v.SortDir == "desc")
		}
		pageRows := paginateSessionRows(rows, v)
		res := &sessiondata.QueryResult{Window: "sessions", Rows: pageRows, Total: total, Page: v.Page, Size: v.Size, Totals: &tot}
		res.Meta = allMetaOf(cfg.PiDir, piMetas, codexMetas, diagnostics)
		return res, nil
	}
	return nil, errUnknownView(string(v.Kind))
}

func mergeGroupRows(piRows, codexRows []domain.GroupRow) []domain.GroupRow {
	type key struct{ model, cwd string }
	merged := map[key]*domain.GroupRow{}
	var order []key
	merge := func(rows []domain.GroupRow) {
		for _, r := range rows {
			k := key{model: r.Model, cwd: r.Cwd}
			g, ok := merged[k]
			if !ok {
				v := r
				g = &v
				merged[k] = g
				order = append(order, k)
				continue
			}
			g.Requests += r.Requests
			g.Input += r.Input
			g.Output += r.Output
			g.CacheRead += r.CacheRead
			g.CacheWrite += r.CacheWrite
			g.Reasoning += r.Reasoning
			g.Cost += r.Cost
			if r.CostStatus == "unpriced" {
				g.CostStatus = "unpriced"
			}
		}
	}
	merge(piRows)
	merge(codexRows)
	rows := make([]domain.GroupRow, 0, len(order))
	for _, k := range order {
		g := merged[k]
		domain.FinalizeTotals(&g.Totals)
		rows = append(rows, *g)
	}
	return rows
}

func mergePeriodRows(piRows, codexRows []domain.PeriodRow) []domain.PeriodRow {
	merged := map[string]*domain.PeriodRow{}
	merge := func(rows []domain.PeriodRow) {
		for _, r := range rows {
			g, ok := merged[r.Period]
			if !ok {
				v := r
				merged[r.Period] = &v
				continue
			}
			g.Requests += r.Requests
			g.Input += r.Input
			g.Output += r.Output
			g.CacheRead += r.CacheRead
			g.CacheWrite += r.CacheWrite
			g.Reasoning += r.Reasoning
			g.Cost += r.Cost
			if r.CostStatus == "unpriced" {
				g.CostStatus = "unpriced"
			}
		}
	}
	merge(piRows)
	merge(codexRows)
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := make([]domain.PeriodRow, 0, len(keys))
	for _, k := range keys {
		g := merged[k]
		domain.FinalizeTotals(&g.Totals)
		rows = append(rows, *g)
	}
	return rows
}

func allMetaOf(dir string, piMetas []piSessionMeta, codexMetas []codexSessionMeta, diagnostics codex.Diagnostics) *sessiondata.QueryMeta {
	var minTs, maxTs *string
	consider := func(ts string) {
		if _, err := timerange.ParseUtcTimestamp(ts); err != nil {
			return
		}
		if minTs == nil || ts < *minTs {
			v := ts
			minTs = &v
		}
		if maxTs == nil || ts > *maxTs {
			v := ts
			maxTs = &v
		}
	}
	for _, m := range piMetas {
		consider(m.headerTs)
	}
	for _, m := range codexMetas {
		consider(m.tsText)
	}
	return &sessiondata.QueryMeta{
		Dir:                dir,
		SessionCount:       len(piMetas) + len(codexMetas),
		DataRange:          sessiondata.DataRangeValue{Since: minTs, Until: maxTs},
		Sources:            SupportedSources(),
		Warnings:           diagnostics.Warnings,
		UncountedSnapshots: diagnostics.UncountedSnapshots,
	}
}
