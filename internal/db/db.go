package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const SchemaVersion = 2

// Database 封装 sql.DB，对应 cc-switch schema.rs
type Database struct {
	DB   *sql.DB
	Path string
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			if p == "~" {
				return home
			}
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// ResolveDbPath 按优先级解析：envDb > dbPath > ~/.cache > data（默认不共库，显式 env/--db 才共库，空白归一）
func ResolveDbPath(dbPath, envDb string) string {
	if strings.TrimSpace(envDb) != "" {
		return expandHome(strings.TrimSpace(envDb))
	}
	if strings.TrimSpace(dbPath) != "" {
		return expandHome(strings.TrimSpace(dbPath))
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" && home != "/" {
		return filepath.Join(home, ".cache", "token-analyzer", "token-analyzer.db")
	}
	return filepath.Join("data", "token-analyzer.db")
}

func ResolveDbPathFromEnv(dbPath string) string {
	return ResolveDbPath(dbPath, os.Getenv("TOKEN_ANALYZER_DB"))
}

// Open 打开或创建 DB 文件，建表并设置 PRAGMA
func Open(path string) (*Database, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		// 回退到 data/
		fallback := filepath.Join("data", "token-analyzer.db")
		if err2 := os.MkdirAll(filepath.Dir(fallback), 0o755); err2 != nil {
			return nil, fmt.Errorf("创建 DB 目录失败: %w", err)
		}
		path = fallback
	}
	// modernc sqlite 需要 file: 前缀
	dsn := fmt.Sprintf("file:%s?cache=shared", path)
	// busy_timeout 通过 PRAGMA 设置
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// PRAGMA 顺序：auto_vacuum 必须在 journal_mode 之前且在建表前
	if _, err := sqlDB.Exec(`PRAGMA auto_vacuum = INCREMENTAL`); err != nil {
		// 忽略，已存在表时无法修改
	}
	if _, err := sqlDB.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		return nil, err
	}
	if _, err := sqlDB.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return nil, err
	}
	if _, err := sqlDB.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		return nil, err
	}
	if _, err := sqlDB.Exec(`PRAGMA synchronous = NORMAL`); err != nil {
		return nil, err
	}
	db := &Database{DB: sqlDB, Path: path}
	if err := db.createTables(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	if err := db.ensureColumns(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	if err := db.ensureUserVersion(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// GetInstance 单例入口（简化：每次 Open）
func GetInstance(dbPath string) (*Database, error) {
	resolved := ResolveDbPathFromEnv(dbPath)
	return Open(resolved)
}

// OpenReadOnly 以只读方式打开已存在的 ledger，供 Query 快照读取。
// 不建目录、不建表、不执行任何写 PRAGMA；文件缺失直接报错。
// query_only 兜底：Query 路径即便有 bug 也写不进 DB。
func OpenReadOnly(path string) (*Database, error) {
	// 路径经 URL 转义含入 URI：含空格/?/# 的路径也不走样。
	uri := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	sqlDB, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	if _, err := sqlDB.Exec(`PRAGMA query_only = ON`); err != nil {
		sqlDB.Close()
		return nil, err
	}
	if _, err := sqlDB.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		sqlDB.Close()
		return nil, err
	}
	// 缺失文件在这里现形：只读打开不存在的库要到首次访问才报错，
	// 提前碰一下 schema 给出明确错误（空 ledger 也是合法快照，不误报）。
	var tables int
	if err := sqlDB.QueryRow(`SELECT count(*) FROM sqlite_master`).Scan(&tables); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("只读 ledger 不可用 %s: %w", path, err)
	}
	return &Database{DB: sqlDB, Path: path}, nil
}

func (d *Database) createTables() error {
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
		if _, err := d.DB.Exec(s); err != nil {
			return fmt.Errorf("建表失败: %w", err)
		}
	}
	return nil
}
func (d *Database) ensureColumns() error {
	rows, err := d.DB.Query(`PRAGMA table_info(proxy_request_logs)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	names := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return err
		}
		names[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	add := []struct{ col, ddl string }{
		{"kind", `kind TEXT NOT NULL DEFAULT 'assistant'`},
		{"reasoning_tokens", `reasoning_tokens INTEGER NOT NULL DEFAULT 0`},
		{"cwd", `cwd TEXT NOT NULL DEFAULT ''`},
		{"timestamp_text", `timestamp_text TEXT NOT NULL DEFAULT ''`},
		{"physical_rollout_id", `physical_rollout_id TEXT NOT NULL DEFAULT ''`},
	}
	for _, a := range add {
		if !names[a.col] {
			if _, err := d.DB.Exec(fmt.Sprintf(`ALTER TABLE proxy_request_logs ADD COLUMN %s`, a.ddl)); err != nil {
				return fmt.Errorf("补列 %s 失败: %w", a.col, err)
			}
		}
	}
	if _, err := d.DB.Exec(`CREATE INDEX IF NOT EXISTS idx_request_logs_physical ON proxy_request_logs(data_source, physical_rollout_id)`); err != nil {
		return err
	}
	rows, err = d.DB.Query(`PRAGMA table_info(source_sessions)`)
	if err != nil {
		return err
	}
	sourceColumns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		sourceColumns[name] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	if !sourceColumns["subagent_history_start_ordinal"] {
		if _, err := d.DB.Exec(`ALTER TABLE source_sessions ADD COLUMN subagent_history_start_ordinal INTEGER`); err != nil {
			return err
		}
	}
	rows, err = d.DB.Query(`PRAGMA table_info(session_log_sync)`)
	if err != nil {
		return err
	}
	syncColumns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		syncColumns[name] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	// per-file 诊断摘要：游标命中的文件不再重扫，诊断必须能重放，否则覆盖率缺口只在首次查询可见。
	if !syncColumns["diagnostics_summary"] {
		if _, err := d.DB.Exec(`ALTER TABLE session_log_sync ADD COLUMN diagnostics_summary TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	return nil
}

func (d *Database) ensureUserVersion() error {
	var v int
	if err := d.DB.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v < SchemaVersion {
		if _, err := d.DB.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, SchemaVersion)); err != nil {
			return err
		}
	}
	return nil
}

func (d *Database) GetUserVersion() (int, error) {
	var v int
	if err := d.DB.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}

func (d *Database) Close() error {
	return d.DB.Close()
}
