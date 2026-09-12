package query

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
	"github.com/heihei0299/token-analyzer/internal/timerange"
)

func TestPartialDayRollupReportsPartialCoverage(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	exec := func(statement string, args ...any) {
		t.Helper()
		if _, err := database.DB.Exec(statement, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO pi_sessions (session_id, header_ts, cwd, file_name, display_name) VALUES ('raw-session', '2026-08-01T00:00:00Z', '/workspace', 'raw.jsonl', 'raw')`)
	rawAt := time.Date(2026, 8, 1, 13, 0, 0, 0, time.FixedZone("CST", 8*60*60)).Unix()
	exec(`INSERT INTO proxy_request_logs (request_id, provider_id, app_type, model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, total_cost_usd, latency_ms, status_code, session_id, created_at, data_source, cwd, timestamp_text) VALUES ('raw-request', 'provider', 'pi', 'raw-model', 1, 1, 0, 0, '0', 0, 200, 'raw-session', ?, 'pi_session', '/workspace', ?)`, rawAt, "2026-08-01T13:00:00+08:00")
	exec(`INSERT INTO usage_daily_rollups (date, app_type, provider_id, model, request_count, success_count, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, total_cost_usd) VALUES (?, 'pi', 'provider', ?, ?, ?, ?, ?, ?, ?, '0')`, "2026-08-01", "lower-boundary", 10, 10, 100, 100, 0, 0)
	exec(`INSERT INTO usage_daily_rollups (date, app_type, provider_id, model, request_count, success_count, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, total_cost_usd) VALUES (?, 'pi', 'provider', ?, ?, ?, ?, ?, ?, ?, '0')`, "2026-08-02", "complete-day", 2, 2, 10, 5, 2, 0)
	exec(`INSERT INTO usage_daily_rollups (date, app_type, provider_id, model, request_count, success_count, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, total_cost_usd) VALUES (?, 'pi', 'provider', ?, ?, ?, ?, ?, ?, ?, '0')`, "2026-08-03", "upper-boundary", 3, 3, 30, 30, 0, 0)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	filterRange, err := timerange.MakeMessageRange("2026-08-01T12:00", "2026-08-03T18:00")
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{PiDir: t.TempDir(), DBPath: dbPath, Source: "pi"}
	views := []sessiondata.View{
		{Kind: sessiondata.ViewTotals},
		{Kind: sessiondata.ViewGroups, By: domain.GroupByModel},
		{Kind: sessiondata.ViewPeriod, Period: domain.PeriodDay},
	}
	for _, view := range views {
		result, err := Query(cfg, sessiondata.Filter{TimeRange: filterRange}, view)
		if err != nil {
			t.Fatalf("view %s: %v", view.Kind, err)
		}
		if result.Meta == nil || result.Meta.CoverageStatus != "partial" || len(result.Meta.Warnings) == 0 {
			t.Fatalf("view %s must expose partial rollup coverage: %+v", view.Kind, result.Meta)
		}
		switch view.Kind {
		case sessiondata.ViewTotals:
			if result.Totals.Requests != 3 || result.Totals.TotalTokens != 19 {
				t.Fatalf("partial totals must exclude boundary-day rollups: %+v", result.Totals)
			}
		case sessiondata.ViewGroups:
			rows := result.Rows.([]domain.GroupRow)
			if len(rows) != 2 {
				t.Fatalf("partial groups must contain raw plus complete interior day: %+v", rows)
			}
		case sessiondata.ViewPeriod:
			rows := result.Rows.([]domain.PeriodRow)
			if len(rows) != 2 || rows[0].Period != "2026-08-01" || rows[1].Period != "2026-08-02" {
				t.Fatalf("partial period must exclude boundary-day rollups: %+v", rows)
			}
		}
	}

	// Rollup rows have no session/request identity: raw-only views must not
	// fabricate rows for the two boundary rollups or the complete interior day.
	filter := sessiondata.Filter{TimeRange: filterRange}
	sessions, err := Query(cfg, filter, sessiondata.View{Kind: sessiondata.ViewSessions})
	if err != nil {
		t.Fatal(err)
	}
	sessionRows := sessions.Rows.([]domain.SessionRow)
	if len(sessionRows) != 1 || sessionRows[0].Requests != 1 || sessionRows[0].TotalTokens != 2 {
		t.Fatalf("partial sessions must remain raw-ledger only: %#v", sessions.Rows)
	}
	requests, err := Query(cfg, filter, sessiondata.View{Kind: sessiondata.ViewRequests})
	if err != nil {
		t.Fatal(err)
	}
	requestRows := requests.Rows.([]domain.RequestRow)
	if len(requestRows) != 1 || requestRows[0].TotalTokens != 2 {
		t.Fatalf("partial requests must remain raw-ledger only: %#v", requests.Rows)
	}
	detail, err := QueryDetail(cfg, "raw-session")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Totals.Main.Requests != 1 || detail.Totals.Merged.Requests != 1 || len(detail.Requests) != 1 {
		t.Fatalf("partial detail must remain raw-ledger only: %+v", detail)
	}
}
