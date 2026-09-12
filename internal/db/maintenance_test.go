package db

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func insertMaintenanceUsage(t *testing.T, database *Database, id string, createdAt time.Time, status int, model string, input, output, cacheRead, cacheWrite int, cost string, latency int) {
	insertMaintenanceRow(t, database, id, createdAt, "pi", "pi_session", status, model, input, output, cacheRead, cacheWrite, cost, latency)
}

func insertMaintenanceRow(t *testing.T, database *Database, id string, createdAt time.Time, appType, dataSource string, status int, model string, input, output, cacheRead, cacheWrite int, cost string, latency int) {
	t.Helper()
	_, err := database.DB.Exec(`INSERT INTO proxy_request_logs (request_id, provider_id, app_type, model, request_model, pricing_model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, total_cost_usd, latency_ms, status_code, session_id, created_at, data_source, kind, cwd, timestamp_text) VALUES (?, 'provider', ?, ?, 'request-model', 'pricing-model', ?, ?, ?, ?, ?, ?, ?, 'session', ?, ?, 'assistant', '/workspace', ?)`,
		id, appType, model, input, output, cacheRead, cacheWrite, cost, latency, status, createdAt.Unix(), dataSource, createdAt.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
}

func assertMaintenanceRow(t *testing.T, database *Database, id, appType, dataSource, model string, input, output int, cost string) {
	t.Helper()
	var gotAppType, gotDataSource, gotModel, gotCost string
	var gotInput, gotOutput int
	err := database.DB.QueryRow(`SELECT app_type, data_source, model, input_tokens, output_tokens, total_cost_usd FROM proxy_request_logs WHERE request_id = ?`, id).
		Scan(&gotAppType, &gotDataSource, &gotModel, &gotInput, &gotOutput, &gotCost)
	if err != nil {
		t.Fatalf("read shared row %q: %v", id, err)
	}
	if gotAppType != appType || gotDataSource != dataSource || gotModel != model || gotInput != input || gotOutput != output || gotCost != cost {
		t.Fatalf("shared row %q changed: got=%q/%q/%q %d/%d %q", id, gotAppType, gotDataSource, gotModel, gotInput, gotOutput, gotCost)
	}
}

func TestRollupAndPruneIsAtomicAndIdempotent(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	insertMaintenanceUsage(t, database, "old-1", time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC), 200, "model-a", 10, 5, 1, 2, "0.10", 100)
	insertMaintenanceUsage(t, database, "old-2", time.Date(2026, 8, 1, 2, 0, 0, 0, time.UTC), 500, "model-a", 3, 2, 2, 1, "0.20", 200)
	insertMaintenanceUsage(t, database, "recent", time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC), 200, "model-a", 100, 100, 0, 0, "1.00", 50)
	if _, err := database.DB.Exec(`UPDATE proxy_request_logs SET reasoning_tokens = 7 WHERE request_id = 'old-1'`); err != nil {
		t.Fatal(err)
	}

	if err := RollupAndPrune(database, now, 30); err != nil {
		t.Fatal(err)
	}
	var rawCount, rollupCount int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM proxy_request_logs`).Scan(&rawCount); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM usage_daily_rollups`).Scan(&rollupCount); err != nil {
		t.Fatal(err)
	}
	if rawCount != 1 || rollupCount != 1 {
		t.Fatalf("maintenance must retain recent raw and create one rollup: raw=%d rollups=%d", rawCount, rollupCount)
	}
	var requests, successes, input, output, cacheRead, cacheWrite, reasoning, avgLatency int
	var cost string
	if err := database.DB.QueryRow(`SELECT request_count, success_count, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, reasoning_tokens, total_cost_usd, avg_latency_ms FROM usage_daily_rollups`).Scan(&requests, &successes, &input, &output, &cacheRead, &cacheWrite, &reasoning, &cost, &avgLatency); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || successes != 1 || input != 13 || output != 7 || cacheRead != 3 || cacheWrite != 3 || reasoning != 7 || cost != "0.3" || avgLatency != 150 {
		t.Fatalf("unexpected rollup: requests=%d successes=%d usage=%d/%d/%d/%d reasoning=%d cost=%q latency=%d", requests, successes, input, output, cacheRead, cacheWrite, reasoning, cost, avgLatency)
	}
	if err := RollupAndPrune(database, now, 30); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM usage_daily_rollups`).Scan(&rollupCount); err != nil {
		t.Fatal(err)
	}
	if rollupCount != 1 {
		t.Fatalf("repeated maintenance must not duplicate rollups: %d", rollupCount)
	}
}

func TestRollupAndPrunePreservesSharedDatabaseRows(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	old := time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC)
	insertMaintenanceRow(t, database, "old-pi", old, "pi", "pi_session", 200, "pi-model", 10, 5, 1, 2, "0.10", 100)
	insertMaintenanceRow(t, database, "old-codex", old, "codex", "codex", 200, "codex-model", 20, 6, 2, 3, "0.20", 200)
	insertMaintenanceRow(t, database, "old-generic", old, "proxy", "proxy", 200, "generic-old", 30, 7, 3, 4, "0.30", 300)
	insertMaintenanceRow(t, database, "old-other", old, "other", "other", 200, "other-old", 40, 8, 4, 5, "0.40", 400)
	insertMaintenanceRow(t, database, "recent-pi", recent, "pi", "pi_session", 200, "pi-recent", 50, 9, 5, 6, "0.50", 500)
	insertMaintenanceRow(t, database, "recent-codex", recent, "codex", "codex", 200, "codex-recent", 60, 10, 6, 7, "0.60", 600)
	insertMaintenanceRow(t, database, "recent-generic", recent, "proxy", "proxy", 200, "generic-recent", 70, 11, 7, 8, "0.70", 700)

	if err := RollupAndPrune(database, now, 30); err != nil {
		t.Fatal(err)
	}

	var rawCount, rollupCount, rollupRequests, rollupInput, rollupOutput, nonOwnedRollups int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM proxy_request_logs`).Scan(&rawCount); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(request_count), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0) FROM usage_daily_rollups`).Scan(&rollupCount, &rollupRequests, &rollupInput, &rollupOutput); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM usage_daily_rollups WHERE app_type NOT IN ('pi', 'codex')`).Scan(&nonOwnedRollups); err != nil {
		t.Fatal(err)
	}
	if rawCount != 5 || rollupCount != 2 || rollupRequests != 2 || rollupInput != 30 || rollupOutput != 11 || nonOwnedRollups != 0 {
		t.Fatalf("shared rows were included in maintenance: raw=%d rollups=%d requests=%d usage=%d/%d non-owned-rollups=%d", rawCount, rollupCount, rollupRequests, rollupInput, rollupOutput, nonOwnedRollups)
	}

	for _, row := range []struct {
		id, appType, dataSource, model string
		input, output                  int
		cost                           string
	}{
		{"old-generic", "proxy", "proxy", "generic-old", 30, 7, "0.30"},
		{"old-other", "other", "other", "other-old", 40, 8, "0.40"},
		{"recent-pi", "pi", "pi_session", "pi-recent", 50, 9, "0.50"},
		{"recent-codex", "codex", "codex", "codex-recent", 60, 10, "0.60"},
		{"recent-generic", "proxy", "proxy", "generic-recent", 70, 11, "0.70"},
	} {
		assertMaintenanceRow(t, database, row.id, row.appType, row.dataSource, row.model, row.input, row.output, row.cost)
	}
	for _, id := range []string{"old-pi", "old-codex"} {
		var count int
		if err := database.DB.QueryRow(`SELECT COUNT(*) FROM proxy_request_logs WHERE request_id = ?`, id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("expired owned row %q should be pruned", id)
		}
	}

	if err := RollupAndPrune(database, now, 30); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM proxy_request_logs`).Scan(&rawCount); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(request_count), 0) FROM usage_daily_rollups`).Scan(&rollupCount, &rollupRequests); err != nil {
		t.Fatal(err)
	}
	if rawCount != 5 || rollupCount != 2 || rollupRequests != 2 {
		t.Fatalf("maintenance is not idempotent: raw=%d rollups=%d requests=%d", rawCount, rollupCount, rollupRequests)
	}
}

func TestMaintenanceOwnershipPredicate(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	query := fmt.Sprintf(`SELECT CASE WHEN (%s) THEN 1 ELSE 0 END FROM (SELECT ? AS app_type, ? AS data_source)`, maintenanceOwnershipPredicate)
	for _, test := range []struct {
		appType, dataSource string
		owned               bool
	}{
		{"pi", "pi_session", true},
		{"codex", "codex", true},
		{"pi", "proxy", false},
		{"codex", "proxy", false},
		{"proxy", "proxy", false},
	} {
		var got int
		if err := database.DB.QueryRow(query, test.appType, test.dataSource).Scan(&got); err != nil {
			t.Fatalf("evaluate ownership predicate for %s/%s: %v", test.appType, test.dataSource, err)
		}
		if (got == 1) != test.owned {
			t.Errorf("ownership for %s/%s = %v, want %v", test.appType, test.dataSource, got == 1, test.owned)
		}
	}
}

func TestRollupAndPruneRollsBackOnRollupFailure(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	old := time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC)
	insertMaintenanceRow(t, database, "old-pi", old, "pi", "pi_session", 200, "pi-model", 10, 5, 1, 2, "0.10", 100)
	insertMaintenanceRow(t, database, "old-codex", old, "codex", "codex", 200, "codex-model", 20, 6, 2, 3, "0.20", 200)
	insertMaintenanceRow(t, database, "old-generic", old, "proxy", "proxy", 200, "generic-model", 30, 7, 3, 4, "0.30", 300)
	insertMaintenanceRow(t, database, "old-other", old, "other", "other", 200, "other-model", 40, 8, 4, 5, "0.40", 400)
	if _, err := database.DB.Exec(`CREATE TRIGGER fail_rollup BEFORE INSERT ON usage_daily_rollups BEGIN SELECT RAISE(ABORT, 'injected rollup failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := RollupAndPrune(database, now, 30); err == nil {
		t.Fatal("rollup failure must be returned")
	}
	if _, err := database.DB.Exec(`DROP TRIGGER fail_rollup`); err != nil {
		t.Fatal(err)
	}
	var rawCount, rollupCount int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM proxy_request_logs`).Scan(&rawCount); err != nil {
		t.Fatal(err)
	}
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM usage_daily_rollups`).Scan(&rollupCount); err != nil {
		t.Fatal(err)
	}
	if rawCount != 4 || rollupCount != 0 {
		t.Fatalf("failed maintenance must keep all raw rows and leave no rollup: raw=%d rollups=%d", rawCount, rollupCount)
	}
	for _, row := range []struct {
		id, appType, dataSource, model string
		input, output                  int
		cost                           string
	}{
		{"old-pi", "pi", "pi_session", "pi-model", 10, 5, "0.10"},
		{"old-codex", "codex", "codex", "codex-model", 20, 6, "0.20"},
		{"old-generic", "proxy", "proxy", "generic-model", 30, 7, "0.30"},
		{"old-other", "other", "other", "other-model", 40, 8, "0.40"},
	} {
		assertMaintenanceRow(t, database, row.id, row.appType, row.dataSource, row.model, row.input, row.output, row.cost)
	}
}
