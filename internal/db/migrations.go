package db

import (
	"database/sql"
	"fmt"
)

func createLatestSchema(tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS proxy_request_logs (
			request_id TEXT PRIMARY KEY,
			provider_id TEXT NOT NULL,
			app_type TEXT NOT NULL,
			model TEXT NOT NULL,
			request_model TEXT,
			pricing_model TEXT,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			cache_read_tokens INTEGER NOT NULL DEFAULT 0,
			cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
			input_token_semantics INTEGER NOT NULL DEFAULT 0,
			input_cost_usd TEXT NOT NULL DEFAULT '0',
			output_cost_usd TEXT NOT NULL DEFAULT '0',
			cache_read_cost_usd TEXT NOT NULL DEFAULT '0',
			cache_creation_cost_usd TEXT NOT NULL DEFAULT '0',
			total_cost_usd TEXT NOT NULL DEFAULT '0',
			latency_ms INTEGER NOT NULL,
			first_token_ms INTEGER,
			duration_ms INTEGER,
			status_code INTEGER NOT NULL,
			stop_reason TEXT NOT NULL DEFAULT '',
			error_message TEXT,
			session_id TEXT,
			provider_type TEXT,
			is_streaming INTEGER NOT NULL DEFAULT 0,
			cost_multiplier TEXT NOT NULL DEFAULT '1.0',
			created_at INTEGER NOT NULL,
			data_source TEXT NOT NULL DEFAULT 'proxy',
			physical_rollout_id TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT 'assistant',
			reasoning_tokens INTEGER NOT NULL DEFAULT 0,
			cwd TEXT NOT NULL DEFAULT '',
			timestamp_text TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_provider ON proxy_request_logs(provider_id, app_type)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON proxy_request_logs(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_model ON proxy_request_logs(model)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_session ON proxy_request_logs(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_status ON proxy_request_logs(status_code)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_usage ON proxy_request_logs(app_type, data_source, created_at)`,
		`CREATE TABLE IF NOT EXISTS session_log_sync (
			file_path TEXT PRIMARY KEY,
			last_modified INTEGER NOT NULL,
			last_line_offset INTEGER NOT NULL DEFAULT 0,
			last_synced_at INTEGER NOT NULL,
			last_byte_offset INTEGER,
			last_tail_fingerprint INTEGER,
			sync_semantics_version INTEGER NOT NULL DEFAULT 0,
			diagnostics_summary TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS session_usage_dedup (
			data_source TEXT NOT NULL,
			request_id TEXT NOT NULL,
			semantic_id TEXT NOT NULL,
			has_entry_id INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (data_source, request_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_session_usage_dedup_semantic ON session_usage_dedup(data_source, semantic_id, has_entry_id)`,
		`CREATE TABLE IF NOT EXISTS usage_daily_rollups (
			date TEXT NOT NULL,
			app_type TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			model TEXT NOT NULL,
			request_model TEXT NOT NULL DEFAULT '',
			pricing_model TEXT NOT NULL DEFAULT '',
			request_count INTEGER NOT NULL DEFAULT 0,
			success_count INTEGER NOT NULL DEFAULT 0,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			cache_read_tokens INTEGER NOT NULL DEFAULT 0,
			cache_creation_tokens INTEGER NOT NULL DEFAULT 0,
			reasoning_tokens INTEGER NOT NULL DEFAULT 0,
			input_token_semantics INTEGER NOT NULL DEFAULT 0,
			total_cost_usd TEXT NOT NULL DEFAULT '0',
			avg_latency_ms INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (date, app_type, provider_id, model, request_model, pricing_model)
		)`,
		`CREATE TABLE IF NOT EXISTS model_pricing (
			model_id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			input_cost_per_million TEXT NOT NULL,
			output_cost_per_million TEXT NOT NULL,
			cache_read_cost_per_million TEXT NOT NULL DEFAULT '0',
			cache_creation_cost_per_million TEXT NOT NULL DEFAULT '0'
		)`,
		`CREATE TABLE IF NOT EXISTS pi_sessions (
			session_id TEXT PRIMARY KEY,
			header_ts TEXT NOT NULL DEFAULT '',
			cwd TEXT NOT NULL DEFAULT '',
			file_name TEXT NOT NULL DEFAULT '',
			display_name TEXT NOT NULL DEFAULT '',
			is_task INTEGER NOT NULL DEFAULT 0,
			parent_session_id TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS source_sessions (
			data_source TEXT NOT NULL,
			physical_id TEXT NOT NULL,
			session_id TEXT NOT NULL DEFAULT '',
			thread_id TEXT NOT NULL DEFAULT '',
			timestamp_text TEXT NOT NULL DEFAULT '',
			cwd TEXT NOT NULL DEFAULT '',
			file_path TEXT NOT NULL DEFAULT '',
			file_name TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			provider_id TEXT NOT NULL DEFAULT '',
			originator TEXT NOT NULL DEFAULT '',
			cli_version TEXT NOT NULL DEFAULT '',
			parent_thread_id TEXT NOT NULL DEFAULT '',
			forked_from_id TEXT NOT NULL DEFAULT '',
			forked_from_ordinal INTEGER,
			subagent_history_start_ordinal INTEGER,
			history_base TEXT NOT NULL DEFAULT '',
			thread_source TEXT NOT NULL DEFAULT '',
			agent_role TEXT NOT NULL DEFAULT '',
			agent_path TEXT NOT NULL DEFAULT '',
			agent_nickname TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (data_source, physical_id)
		)`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("建表失败: %w", err)
		}
	}
	return nil
}

type migration struct {
	version int
	apply   func(*sql.Tx) error
}

var migrations = []migration{
	{version: 2, apply: migrateToLatest},
	{version: 3, apply: migrateToSourceRootBindings},
}

// migrate applies each schema version in its own transaction. A migration
// advances user_version only after every table, index, and column succeeds.
func migrate(database *sql.DB) error {
	version, err := readUserVersion(database)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version < 0 || version > SchemaVersion {
		return fmt.Errorf("unsupported schema version: %d", version)
	}
	for _, migration := range migrations {
		if version >= migration.version {
			continue
		}
		if err := applyMigration(database, migration); err != nil {
			return err
		}
		version = migration.version
	}
	// Older releases marked the schema as v2 before adding every final column.
	// Reconcile that shape transactionally without changing its public version.
	if version == SchemaVersion {
		if err := reconcileLatest(database); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(database *sql.DB, migration migration) error {
	tx, err := database.Begin()
	if err != nil {
		return fmt.Errorf("begin schema migration %d: %w", migration.version, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := migration.apply(tx); err != nil {
		return fmt.Errorf("apply schema migration %d: %w", migration.version, err)
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, migration.version)); err != nil {
		return fmt.Errorf("advance schema version to %d: %w", migration.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema migration %d: %w", migration.version, err)
	}
	committed = true
	return nil
}

func reconcileLatest(database *sql.DB) error {
	tx, err := database.Begin()
	if err != nil {
		return fmt.Errorf("begin schema reconciliation: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := migrateToLatest(tx); err != nil {
		return fmt.Errorf("reconcile latest schema: %w", err)
	}
	if err := migrateToSourceRootBindings(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema reconciliation: %w", err)
	}
	committed = true
	return nil
}

func migrateToSourceRootBindings(tx *sql.Tx) error {
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS source_root_bindings (
		data_source TEXT PRIMARY KEY,
		root_path TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create source_root_bindings: %w", err)
	}
	return nil
}

func migrateToLatest(tx *sql.Tx) error {
	if err := createLatestSchema(tx); err != nil {
		return err
	}
	for _, tableMigration := range []struct {
		table   string
		columns []struct{ name, definition string }
	}{
		{table: "proxy_request_logs", columns: []struct{ name, definition string }{
			{name: "physical_rollout_id", definition: `physical_rollout_id TEXT NOT NULL DEFAULT ''`},
			{name: "kind", definition: `kind TEXT NOT NULL DEFAULT 'assistant'`},
			{name: "stop_reason", definition: `stop_reason TEXT NOT NULL DEFAULT ''`},
			{name: "reasoning_tokens", definition: `reasoning_tokens INTEGER NOT NULL DEFAULT 0`},
			{name: "cwd", definition: `cwd TEXT NOT NULL DEFAULT ''`},
			{name: "timestamp_text", definition: `timestamp_text TEXT NOT NULL DEFAULT ''`},
		}},
		{table: "usage_daily_rollups", columns: []struct{ name, definition string }{
			{name: "reasoning_tokens", definition: `reasoning_tokens INTEGER NOT NULL DEFAULT 0`},
		}},
		{table: "session_log_sync", columns: []struct{ name, definition string }{
			{name: "sync_semantics_version", definition: `sync_semantics_version INTEGER NOT NULL DEFAULT 0`},
			{name: "diagnostics_summary", definition: `diagnostics_summary TEXT NOT NULL DEFAULT ''`},
		}},
		{table: "source_sessions", columns: []struct{ name, definition string }{
			{name: "subagent_history_start_ordinal", definition: `subagent_history_start_ordinal INTEGER`},
		}},
	} {
		if err := addMissingColumns(tx, tableMigration.table, tableMigration.columns); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_request_logs_physical ON proxy_request_logs(data_source, physical_rollout_id)`); err != nil {
		return fmt.Errorf("create physical request index: %w", err)
	}
	return nil
}

func addMissingColumns(tx *sql.Tx, table string, columns []struct{ name, definition string }) error {
	existing, err := tableColumns(tx, table)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", table, err)
	}
	for _, column := range columns {
		if existing[column.name] {
			continue
		}
		if _, err := tx.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column.definition); err != nil {
			return fmt.Errorf("add %s.%s: %w", table, column.name, err)
		}
	}
	return nil
}

func tableColumns(tx *sql.Tx, table string) (map[string]bool, error) {
	rows, err := tx.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func readUserVersion(database *sql.DB) (int, error) {
	var version int
	if err := database.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}
