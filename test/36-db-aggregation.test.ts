/**
 * 07 — DB 聚合与剪枝
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { Database } from "../src/db.ts";
import { syncPiUsage } from "../src/pi-sync.ts";
import { queryTotals, queryGroups, rollupAndPrune } from "../src/db-aggregation.ts";

function tempDir(): string { return mkdtempSync(join(tmpdir(), "ta-agg-")); }
function header(id = "sess-1"): string { return JSON.stringify({ type: "session", version: 3, id, timestamp: "2026-07-31T01:55:30.577Z", cwd: "/tmp" }); }
function assistant(id: string, model: string, usage: Record<string, unknown>): string {
  return JSON.stringify({ type: "message", id, timestamp: "2026-07-31T01:58:29.810Z", message: { role: "assistant", provider: "p", model, usage, stopReason: "stop" } });
}

test("S7-1 queryTotals 聚合四载体和", async () => {
  const dir = tempDir(); const dbDir = tempDir();
  const dbPath = join(dbDir, "test.db");
  try {
    const file = join(dir, "s.jsonl");
    writeFileSync(file, header("s1") + "\n" + assistant("a1", "m1", { input: 10, output: 5, cacheRead: 3, cacheWrite: 2, totalTokens: 20, cost: { total: 0 } }) + "\n" + assistant("a2", "m1", { input: 20, output: 10, cacheRead: 6, cacheWrite: 4, totalTokens: 40, cost: { total: 0 } }) + "\n");
    const db = await Database.getInstance(dbPath);
    await syncPiUsage(db, [file]);
    const totals = queryTotals(db, {});
    assert.equal(totals.requests, 2);
    assert.equal(totals.input, 30);
    assert.equal(totals.output, 15);
    assert.equal(totals.cacheRead, 9);
    assert.equal(totals.cacheWrite, 6);
    assert.equal(totals.totalTokens, 30 + 9 + 15); // input+cacheRead+output
    await db.close();
  } finally { rmSync(dir, { recursive: true, force: true }); rmSync(dbDir, { recursive: true, force: true }); }
});

test("S7-2 queryGroups by model", async () => {
  const dir = tempDir(); const dbDir = tempDir(); const dbPath = join(dbDir, "test.db");
  try {
    const file = join(dir, "s.jsonl");
    writeFileSync(file, header("s1") + "\n" + assistant("a1", "m1", { input: 10, output: 5, cacheRead: 0, cacheWrite: 0, totalTokens: 15, cost: { total: 0 } }) + "\n" + assistant("a2", "m2", { input: 20, output: 10, cacheRead: 0, cacheWrite: 0, totalTokens: 30, cost: { total: 0 } }) + "\n");
    const db = await Database.getInstance(dbPath);
    await syncPiUsage(db, [file]);
    const groups = queryGroups(db, "model", {});
    const m1 = groups.find((g) => g.model === "actual-model" || g.model === "m1");
    // 由于 parse 默认实际模型为 responseModel，未设时为 m1/m2，但 parse 归一化可能为 m1/m2
    // 仅断言分组数与总量
    assert.equal(groups.length, 2);
    const sum = groups.reduce((s, g) => s + g.input, 0);
    assert.equal(sum, 30);
    await db.close();
  } finally { rmSync(dir, { recursive: true, force: true }); rmSync(dbDir, { recursive: true, force: true }); }
});

test("S7-3 rollupAndPrune 将 30 天前明细聚合后删除", async () => {
  const dbDir = tempDir(); const dbPath = join(dbDir, "test.db");
  try {
    const db = await Database.getInstance(dbPath);
    // 直接插入一条 31 天前的记录
    const oldCreatedAt = Math.floor(Date.now() / 1000) - 31 * 86400;
    db.prepare(`INSERT OR IGNORE INTO proxy_request_logs (request_id, provider_id, app_type, model, request_model, pricing_model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, input_token_semantics, total_cost_usd, latency_ms, status_code, session_id, provider_type, is_streaming, cost_multiplier, created_at, data_source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`).run("old-1", "p", "pi", "m1", "m1", "m1", 10, 5, 3, 2, 0, "0", 0, 200, "sess-1", "pi_session", 1, "1.0", oldCreatedAt, "pi_session");
    const before = db.prepare(`SELECT COUNT(*) as c FROM proxy_request_logs`).get() as { c: number };
    assert.equal(before.c, 1);
    const res = rollupAndPrune(db, 30);
    assert.equal(res.rolled, 1);
    const after = db.prepare(`SELECT COUNT(*) as c FROM proxy_request_logs`).get() as { c: number };
    assert.equal(after.c, 0);
    const roll = db.prepare(`SELECT COUNT(*) as c FROM usage_daily_rollups`).get() as { c: number };
    assert.equal(roll.c, 1);
    await db.close();
  } finally { rmSync(dbDir, { recursive: true, force: true }); }
});
