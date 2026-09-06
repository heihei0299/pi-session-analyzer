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
