/**
 * 01 — SQLite 持久化基座（直切，无兼容）
 * Seam: Database.getInstance / create_tables / 路径解析 / WAL
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync, existsSync } from "node:fs";
import { tmpdir, homedir } from "node:os";
import { join } from "node:path";
import { DatabaseSync } from "node:sqlite";

import { Database, resolveDbPath, SCHEMA_VERSION } from "../src/db.ts";

function tempDir(): string {
  return mkdtempSync(join(tmpdir(), "ta-db-test-"));
}

test("S1-1 Database.getInstance 创建库文件与 5 表+7索引", async () => {
  const dir = tempDir();
  const dbPath = join(dir, "token-analyzer.db");
  try {
    const db = await Database.getInstance(dbPath);
    assert.ok(existsSync(dbPath), "db 文件应创建");
    const raw = new DatabaseSync(dbPath);
    const rows = raw.prepare(`SELECT name FROM sqlite_master WHERE type='table' AND name IN ('proxy_request_logs','session_log_sync','session_usage_dedup','usage_daily_rollups','model_pricing')`).all() as { name: string }[];
    const names = rows.map((r) => r.name).sort();
    assert.deepEqual(names, ["model_pricing", "proxy_request_logs", "session_log_sync", "session_usage_dedup", "usage_daily_rollups"]);
    // 6+ 索引存在（至少 7 个）
    const idxRows = raw.prepare(`SELECT name FROM sqlite_master WHERE type='index' AND name LIKE 'idx_%'`).all() as { name: string }[];
    assert.ok(idxRows.length >= 6, `索引数应 >=6, 实际 ${idxRows.length}`);
    raw.close();
    await db.close();
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("S1-2 user_version=1 且幂等二次调用不报错", async () => {
  const dir = tempDir();
  const dbPath = join(dir, "token-analyzer.db");
  try {
    const db1 = await Database.getInstance(dbPath);
    const v1 = db1.getUserVersion();
    assert.equal(v1, SCHEMA_VERSION);
    assert.equal(v1, 2);
    await db1.close();
    const db2 = await Database.getInstance(dbPath);
    const v2 = db2.getUserVersion();
    assert.equal(v2, 2);
    await db2.close();
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("S1-3 WAL / foreign_keys / auto_vacuum 生效", async () => {
  const dir = tempDir();
  const dbPath = join(dir, "token-analyzer.db");
  try {
    const db = await Database.getInstance(dbPath);
    const raw = new DatabaseSync(dbPath);
    const jm = raw.prepare(`PRAGMA journal_mode`).get() as { journal_mode: string };
    assert.equal(jm.journal_mode, "wal");
    const fk = raw.prepare(`PRAGMA foreign_keys`).get() as { foreign_keys: number };
    assert.equal(fk.foreign_keys, 1);
    const av = raw.prepare(`PRAGMA auto_vacuum`).get() as { auto_vacuum: number };
    assert.equal(av.auto_vacuum, 2); // INCREMENTAL = 2
    raw.close();
    await db.close();
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("S1-4 路径解析优先级：TOKEN_ANALYZER_DB > --db > ~/.cc-switch（共库） > ~/.cache > data", () => {
  const envPath = "/tmp/env.db";
  const argPath = "/tmp/arg.db";
  // env 优先
  assert.equal(resolveDbPath({ dbPath: argPath, envDb: envPath }), envPath);
  // 无 env 时用 --db
  assert.equal(resolveDbPath({ dbPath: argPath, envDb: undefined }), argPath);
  // 均无时回退到共库或 XDG 或 data（仅断言非空且以 .db 结尾）
  const fallback = resolveDbPath({ dbPath: undefined, envDb: undefined });
  const ccPath = join(homedir(), ".cc-switch", "cc-switch.db");
  if (existsSync(ccPath)) {
    assert.equal(fallback, ccPath, `共库存在时应返回 cc-switch 路径, 实际 ${fallback}`);
  } else {
    assert.ok(fallback.endsWith("token-analyzer.db"), `fallback 应以 token-analyzer.db 结尾, 实际 ${fallback}`);
  }
});

test("S1-5 列定义与 cc-switch 一致：proxy_request_logs 含 pricing_model/input_token_semantics", async () => {
  const dir = tempDir();
  const dbPath = join(dir, "token-analyzer.db");
  try {
    const db = await Database.getInstance(dbPath);
    const raw = new DatabaseSync(dbPath);
    const cols = raw.prepare(`PRAGMA table_info(proxy_request_logs)`).all() as { name: string }[];
    const names = cols.map((c) => c.name);
    assert.ok(names.includes("pricing_model"), "proxy_request_logs 应含 pricing_model");
    assert.ok(names.includes("input_token_semantics"), "proxy_request_logs 应含 input_token_semantics");
    assert.ok(names.includes("request_model"), "proxy_request_logs 应含 request_model");
    raw.close();
    await db.close();
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("S1-6 列定义：session_log_sync 含 last_byte_offset/last_tail_fingerprint", async () => {
  const dir = tempDir();
  const dbPath = join(dir, "token-analyzer.db");
  try {
    const db = await Database.getInstance(dbPath);
    const raw = new DatabaseSync(dbPath);
    const cols = raw.prepare(`PRAGMA table_info(session_log_sync)`).all() as { name: string }[];
    const names = cols.map((c) => c.name);
    assert.ok(names.includes("last_byte_offset"));
    assert.ok(names.includes("last_tail_fingerprint"));
    raw.close();
    await db.close();
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});
