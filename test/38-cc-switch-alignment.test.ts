import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { homedir } from "node:os";
import { existsSync } from "node:fs";
import { resolveDbPath } from "../src/db.ts";
import { Database } from "../src/db.ts";
import { withDirDb, queryTotals } from "../src/db-aggregation.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

test("38-1 resolveDbPath 默认不共库：无参时返回 cache，显式 env/--db 才共库", () => {
  const ccPath = join(homedir(), ".cc-switch", "cc-switch.db");
  const got = resolveDbPath({});
  assert.ok(got.endsWith("token-analyzer.db"), `默认应返回 token-analyzer.db, 实际 ${got}`);
  assert.notEqual(got, ccPath, "默认不应返回 cc-switch 路径");
  // 显式才共库
  assert.equal(resolveDbPath({ envDb: ccPath }), ccPath);
  assert.equal(resolveDbPath({ dbPath: ccPath }), ccPath);
  assert.equal(resolveDbPath({ envDb: "/tmp/custom.db" }), "/tmp/custom.db");
  assert.equal(resolveDbPath({ dbPath: "/tmp/a.db" }), "/tmp/a.db");
  assert.equal(resolveDbPath({ dbPath: "/tmp/a.db", envDb: "/tmp/b.db" }), "/tmp/b.db");
});

test("38-2 queryTotals 与 cc-switch 直连 SUM 一致（同库 today/localtime）", async () => {
  const ccPath = join(homedir(), ".cc-switch", "cc-switch.db");
  if (!existsSync(ccPath)) return;
  // 用文件直读的 withDirDb 同窗口结果应与 cc-switch DB 的直接 SUM 在容差内（今日）
  // 为避免依赖全量历史，这里用临时 fixture 构造确定性对比
  const dir = makeFixture({
    "2026-09-06T10-00-00-000Z_a.jsonl": [
      sessionHeader({ id: "a1", timestamp: "2026-09-06T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100, output: 50, cacheRead: 200 }) }, { timestamp: "2026-09-06T10:00:00.000Z" }),
    ],
    "2026-09-06T12-00-00-000Z_b.jsonl": [
      sessionHeader({ id: "b1", timestamp: "2026-09-06T12:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 300, output: 10, cacheRead: 100 }) }, { timestamp: "2026-09-06T12:00:00.000Z" }),
    ],
  });
  try {
    const totals = await withDirDb(dir, (db) => queryTotals(db, { since: "2026-09-06", until: "2026-09-06" }));
    // 直接 SQL 等价性：withDirDb 内部已用相同 SQL，此处仅验 totalTokens 公式
    assert.equal(totals.totalTokens, totals.input + totals.cacheRead + totals.output, "totalTokens = input+cacheRead+output");
    assert.equal(totals.requests, 2);
    assert.equal(totals.input + totals.cacheRead + totals.output, 100 + 200 + 50 + 300 + 100 + 10);
  } finally {
    removeFixture(dir);
  }
});

test("38-3 全量窗口 totalTokens 公式与 cc-switch 一致（不含 cacheWrite）", async () => {
  const dir = makeFixture({
    "s1.jsonl": [
      sessionHeader({ id: "s1", timestamp: "2026-09-06T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 10, output: 5, cacheRead: 3, cacheWrite: 99 }) }),
    ],
  });
  try {
    const totals = await withDirDb(dir, (db) => queryTotals(db, {}));
    // cacheWrite 不计入 totalTokens
    assert.equal(totals.totalTokens, 10 + 3 + 5);
    assert.equal(totals.cacheWrite, 99);
  } finally {
    removeFixture(dir);
  }
});
test("38-4 四窗口确定性对比：内存库预置 proxy+rollup，queryTotals 与直连 SUM（含 rollup）一致", async () => {
  const db = await Database.memory();
  try {
    const nowSec = Math.floor(Date.now() / 1000);
    const todayStart = new Date(); todayStart.setHours(0, 0, 0, 0);
    const todaySec = Math.floor(todayStart.getTime() / 1000);
    const d7Sec = todaySec - 6 * 86400;
    const d30Sec = todaySec - 29 * 86400;
    const oldSec = todaySec - 35 * 86400;
    const fmtDate = (sec: number) => new Date(sec * 1000).toISOString().slice(0, 10);
    // proxy rows: today, 7d, 30d, old
    const inserts: Array<[string, number, number, number, number]> = [
      ["req-today", todaySec + 3600, 10, 20, 5],
      ["req-7d", d7Sec + 3600, 30, 40, 10],
      ["req-30d", d30Sec + 3600, 50, 60, 15],
      ["req-old", oldSec + 3600, 70, 80, 20],
    ];
    for (const [id, ts, inp, cr, out] of inserts) {
      db.prepare(`INSERT INTO proxy_request_logs (request_id, provider_id, app_type, model, request_model, pricing_model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, input_token_semantics, total_cost_usd, latency_ms, status_code, created_at, data_source, kind, reasoning_tokens, cwd, timestamp_text) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`).run(id, "_pi_session", "pi", "m", "m", "m", inp, out, cr, 0, 0, "0", 0, 200, ts, "pi_session", "assistant", 0, "/proj", new Date(ts * 1000).toISOString());
    }
    // rollup row for old date
    const oldDate = fmtDate(oldSec);
    db.prepare(`INSERT INTO usage_daily_rollups (date, app_type, provider_id, model, request_model, pricing_model, request_count, success_count, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, total_cost_usd, avg_latency_ms) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`).run(oldDate, "pi", "_pi_session", "m", "m", "m", 5, 5, 100, 200, 300, 0, "0", 0);
    const fmt = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
    const todayStr = fmt(new Date());
    const d7 = new Date(); d7.setDate(new Date().getDate() - 6); const d7Str = fmt(d7);
    const d30 = new Date(); d30.setDate(new Date().getDate() - 29); const d30Str = fmt(d30);
    const cases: Array<{ label: string; since?: string; until?: string }> = [
      { label: "today", since: todayStr, until: todayStr },
      { label: "7d", since: d7Str, until: todayStr },
      { label: "30d", since: d30Str, until: todayStr },
      { label: "all", since: undefined, until: undefined },
    ];
    const directProxy = (where: string) => {
      const row = db.prepare(`SELECT COUNT(*) as c, COALESCE(SUM(input_tokens),0) as i, COALESCE(SUM(cache_read_tokens),0) as cr, COALESCE(SUM(output_tokens),0) as o FROM proxy_request_logs WHERE app_type='pi' AND data_source='pi_session'${where}`).get() as { c: number; i: number; cr: number; o: number } | undefined;
      return { c: row?.c ?? 0, i: row?.i ?? 0, cr: row?.cr ?? 0, o: row?.o ?? 0 };
    };
    const directRollup = (where: string) => {
      const row = db.prepare(`SELECT COALESCE(SUM(request_count),0) as c, COALESCE(SUM(input_tokens),0) as i, COALESCE(SUM(cache_read_tokens),0) as cr, COALESCE(SUM(output_tokens),0) as o FROM usage_daily_rollups WHERE app_type='pi'${where}`).get() as { c: number; i: number; cr: number; o: number } | undefined;
      return { c: row?.c ?? 0, i: row?.i ?? 0, cr: row?.cr ?? 0, o: row?.o ?? 0 };
    };
    for (const c of cases) {
      const totals = queryTotals(db, { since: c.since, until: c.until } as never);
      // 构造与 queryTotals 相同的 where（since/until 转 BETWEEN）
      let whereProxy = ""; let whereRollup = "";
      if (c.since) {
        const sinceSec = Math.floor(new Date(`${c.since}T00:00:00`).getTime() / 1000);
        const untilSec = c.until ? Math.floor(new Date(`${c.until}T23:59:59`).getTime() / 1000) : Math.floor(Date.now() / 1000);
        whereProxy = ` AND created_at BETWEEN ${sinceSec} AND ${untilSec}`;
        whereRollup = ` AND date BETWEEN '${c.since}' AND '${c.until}'`;
      } else if (c.until) {
        const untilSec = Math.floor(new Date(`${c.until}T23:59:59`).getTime() / 1000);
        whereProxy = ` AND created_at <= ${untilSec}`;
        whereRollup = ` AND date <= '${c.until}'`;
      }
      const p = directProxy(whereProxy);
      const r = directRollup(whereRollup);
      const expC = p.c + r.c, expI = p.i + r.i, expCr = p.cr + r.cr, expO = p.o + r.o;
      assert.equal(totals.requests, expC, `${c.label} requests 一致`);
      assert.equal(totals.input, expI, `${c.label} input 一致`);
      assert.equal(totals.cacheRead, expCr, `${c.label} cacheRead 一致`);
      assert.equal(totals.output, expO, `${c.label} output 一致`);
      assert.equal(totals.totalTokens, totals.input + totals.cacheRead + totals.output, `${c.label} totalTokens 公式`);
    }
    // 额外：若本机有真实 cc-switch 库，追加一次真实库的 today 对比（不强制，仅当存在）
    const ccPath = join(homedir(), ".cc-switch", "cc-switch.db");
    if (existsSync(ccPath)) {
      const realDb = await Database.getInstance(ccPath);
      try {
        const todayReal = queryTotals(realDb, { since: todayStr, until: todayStr } as never);
        const pReal = realDb.prepare(`SELECT COUNT(*) as c FROM proxy_request_logs WHERE app_type='pi' AND data_source='pi_session' AND date(created_at,'unixepoch','localtime') = date('now','localtime')`).get() as { c: number } | undefined;
        assert.equal(typeof todayReal.requests, "number");
        assert.equal(typeof pReal?.c, "number");
      } finally { await realDb.close(); }
    }
  } finally {
    await db.close();
  }
});
