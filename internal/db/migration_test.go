package db

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func createLegacyV1(t *testing.T, path string, sourceSessionsView bool) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	statements := []string{
		`CREATE TABLE proxy_request_logs (
			request_id TEXT PRIMARY KEY, provider_id TEXT NOT NULL, app_type TEXT NOT NULL,
			model TEXT NOT NULL, request_model TEXT, pricing_model TEXT,
			input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0,
			cache_read_tokens INTEGER NOT NULL DEFAULT 0, cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
			input_token_semantics INTEGER NOT NULL DEFAULT 0, input_cost_usd TEXT NOT NULL DEFAULT '0',
			output_cost_usd TEXT NOT NULL DEFAULT '0', cache_read_cost_usd TEXT NOT NULL DEFAULT '0',
			cache_creation_cost_usd TEXT NOT NULL DEFAULT '0', total_cost_usd TEXT NOT NULL DEFAULT '0',
			latency_ms INTEGER NOT NULL, first_token_ms INTEGER, duration_ms INTEGER,
			status_code INTEGER NOT NULL, error_message TEXT, session_id TEXT, provider_type TEXT,
			is_streaming INTEGER NOT NULL DEFAULT 0, cost_multiplier TEXT NOT NULL DEFAULT '1.0',
			created_at INTEGER NOT NULL, data_source TEXT NOT NULL DEFAULT 'proxy'
		)`,
		`CREATE TABLE session_log_sync (
			file_path TEXT PRIMARY KEY, last_modified INTEGER NOT NULL,
			last_line_offset INTEGER NOT NULL DEFAULT 0, last_synced_at INTEGER NOT NULL,
			last_byte_offset INTEGER, last_tail_fingerprint INTEGER
		)`,
		`CREATE TABLE session_usage_dedup (
			data_source TEXT NOT NULL, request_id TEXT NOT NULL, semantic_id TEXT NOT NULL,
			has_entry_id INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (data_source, request_id)
		)`,
		`CREATE TABLE usage_daily_rollups (
			date TEXT NOT NULL, app_type TEXT NOT NULL, provider_id TEXT NOT NULL, model TEXT NOT NULL,
			request_model TEXT NOT NULL DEFAULT '', pricing_model TEXT NOT NULL DEFAULT '',
			request_count INTEGER NOT NULL DEFAULT 0, success_count INTEGER NOT NULL DEFAULT 0,
			input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0,
			cache_read_tokens INTEGER NOT NULL DEFAULT 0, cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
			input_token_semantics INTEGER NOT NULL DEFAULT 0, total_cost_usd TEXT NOT NULL DEFAULT '0',
			avg_latency_ms INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (date, app_type, provider_id, model, request_model, pricing_model)
		)`,
		`CREATE TABLE model_pricing (
			model_id TEXT PRIMARY KEY, display_name TEXT NOT NULL,
			input_cost_per_million TEXT NOT NULL, output_cost_per_million TEXT NOT NULL,
			cache_read_cost_per_million TEXT NOT NULL DEFAULT '0',
			cache_creation_cost_per_million TEXT NOT NULL DEFAULT '0'
		)`,
		`INSERT INTO proxy_request_logs (request_id, provider_id, app_type, model, latency_ms, status_code, created_at, data_source)
		 VALUES ('legacy-row', 'provider', 'pi', 'legacy-model', 0, 200, 1, 'pi_session')`,
		`PRAGMA user_version = 1`,
	}
	if sourceSessionsView {
		statements = append(statements, `CREATE VIEW source_sessions AS SELECT request_id FROM proxy_request_logs`)
	}
	for _, statement := range statements {
		if _, err := sqlDB.Exec(statement); err != nil {
			t.Fatalf("legacy fixture statement failed: %s: %v", statement, err)
		}
	}
}

func createLegacyV2WithoutRootBinding(t *testing.T, path string) {
	t.Helper()
	createLegacyV1(t, path, false)
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 2`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
}

func readTableColumns(t *testing.T, database *Database, table string) map[string]bool {
	t.Helper()
	rows, err := database.DB.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return columns
}

func TestSchemaMigrationFreshAndSupportedV1(t *testing.T) {
	if SchemaVersion != 3 {
		t.Fatalf("source root binding requires schema version 3, got %d", SchemaVersion)
	}
	freshPath := filepath.Join(t.TempDir(), "fresh.db")
	fresh, err := Open(freshPath)
	if err != nil {
		t.Fatal(err)
	}
	version, err := fresh.GetUserVersion()
	if err != nil || version != SchemaVersion {
		t.Fatalf("fresh database version=%d err=%v", version, err)
	}
	for _, table := range []string{"proxy_request_logs", "session_log_sync", "usage_daily_rollups", "pi_sessions", "source_sessions", "source_root_bindings"} {
		if len(readTableColumns(t, fresh, table)) == 0 {
			t.Fatalf("fresh database missing table %s", table)
		}
	}
	if err := fresh.Close(); err != nil {
		t.Fatal(err)
	}

	legacyPath := filepath.Join(t.TempDir(), "legacy.db")
	createLegacyV1(t, legacyPath, false)
	legacy, err := Open(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	version, err = legacy.GetUserVersion()
	if err != nil || version != SchemaVersion {
		t.Fatalf("migrated database version=%d err=%v", version, err)
	}
	columns := readTableColumns(t, legacy, "proxy_request_logs")
	for _, column := range []string{"physical_rollout_id", "stop_reason", "kind", "reasoning_tokens", "cwd", "timestamp_text"} {
		if !columns[column] {
			t.Fatalf("migrated database missing proxy column %s", column)
		}
	}
	var count int
	if err := legacy.DB.QueryRow(`SELECT COUNT(*) FROM proxy_request_logs WHERE request_id = 'legacy-row'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("legacy row was not preserved: count=%d err=%v", count, err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(legacyPath)
	if err != nil {
		t.Fatalf("reopening an upgraded database must be idempotent: %v", err)
	}
	defer reopened.Close()
	version, err = reopened.GetUserVersion()
	if err != nil || version != SchemaVersion {
		t.Fatalf("reopened database version=%d err=%v", version, err)
	}
}

func TestLegacyV2MigrationAddsRootBinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-v2.db")
	createLegacyV2WithoutRootBinding(t, path)
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	version, err := database.GetUserVersion()
	if err != nil || version != SchemaVersion {
		t.Fatalf("legacy v2 migration version=%d err=%v", version, err)
	}
	var count int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'source_root_bindings' AND type = 'table'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("legacy v2 migration must add source_root_bindings: count=%d err=%v", count, err)
	}
}

func TestReadOnlyLegacyV2WithoutRootBindingFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-v2-readonly.db")
	createLegacyV2WithoutRootBinding(t, path)
	readonly, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	if err := CheckPinnedSourceRoot(readonly, "pi", canonicalRoot(t, t.TempDir())); !errors.Is(err, ErrSourceRootBindingMissing) {
		t.Fatalf("read-only legacy ledger without binding must fail closed: %v", err)
	}
}

func TestSchemaMigrationRollbackAndRetry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failed.db")
	createLegacyV1(t, path, true)
	if database, err := Open(path); err == nil {
		database.Close()
		t.Fatal("conflicting schema object must fail migration")
	}

	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err := raw.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if version != 1 {
		raw.Close()
		t.Fatalf("failed migration advanced version to %d", version)
	}
	var objects int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'pi_sessions' AND type = 'table'`).Scan(&objects); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if objects != 0 {
		raw.Close()
		t.Fatal("failed migration left a newly created table")
	}
	raw.Close()

	raw, err = sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`DROP VIEW source_sessions`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	database, err := Open(path)
	if err != nil {
		t.Fatal(fmt.Errorf("retry migration: %w", err))
	}
	defer database.Close()
	version, err = database.GetUserVersion()
	if err != nil || version != SchemaVersion {
		t.Fatalf("retry version=%d err=%v", version, err)
	}
}
