package db

import (
	"fmt"
	"time"
)

const DefaultRollupRetentionDays = 30

const maintenanceOwnershipPredicate = `(app_type = 'pi' AND data_source = 'pi_session') OR (app_type = 'codex' AND data_source = 'codex')`

// RollupAndPrune atomically moves expired request rows into daily aggregates,
// deletes the moved rows, and reclaims SQLite pages. A failed transaction leaves
// both tables exactly as they were before maintenance.
func RollupAndPrune(database *Database, now time.Time, retentionDays int) error {
	if database == nil || database.DB == nil {
		return fmt.Errorf("rollup maintenance requires a database")
	}
	if retentionDays < 0 {
		return fmt.Errorf("invalid rollup retention days: %d", retentionDays)
	}

	tx, err := database.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin rollup maintenance: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour).Unix()
	_, err = tx.Exec(fmt.Sprintf(`
INSERT INTO usage_daily_rollups (
	date, app_type, provider_id, model, request_model, pricing_model,
	request_count, success_count, input_tokens, output_tokens,
	cache_read_tokens, cache_creation_tokens, reasoning_tokens, input_token_semantics,
	total_cost_usd, avg_latency_ms
)
SELECT
	date(created_at, 'unixepoch', 'localtime'),
	app_type,
	provider_id,
	model,
	COALESCE(request_model, ''),
	COALESCE(pricing_model, ''),
	COUNT(*),
	SUM(CASE WHEN status_code >= 200 AND status_code < 300 THEN 1 ELSE 0 END),
	SUM(input_tokens),
	SUM(output_tokens),
	SUM(cache_read_tokens),
	SUM(cache_creation_tokens),
	SUM(reasoning_tokens),
	MAX(input_token_semantics),
	CAST(SUM(CAST(COALESCE(total_cost_usd, '0') AS REAL)) AS TEXT),
	CAST(AVG(latency_ms) AS INTEGER)
FROM proxy_request_logs
WHERE (%s)
  AND created_at < ?
GROUP BY date(created_at, 'unixepoch', 'localtime'), app_type, provider_id, model,
         COALESCE(request_model, ''), COALESCE(pricing_model, '')
ON CONFLICT(date, app_type, provider_id, model, request_model, pricing_model)
DO UPDATE SET
	request_count = usage_daily_rollups.request_count + excluded.request_count,
	success_count = usage_daily_rollups.success_count + excluded.success_count,
	input_tokens = usage_daily_rollups.input_tokens + excluded.input_tokens,
	output_tokens = usage_daily_rollups.output_tokens + excluded.output_tokens,
	cache_read_tokens = usage_daily_rollups.cache_read_tokens + excluded.cache_read_tokens,
	cache_creation_tokens = usage_daily_rollups.cache_creation_tokens + excluded.cache_creation_tokens,
	reasoning_tokens = usage_daily_rollups.reasoning_tokens + excluded.reasoning_tokens,
	input_token_semantics = MAX(usage_daily_rollups.input_token_semantics, excluded.input_token_semantics),
	total_cost_usd = CAST(CAST(usage_daily_rollups.total_cost_usd AS REAL) + CAST(excluded.total_cost_usd AS REAL) AS TEXT),
	avg_latency_ms = CASE
		WHEN usage_daily_rollups.request_count + excluded.request_count = 0 THEN 0
		ELSE CAST((usage_daily_rollups.avg_latency_ms * usage_daily_rollups.request_count + excluded.avg_latency_ms * excluded.request_count) * 1.0 / (usage_daily_rollups.request_count + excluded.request_count) AS INTEGER)
	END`, maintenanceOwnershipPredicate), cutoff)
	if err != nil {
		return fmt.Errorf("write usage daily rollups: %w", err)
	}

	if _, err = tx.Exec(fmt.Sprintf(`DELETE FROM proxy_request_logs WHERE (%s) AND created_at < ?`, maintenanceOwnershipPredicate), cutoff); err != nil {
		return fmt.Errorf("prune expired usage: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rollup maintenance: %w", err)
	}
	committed = true
	// Vacuum only reclaims pages; a failure must not make a committed
	// rollup/prune look like a failed maintenance transaction.
	_, _ = database.DB.Exec(`PRAGMA incremental_vacuum`)
	return nil
}
