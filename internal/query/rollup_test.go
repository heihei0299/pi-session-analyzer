package query

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

func TestQueryUsesRollupsForAggregateWindowsOnly(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	piDir := t.TempDir()
	bindPiRoot(t, database, piDir)
	exec := func(statement string, args ...any) {
		t.Helper()
		if _, err := database.DB.Exec(statement, args...); err != nil {
			t.Fatal(err)
		}
	}

	exec(`INSERT INTO pi_sessions (session_id, header_ts, cwd, file_name, display_name) VALUES (?, ?, ?, ?, ?)`,
		"recent-session", "2026-09-08T12:00:00Z", "/workspace", "recent.jsonl", "recent")
	recentAt := time.Date(2026, 9, 8, 12, 0, 1, 0, time.UTC).Unix()
	exec(`INSERT INTO proxy_request_logs (request_id, provider_id, app_type, model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, total_cost_usd, latency_ms, status_code, session_id, created_at, data_source, cwd, timestamp_text) VALUES (?, ?, 'pi', ?, ?, ?, ?, ?, ?, 0, 200, ?, ?, 'pi_session', ?, ?)`,
		"recent-request", "provider", "recent-model", 2, 3, 0, 1, "0.20", "recent-session", recentAt, "/workspace", "2026-09-08T12:00:01Z")
	exec(`INSERT INTO usage_daily_rollups (date, app_type, provider_id, model, request_count, success_count, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, total_cost_usd) VALUES (?, 'pi', ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"2026-09-01", "provider", "old-model", 2, 2, 10, 5, 3, 1, "0.40")
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := Config{PiDir: piDir, DBPath: dbPath, Source: "pi"}
	totals, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	if totals.Totals == nil || totals.Totals.Requests != 3 || totals.Totals.Input != 12 || totals.Totals.Output != 8 || totals.Totals.CacheRead != 3 || totals.Totals.CacheWrite != 2 || totals.Totals.TotalTokens != 23 || math.Abs(totals.Totals.Cost-0.6) > 1e-9 {
		t.Fatalf("totals must include raw and pruned rollup rows: %+v", totals.Totals)
	}

	groups, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewGroups, By: domain.GroupByModel})
	if err != nil {
		t.Fatal(err)
	}
	groupRows, ok := groups.Rows.([]domain.GroupRow)
	if !ok || len(groupRows) != 2 {
		t.Fatalf("model groups must include raw and rollup models: %#v", groups.Rows)
	}
	groupTokens := map[string]float64{}
	for _, row := range groupRows {
		groupTokens[row.Model] = row.TotalTokens
	}
	if groupTokens["old-model"] != 18 || groupTokens["recent-model"] != 5 {
		t.Fatalf("unexpected model rollup groups: %+v", groupTokens)
	}

	period, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewPeriod, Period: domain.PeriodDay})
	if err != nil {
		t.Fatal(err)
	}
	periodRows, ok := period.Rows.([]domain.PeriodRow)
	if !ok || len(periodRows) != 2 || periodRows[0].Period != "2026-09-01" || periodRows[0].TotalTokens != 18 || periodRows[1].Period != "2026-09-08" || periodRows[1].TotalTokens != 5 {
		t.Fatalf("period must include both raw and rollup days: %#v", period.Rows)
	}

	sessions, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewSessions})
	if err != nil {
		t.Fatal(err)
	}
	sessionRows, ok := sessions.Rows.([]domain.SessionRow)
	if !ok || len(sessionRows) != 1 || sessionRows[0].Requests != 1 || sessionRows[0].TotalTokens != 5 {
		t.Fatalf("sessions must remain raw-ledger only: %#v", sessions.Rows)
	}

	requests, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewRequests})
	if err != nil {
		t.Fatal(err)
	}
	requestRows, ok := requests.Rows.([]domain.RequestRow)
	if !ok || len(requestRows) != 1 || requestRows[0].TotalTokens != 5 {
		t.Fatalf("requests must remain raw-ledger only: %#v", requests.Rows)
	}

	detail, err := QueryDetail(cfg, "recent-session")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Totals.Main.Requests != 1 || detail.Totals.Merged.Requests != 1 || len(detail.Requests) != 1 {
		t.Fatalf("detail must remain raw-ledger only: %+v", detail)
	}

	cwdGroups, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewGroups, By: domain.GroupByCwd})
	if err != nil {
		t.Fatal(err)
	}
	cwdRows, ok := cwdGroups.Rows.([]domain.GroupRow)
	if !ok || len(cwdRows) != 1 || cwdRows[0].Requests != 1 || cwdRows[0].TotalTokens != 5 {
		t.Fatalf("cwd groups must not fabricate rollup cwd: %#v", cwdGroups.Rows)
	}
}
