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

	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/pi"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
	"github.com/heihei0299/token-analyzer/internal/timerange"
)

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

func queryPi(database *db.Database, dir string, f sessiondata.Filter, v sessiondata.View, whitelist map[string]bool) (*sessiondata.QueryResult, error) {
	metas, err := loadPiMetas(database)
	if err != nil {
		return nil, err
	}
	piDiagnostics, err := pi.LoadDiagnostics(database, dir)
	if err != nil {
		return nil, err
	}
	meta := piMetaOf(dir, metas, piDiagnostics)
	if v.Kind == sessiondata.ViewMeta {
		return &sessiondata.QueryResult{Window: "meta", Meta: meta}, nil
	}
	attachDiagnostics := func(result *sessiondata.QueryResult) {
		if len(piDiagnostics.Warnings) > 0 || piDiagnostics.Skipped > 0 {
			result.Meta = meta
		}
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
		attachDiagnostics(result)
		return result, nil
	case sessiondata.ViewGroups:
		result := &sessiondata.QueryResult{Window: "totals", By: v.By, Rows: buildGroups(queryReqs, v.By, false)}
		if partialCoverage {
			markPartialRollupCoverage(result)
		}
		attachDiagnostics(result)
		return result, nil
	case sessiondata.ViewPeriod:
		result := &sessiondata.QueryResult{Window: "totals", Period: v.Period, Rows: buildPeriod(queryReqs, v.Period, false)}
		if partialCoverage {
			markPartialRollupCoverage(result)
		}
		attachDiagnostics(result)
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
		sortSessionRows(rows, v)
		pageRows := paginateSessionRows(rows, v)
		result := &sessiondata.QueryResult{Window: "sessions", Rows: pageRows, Total: total, Page: v.Page, Size: v.Size, Totals: &tot}
		attachDiagnostics(result)
		return result, nil
	case sessiondata.ViewRequests:
		rows := buildPiRequestRows(scoped, bySession)
		total := len(rows)
		sortRequestRows(rows, v)
		pageRows := paginateRequestRows(rows, v)
		result := &sessiondata.QueryResult{Window: "requests", Rows: pageRows, Total: total, Page: v.Page, Size: v.Size}
		attachDiagnostics(result)
		return result, nil
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

func piMetaOf(dir string, metas []piSessionMeta, diagnostics pi.Diagnostics) *sessiondata.QueryMeta {
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
		Warnings:     diagnostics.Warnings,
	}
}

func paginationBounds(length, page, size int) (start, end int, ok bool) {
	if page == 0 && size == 0 {
		return 0, length, true
	}
	if page <= 0 || size <= 0 {
		return 0, 0, false
	}
	pageOffset := page - 1
	if pageOffset > length/size {
		return 0, 0, false
	}
	start = pageOffset * size
	if start >= length {
		return 0, 0, false
	}
	end = length
	if size <= length-start {
		end = start + size
	}
	return start, end, true
}

func paginateSessionRows(rows []domain.SessionRow, v sessiondata.View) []domain.SessionRow {
	start, end, ok := paginationBounds(len(rows), v.Page, v.Size)
	if !ok {
		return make([]domain.SessionRow, 0)
	}
	out := rows[start:end]
	if out == nil {
		return make([]domain.SessionRow, 0)
	}
	return out
}

func paginateRequestRows(rows []domain.RequestRow, v sessiondata.View) []domain.RequestRow {
	start, end, ok := paginationBounds(len(rows), v.Page, v.Size)
	if !ok {
		return make([]domain.RequestRow, 0)
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
