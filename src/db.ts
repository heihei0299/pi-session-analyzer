/**
 * SQLite 持久化基座（直切 cc-switch，无兼容）
 * - 路径：TOKEN_ANALYZER_DB > --db > ~/.cache/token-analyzer/token-analyzer.db
 * - 引擎：node:sqlite DatabaseSync
 * - 5 表 1:1 与 cc-switch schema.rs
 */
import { DatabaseSync } from "node:sqlite";
import { mkdirSync, existsSync } from "node:fs";
import { homedir } from "node:os";
import { join, dirname } from "node:path";

export const SCHEMA_VERSION = 2;

/** 路径解析：envDb（TOKEN_ANALYZER_DB）优先，其次 dbPath（--db），最后 XDG 回退（默认不共库，显式 env/--db 才共库，空白归一） */
export function resolveDbPath(opts: { dbPath?: string; envDb?: string }): string {
  if (opts.envDb && opts.envDb.trim() !== "") return opts.envDb.trim();
  if (opts.dbPath && opts.dbPath.trim() !== "") return opts.dbPath.trim();
  // XDG 回退（默认不共库）
  try {
    const cache = join(homedir(), ".cache", "token-analyzer", "token-analyzer.db");
    // 若 homedir 可用则用它，否则回退 data/
    if (homedir() && homedir() !== "/") return cache;
  } catch {
    // ignore
  }
  return join("data", "token-analyzer.db");
}

export function resolveDbPathFromEnv(dbPath?: string): string {
  const envDb = process.env.TOKEN_ANALYZER_DB;
  return resolveDbPath({ dbPath, envDb });
}

export class Database {
  private raw: DatabaseSync;
  private path: string;

  private constructor(raw: DatabaseSync, path: string) {
    this.raw = raw;
    this.path = path;
  }

  static async getInstance(dbPath?: string): Promise<Database> {
    const resolved = resolveDbPathFromEnv(dbPath);
    const dir = dirname(resolved);
    try {
      mkdirSync(dir, { recursive: true });
    } catch {
      // 回退到 data/
      const fallback = join("data", "token-analyzer.db");
      const fallbackDir = dirname(fallback);
      mkdirSync(fallbackDir, { recursive: true });
      return Database.open(fallback);
    }
    return Database.open(resolved);
  }

  /** 内存库（读路径按目录隔离：每次全量同步后聚合，不污染持久库） */
  static async memory(): Promise<Database> {
    return Database.open(":memory:");
  }

  private static open(path: string): Database {
    const dir = dirname(path);
    if (!existsSync(dir)) mkdirSync(dir, { recursive: true });
    const raw = new DatabaseSync(path);
    // auto_vacuum 必须在建表前且在 journal_mode 之前设置，否则 WAL 后无法修改（实测）
    try {
      const av = raw.prepare(`PRAGMA auto_vacuum`).get() as { auto_vacuum: number } | undefined;
      if (av && av.auto_vacuum !== 2) {
        raw.exec(`PRAGMA auto_vacuum = INCREMENTAL`);
      }
    } catch {}
    try {
      raw.exec(`PRAGMA journal_mode = WAL`);
    } catch {}
    try {
      raw.exec(`PRAGMA foreign_keys = ON`);
    } catch {}
    try {
      raw.exec(`PRAGMA busy_timeout = 5000`);
    } catch {}
    try {
      raw.exec(`PRAGMA synchronous = NORMAL`);
    } catch {}
    const db = new Database(raw, path);
    db.createTables();
    db.ensureColumns();
    db.ensureUserVersion();
    return db;
  }

  private createTables(): void {
    // proxy_request_logs
    this.raw.exec(`
      CREATE TABLE IF NOT EXISTS proxy_request_logs (
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
        kind TEXT NOT NULL DEFAULT 'assistant',
        reasoning_tokens INTEGER NOT NULL DEFAULT 0,
        cwd TEXT NOT NULL DEFAULT '',
        timestamp_text TEXT NOT NULL DEFAULT ''
      )
    `);
    this.raw.exec(`CREATE INDEX IF NOT EXISTS idx_request_logs_provider ON proxy_request_logs(provider_id, app_type)`);
    this.raw.exec(`CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON proxy_request_logs(created_at)`);
    this.raw.exec(`CREATE INDEX IF NOT EXISTS idx_request_logs_model ON proxy_request_logs(model)`);
    this.raw.exec(`CREATE INDEX IF NOT EXISTS idx_request_logs_session ON proxy_request_logs(session_id)`);
    this.raw.exec(`CREATE INDEX IF NOT EXISTS idx_request_logs_status ON proxy_request_logs(status_code)`);
    // 兼容 cc-switch 的 usage 复合索引（若支持）
    try {
      this.raw.exec(`CREATE INDEX IF NOT EXISTS idx_request_logs_usage ON proxy_request_logs(app_type, data_source, created_at)`);
    } catch {}

    // session_log_sync
    this.raw.exec(`
      CREATE TABLE IF NOT EXISTS session_log_sync (
        file_path TEXT PRIMARY KEY,
        last_modified INTEGER NOT NULL,
        last_line_offset INTEGER NOT NULL DEFAULT 0,
        last_synced_at INTEGER NOT NULL,
        last_byte_offset INTEGER,
        last_tail_fingerprint INTEGER
      )
    `);

    // session_usage_dedup
    this.raw.exec(`
      CREATE TABLE IF NOT EXISTS session_usage_dedup (
        data_source TEXT NOT NULL,
        request_id TEXT NOT NULL,
        semantic_id TEXT NOT NULL,
        has_entry_id INTEGER NOT NULL DEFAULT 0,
        PRIMARY KEY (data_source, request_id)
      )
    `);
    this.raw.exec(`CREATE INDEX IF NOT EXISTS idx_session_usage_dedup_semantic ON session_usage_dedup(data_source, semantic_id, has_entry_id)`);

    // usage_daily_rollups
    this.raw.exec(`
      CREATE TABLE IF NOT EXISTS usage_daily_rollups (
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
      )
    `);

    // model_pricing
    this.raw.exec(`
      CREATE TABLE IF NOT EXISTS model_pricing (
        model_id TEXT PRIMARY KEY,
        display_name TEXT NOT NULL,
        input_cost_per_million TEXT NOT NULL,
        output_cost_per_million TEXT NOT NULL,
        cache_read_cost_per_million TEXT NOT NULL DEFAULT '0',
        cache_creation_cost_per_million TEXT NOT NULL DEFAULT '0'
      )
    `);

    // pi_sessions（cc-switch 外的扩展：会话级元数据，供 sessions/detail 窗口）
    this.raw.exec(`
      CREATE TABLE IF NOT EXISTS pi_sessions (
        session_id TEXT PRIMARY KEY,
        header_ts TEXT NOT NULL DEFAULT '',
        cwd TEXT NOT NULL DEFAULT '',
        file_name TEXT NOT NULL DEFAULT '',
        display_name TEXT NOT NULL DEFAULT '',
        is_task INTEGER NOT NULL DEFAULT 0,
        parent_session_id TEXT
      )
    `);
  }

  /** v1→v2 迁移：已存在的 proxy_request_logs 补扩展列 */
  private ensureColumns(): void {
    const cols = this.raw.prepare(`PRAGMA table_info(proxy_request_logs)`).all() as { name: string }[];
    const names = new Set(cols.map((c) => c.name));
    const add = (col: string, ddl: string): void => {
      if (!names.has(col)) this.raw.exec(`ALTER TABLE proxy_request_logs ADD COLUMN ${ddl}`);
    };
    add("kind", `kind TEXT NOT NULL DEFAULT 'assistant'`);
    add("reasoning_tokens", `reasoning_tokens INTEGER NOT NULL DEFAULT 0`);
    add("cwd", `cwd TEXT NOT NULL DEFAULT ''`);
    add("timestamp_text", `timestamp_text TEXT NOT NULL DEFAULT ''`);
  }

  private ensureUserVersion(): void {
    const row = this.raw.prepare(`PRAGMA user_version`).get() as { user_version: number } | undefined;
    const v = row?.user_version ?? 0;
    if (v < SCHEMA_VERSION) {
      this.raw.exec(`PRAGMA user_version = ${SCHEMA_VERSION}`);
    }
  }

  getUserVersion(): number {
    const row = this.raw.prepare(`PRAGMA user_version`).get() as { user_version: number } | undefined;
    return row?.user_version ?? 0;
  }

  getDbPath(): string {
    return this.path;
  }

  exec(sql: string): void {
    this.raw.exec(sql);
  }

  prepare(sql: string): ReturnType<DatabaseSync["prepare"]> {
    return this.raw.prepare(sql);
  }

  async close(): Promise<void> {
    try {
      this.raw.close();
    } catch {}
  }

  // 兼容测试的 exec 代理
  getRaw(): DatabaseSync {
    return this.raw;
  }
}
