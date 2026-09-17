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

	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

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
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Model != rows[j].Model {
			return rows[i].Model < rows[j].Model
		}
		return rows[i].Cwd < rows[j].Cwd
	})
	return rows
}

func sortSessionRows(rows []domain.SessionRow, view sessiondata.View) {
	key, desc := view.SortKey, view.SortDir == "desc"
	if key == "" {
		key, desc = "timestamp", true
	}
	sessiondata.SortSessionRows(rows, key, desc)
}

func sortRequestRows(rows []domain.RequestRow, view sessiondata.View) {
	key, desc := view.SortKey, view.SortDir == "desc"
	if key == "" {
		key, desc = "timestamp", true
	}
	sessiondata.SortRequestRows(rows, key, desc)
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
