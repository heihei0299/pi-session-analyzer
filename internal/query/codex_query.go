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
	"strings"

	"github.com/heihei0299/token-analyzer/internal/codex"
	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
	"github.com/heihei0299/token-analyzer/internal/timerange"
)

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
func loadCodexDiagnostics(database *db.Database, home string) (codex.Diagnostics, error) {
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

func queryCodex(database *db.Database, home string, f sessiondata.Filter, v sessiondata.View) (*sessiondata.QueryResult, error) {
	metas, err := loadCodexMetas(database, home)
	if err != nil {
		return nil, err
	}
	metas, err = countedCodexMetas(database, metas)
	if err != nil {
		return nil, err
	}
	diagnostics, err := loadCodexDiagnostics(database, home)
	if err != nil {
		return nil, err
	}
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
		sortSessionRows(rows, v)
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
