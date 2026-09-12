package db

import (
	"path/filepath"
	"testing"
	"time"
)

func insertMaintenanceUsage(t *testing.T, database *Database, id string, createdAt time.Time, status int, model string, input, output, cacheRead, cacheWrite int, cost string, latency int) {
	t.Helper()
	_, err := database.DB.Exec(`INSERT INTO proxy_request_logs (request_id, provider_id, app_type, model, request_model, pricing_model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, total_cost_usd, latency_ms, status_code, session_id, created_at, data_source, kind, cwd, timestamp_text) VALUES (?, 'provider', 'pi', ?, 'request-model', 'pricing-model', ?, ?, ?, ?, ?, ?, ?, 'session', ?, 'pi_session', 'assistant', '/workspace', ?)`,
		id, model, input, output, cacheRead, cacheWrite, cost, latency, status, createdAt.Unix(), createdAt.UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
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

func TestRollupAndPruneRollsBackOnRollupFailure(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	insertMaintenanceUsage(t, database, "old", time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC), 200, "model-a", 10, 5, 0, 0, "0.10", 100)
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
	if rawCount != 1 || rollupCount != 0 {
		t.Fatalf("failed maintenance must keep raw and leave no rollup: raw=%d rollups=%d", rawCount, rollupCount)
	}
}
