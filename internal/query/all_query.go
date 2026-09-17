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
	"sort"

	"github.com/heihei0299/token-analyzer/internal/codex"
	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/pi"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
	"github.com/heihei0299/token-analyzer/internal/timerange"
)

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
	diagnostics, err := loadCodexDiagnostics(database, home)
	if err != nil {
		return nil, err
	}
	piDiagnostics, err := pi.LoadDiagnostics(database, cfg.PiDir)
	if err != nil {
		return nil, err
	}
	metaOfAll := func() *sessiondata.QueryMeta {
		meta := allMetaOf(cfg.PiDir, piMetas, codexMetas, diagnostics)
		meta.Warnings = append(meta.Warnings, piDiagnostics.Warnings...)
		return meta
	}
	if v.Kind == sessiondata.ViewMeta {
		return &sessiondata.QueryResult{Window: "meta", Meta: metaOfAll()}, nil
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
		res.Meta = metaOfAll()
		if piPartialCoverage || codexPartialCoverage {
			markPartialRollupCoverage(res)
		}
		return res, nil
	case sessiondata.ViewGroups:
		rows := mergeGroupRows(buildGroups(piQueryReqs, v.By, false), buildGroups(codexQueryReqs, v.By, true))
		res := &sessiondata.QueryResult{Window: "totals", By: v.By, Rows: rows}
		res.Meta = metaOfAll()
		if piPartialCoverage || codexPartialCoverage {
			markPartialRollupCoverage(res)
		}
		return res, nil
	case sessiondata.ViewPeriod:
		rows := mergePeriodRows(buildPeriod(piQueryReqs, v.Period, false), buildPeriod(codexQueryReqs, v.Period, true))
		res := &sessiondata.QueryResult{Window: "totals", Period: v.Period, Rows: rows}
		res.Meta = metaOfAll()
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
		sortSessionRows(rows, v)
		pageRows := paginateSessionRows(rows, v)
		res := &sessiondata.QueryResult{Window: "sessions", Rows: pageRows, Total: total, Page: v.Page, Size: v.Size, Totals: &tot}
		res.Meta = metaOfAll()
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
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Model != rows[j].Model {
			return rows[i].Model < rows[j].Model
		}
		return rows[i].Cwd < rows[j].Cwd
	})
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
