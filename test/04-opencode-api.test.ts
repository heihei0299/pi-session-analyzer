/**
 * Ticket 04 — OpenCode HTTP API 端点
 * Seams: T1 costs, T2 history, T3 audit, T4 sync, T5 统一错误
 * 直接调用 handleApi（不启真实 server），用临时 dataDir + 环境变量注入 + mock client/fetch
 */
import { test, after } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { handleApi, __resetOpencodeSyncLockForTest } from "../src/api.ts";
import { OpenCodeStorage } from "../src/opencode/storage.ts";
import { OpenCodeClient } from "../src/opencode/client.ts";
import type { OpenCodeCostsResult, OpenCodeUsageRecord } from "../src/opencode/types.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

// helpers
function tmpDir(): string {
  return mkdtempSync(join(tmpdir(), "opencode-api-test-"));
}
function cleanup(dir: string) {
  rmSync(dir, { recursive: true, force: true });
}
function rec(overrides: Partial<OpenCodeUsageRecord> & { id: string; timeCreated: string }): OpenCodeUsageRecord {
  return {
    workspaceID: "wrk_test",
    timeUpdated: overrides.timeCreated,
    timeDeleted: null,
    model: "x-preview-f-free",
    provider: "inf.oa-compat",
    inputTokens: 100,
    outputTokens: 50,
    reasoningTokens: 10,
    cacheReadTokens: 5,
    cacheWrite5mTokens: null,
    cacheWrite1hTokens: null,
    cost: 0.001,
    keyID: "key_1",
    sessionID: "sess_001",
    enrichment: null,
    ...overrides,
  } as OpenCodeUsageRecord;
}
function costsResult(overrides?: Partial<OpenCodeCostsResult>): OpenCodeCostsResult {
  return {
    usage: [{ date: "2026-08-01", model: "x-preview", totalCost: 891912946, keyId: "key_1", plan: "lite" }],
    keys: [{ id: "key_1", name: "default" }],
    ...overrides,
  };
}

// env helper
function withOpencodeEnv(opencodeDir: string, extra?: Record<string, string>): { restore: () => void } {
  const prevDataDir = process.env.OPENCODE_DATA_DIR;
  const prevAuth = process.env.OPENCODE_AUTH;
  const prevWid = process.env.OPENCODE_WORKSPACE_ID;
  process.env.OPENCODE_DATA_DIR = opencodeDir;
  if (extra) {
    for (const [k, v] of Object.entries(extra)) process.env[k] = v;
  }
  return {
    restore() {
      if (prevDataDir === undefined) delete process.env.OPENCODE_DATA_DIR; else process.env.OPENCODE_DATA_DIR = prevDataDir;
      if (prevAuth === undefined) delete process.env.OPENCODE_AUTH; else process.env.OPENCODE_AUTH = prevAuth;
      if (prevWid === undefined) delete process.env.OPENCODE_WORKSPACE_ID; else process.env.OPENCODE_WORKSPACE_ID = prevWid;
    },
  };
}

// ---------- T1 costs ----------

test("T1 costs 正常返回 {year, month, costs}", async () => {
  const opDir = tmpDir();
  const { restore } = withOpencodeEnv(opDir);
  try {
    const storage = new OpenCodeStorage(opDir);
    const expected = costsResult({ usage: [{ date: "2026-08-01", model: "m1", totalCost: 100, keyId: "k1", plan: "lite" }] });
    await storage.saveCosts(2026, 8, expected);
    const sessionDir = makeFixture({}); // audit not needed but dir required
    try {
      const res = await handleApi("GET", "/api/opencode/costs", new URLSearchParams("year=2026&month=8"), sessionDir);
      assert.equal(res.status, 200);
      const body = res.body as { year: number; month: number; costs: OpenCodeCostsResult };
      assert.equal(body.year, 2026);
      assert.equal(body.month, 8);
      assert.deepEqual(body.costs, expected);
    } finally { removeFixture(sessionDir); }
  } finally { restore(); cleanup(opDir); __resetOpencodeSyncLockForTest(); }
});

test("T1 costs 缺失参数 400", async () => {
  const opDir = tmpDir();
  const { restore } = withOpencodeEnv(opDir);
  const sessionDir = makeFixture({});
  try {
    for (const qs of ["", "year=2026", "month=8", "year=abcd&month=8"]) {
      const res = await handleApi("GET", "/api/opencode/costs", new URLSearchParams(qs), sessionDir);
      assert.equal(res.status, 400, `${qs} 应 400`);
      const body = res.body as { error: string; detail: string };
      assert.ok(body.error.length > 0);
      assert.ok(body.detail.length > 0);
    }
  } finally { removeFixture(sessionDir); restore(); cleanup(opDir); }
});

test("T1 costs 非法 year/month 格式 400", async () => {
  const opDir = tmpDir();
  const { restore } = withOpencodeEnv(opDir);
  const sessionDir = makeFixture({});
  try {
    const cases = [
      "year=20&month=8",
      "year=2026&month=13",
      "year=2026&month=0",
      "year=20266&month=8",
      "year=2026&month=abc",
    ];
    for (const qs of cases) {
      const res = await handleApi("GET", "/api/opencode/costs", new URLSearchParams(qs), sessionDir);
      assert.equal(res.status, 400, `${qs} 应 400`);
    }
  } finally { removeFixture(sessionDir); restore(); cleanup(opDir); }
});

test("T1 costs 未找到时 404", async () => {
  const opDir = tmpDir();
  const { restore } = withOpencodeEnv(opDir);
  const sessionDir = makeFixture({});
  try {
    const res = await handleApi("GET", "/api/opencode/costs", new URLSearchParams("year=2026&month=9"), sessionDir);
    assert.equal(res.status, 404);
    const body = res.body as { error: string; detail: string };
    assert.equal(body.error, "Not Found");
    assert.ok(body.detail.includes("2026-09") || body.detail.includes("未找到"));
  } finally { removeFixture(sessionDir); restore(); cleanup(opDir); }
});

// ---------- T2 history ----------

test("T2 history 分页与总数正确", async () => {
  const opDir = tmpDir();
  const { restore } = withOpencodeEnv(opDir);
  try {
    const storage = new OpenCodeStorage(opDir);
    const r1 = rec({ id: "usg_1", timeCreated: "2026-08-22T00:00:00.000Z" });
    const r2 = rec({ id: "usg_2", timeCreated: "2026-08-21T00:00:00.000Z" });
    const r3 = rec({ id: "usg_3", timeCreated: "2026-08-20T00:00:00.000Z" });
    await storage.mergeHistory([r1, r2, r3]);
    const sessionDir = makeFixture({});
    try {
      // page 1 size 2
      const res = await handleApi("GET", "/api/opencode/history", new URLSearchParams("page=1&size=2"), sessionDir);
      assert.equal(res.status, 200);
      const body = res.body as { rows: OpenCodeUsageRecord[]; total: number; page: number; size: number };
      assert.equal(body.total, 3);
      assert.equal(body.page, 1);
      assert.equal(body.size, 2);
      assert.equal(body.rows.length, 2);
      // 逆序：最新在前
      assert.equal(body.rows[0].id, "usg_1");
      // page 2
      const res2 = await handleApi("GET", "/api/opencode/history", new URLSearchParams("page=2&size=2"), sessionDir);
      assert.equal((res2.body as any).rows.length, 1);
      assert.equal((res2.body as any).rows[0].id, "usg_3");
      // 无分页：返回全部
      const resAll = await handleApi("GET", "/api/opencode/history", new URLSearchParams(""), sessionDir);
      assert.equal(resAll.status, 200);
      assert.equal((resAll.body as any).total, 3);
      assert.equal((resAll.body as any).rows.length, 3);
    } finally { removeFixture(sessionDir); }
  } finally { restore(); cleanup(opDir); }
});

test("T2 history 按 model 精确过滤", async () => {
  const opDir = tmpDir();
  const { restore } = withOpencodeEnv(opDir);
  try {
    const s = new OpenCodeStorage(opDir);
    await s.mergeHistory([
      rec({ id: "usg_a", timeCreated: "2026-08-20T00:00:00.000Z", model: "m1" }),
      rec({ id: "usg_b", timeCreated: "2026-08-21T00:00:00.000Z", model: "m2" }),
      rec({ id: "usg_c", timeCreated: "2026-08-22T00:00:00.000Z", model: "m1" }),
    ]);
    const sessionDir = makeFixture({});
    try {
      const res = await handleApi("GET", "/api/opencode/history", new URLSearchParams("model=m1"), sessionDir);
      assert.equal(res.status, 200);
      const body = res.body as { rows: OpenCodeUsageRecord[]; total: number };
      assert.equal(body.total, 2);
      assert.ok(body.rows.every((r) => r.model === "m1"));
    } finally { removeFixture(sessionDir); }
  } finally { restore(); cleanup(opDir); }
});

test("T2 history 按 session 精确过滤（兼容 session/sessionID）", async () => {
  const opDir = tmpDir();
  const { restore } = withOpencodeEnv(opDir);
  try {
    const s = new OpenCodeStorage(opDir);
    await s.mergeHistory([
      rec({ id: "usg_1", timeCreated: "2026-08-20T00:00:00.000Z", sessionID: "sess_A" }),
      rec({ id: "usg_2", timeCreated: "2026-08-21T00:00:00.000Z", sessionID: "sess_B" }),
      rec({ id: "usg_3", timeCreated: "2026-08-22T00:00:00.000Z", sessionID: "sess_A" }),
    ]);
    const sessionDir = makeFixture({});
    try {
      const res1 = await handleApi("GET", "/api/opencode/history", new URLSearchParams("session=sess_A"), sessionDir);
      assert.equal((res1.body as any).total, 2);
      const res2 = await handleApi("GET", "/api/opencode/history", new URLSearchParams("sessionID=sess_A"), sessionDir);
      assert.equal((res2.body as any).total, 2);
      const res3 = await handleApi("GET", "/api/opencode/history", new URLSearchParams("sessionId=sess_A"), sessionDir);
      assert.equal((res3.body as any).total, 2);
    } finally { removeFixture(sessionDir); }
  } finally { restore(); cleanup(opDir); }
});

test("T2 history 分页参数校验 400：成对/范围 1-200", async () => {
  const opDir = tmpDir();
  const { restore } = withOpencodeEnv(opDir);
  const sessionDir = makeFixture({});
  try {
    const cases = [
      "page=1", // size 缺失
      "size=10", // page 缺失
      "page=0&size=10",
      "page=1&size=0",
      "page=1&size=201",
      "page=201&size=10",
      "page=abc&size=10",
    ];
    for (const qs of cases) {
      const res = await handleApi("GET", "/api/opencode/history", new URLSearchParams(qs), sessionDir);
      assert.equal(res.status, 400, `${qs} 应 400`);
      const body = res.body as { error: string; detail: string };
      assert.ok(body.error);
      assert.ok(body.detail);
    }
  } finally { removeFixture(sessionDir); restore(); cleanup(opDir); }
});

// ---------- T3 audit ----------

test("T3 audit 计算正确：本地 vs opencode 对比（含空数据与跨月过滤）", async () => {
  const opDir = tmpDir();
  // sessionDir 含 2026-08 的两条会话消息：input 100+200 =300? 按 assistantUsage 默认 input 100 each? 我们用自定义
  // 为精确控制，用 assistantUsage 覆盖 input
  const sessionDir = makeFixture({
    "2026-08-10T10-00-00-000Z_u1.jsonl": [
      sessionHeader({ id: "u1", timestamp: "2026-08-10T10:00:00.000Z", cwd: "/proj/a" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100, output: 50, cacheRead: 20, cost: { total: 0.01 } }) }, { timestamp: "2026-08-10T11:00:00.000Z" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200, output: 50, cacheRead: 30, cost: { total: 0.02 } }) }, { timestamp: "2026-08-10T12:00:00.000Z" }),
    ],
    "2026-08-20T10-00-00-000Z_u2.jsonl": [
      sessionHeader({ id: "u2", timestamp: "2026-08-20T10:00:00.000Z", cwd: "/proj/a" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 300, output: 20, cacheRead: 10, cost: { total: 0.03 } }) }, { timestamp: "2026-08-20T11:00:00.000Z" }),
    ],
    // 9 月会话不应计入 8 月审计
    "2026-09-01T10-00-00-000Z_u3.jsonl": [
      sessionHeader({ id: "u3", timestamp: "2026-09-01T10:00:00.000Z", cwd: "/proj/a" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 999, output: 999, cacheRead: 999, cost: { total: 9.99 } }) }, { timestamp: "2026-09-01T11:00:00.000Z" }),
    ],
  });
  const { restore } = withOpencodeEnv(opDir);
  try {
    const storage = new OpenCodeStorage(opDir);
    // opencode: 8 月两条，9 月一条
    await storage.mergeHistory([
      rec({ id: "usg_a1", timeCreated: "2026-08-15T12:00:00.000Z", inputTokens: 500, outputTokens: 100, cacheReadTokens: 50, cost: 0.05 }),
      rec({ id: "usg_a2", timeCreated: "2026-08-20T12:00:00.000Z", inputTokens: 100, outputTokens: 200, cacheReadTokens: 0, cost: 0.10 }),
      rec({ id: "usg_b1", timeCreated: "2026-09-05T12:00:00.000Z", inputTokens: 999, outputTokens: 999, cacheReadTokens: 999, cost: 9.99 }),
    ]);

    // 本地 8 月: requests 3, input 600 (100+200+300), output 120 (50+50+20), cacheRead 60 (20+30+10) => totalTokens = 600+60+120=780, cost 0.06
    // opencode 8 月: requests 2, input 600 (500+100), output 300 (100+200), cacheRead 50 => totalTokens 950, cost 0.15
    const res = await handleApi("GET", "/api/opencode/audit", new URLSearchParams("year=2026&month=8"), sessionDir);
    assert.equal(res.status, 200);
    const body = res.body as {
      year: number; month: number;
      localTotals: { requests: number; input: number; output: number; cacheRead: number; totalTokens: number; cost: number };
      opencodeTotals: { requests: number; input: number; output: number; cacheRead: number; totalTokens: number; cost: number };
      diff: { requests: number; tokens: number; cost: number };
      diffRate: { requests: number; tokens: number; cost: number };
      comparison: string;
    };
    assert.equal(body.year, 2026);
    assert.equal(body.month, 8);
    // 本地 totals 断言
    assert.equal(body.localTotals.requests, 3);
    assert.equal(body.localTotals.input, 600);
    assert.equal(body.localTotals.totalTokens, 780);
    assert.ok(Math.abs(body.localTotals.cost - 0.06) < 1e-9, `cost ${body.localTotals.cost} ≈ 0.06`);
    // opencode totals 断言（月内 2 条）
    assert.equal(body.opencodeTotals.requests, 2);
    assert.equal(body.opencodeTotals.totalTokens, 950);
    assert.ok(Math.abs(body.opencodeTotals.cost - 0.15) < 1e-9, `cost ${body.opencodeTotals.cost} ≈ 0.15`);
    // diff = opencode - local
    assert.equal(body.diff.requests, -1);
    assert.equal(body.diff.tokens, 170);
    // diffRate = diff / opencode
    assert.equal(body.diffRate.tokens, 170 / 950);
    assert.ok(body.comparison.length > 10);
  } finally { removeFixture(sessionDir); restore(); cleanup(opDir); }
});

test("T3 audit 空数据：本地与 opencode 均为 0 时 diff 0", async () => {
  const opDir = tmpDir();
  const sessionDir = makeFixture({
    "2026-08-10T10-00-00-000Z_u1.jsonl": [
      sessionHeader({ id: "u1", timestamp: "2026-07-01T10:00:00.000Z", cwd: "/proj/a" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    ],
  });
  const { restore } = withOpencodeEnv(opDir);
  try {
    const storage = new OpenCodeStorage(opDir);
    await storage.ensureDataDir(); // 空 history
    const res = await handleApi("GET", "/api/opencode/audit", new URLSearchParams("year=2026&month=8"), sessionDir);
    assert.equal(res.status, 200);
    const body = res.body as any;
    assert.equal(body.localTotals.requests, 0);
    assert.equal(body.opencodeTotals.requests, 0);
    assert.equal(body.diff.tokens, 0);
    assert.equal(body.diffRate.tokens, 0);
    assert.equal(body.diffRate.cost, 0);
  } finally { removeFixture(sessionDir); restore(); cleanup(opDir); }
});

test("T3 audit 参数校验 400", async () => {
  const opDir = tmpDir();
  const { restore } = withOpencodeEnv(opDir);
  const sessionDir = makeFixture({});
  try {
    for (const qs of ["", "year=2026", "month=8", "year=2026&month=13", "year=abc&month=8"]) {
      const res = await handleApi("GET", "/api/opencode/audit", new URLSearchParams(qs), sessionDir);
      assert.equal(res.status, 400, `${qs} 应 400`);
    }
  } finally { removeFixture(sessionDir); restore(); cleanup(opDir); }
});

// ---------- T4 sync ----------

test("T4 sync 成功：返回 {added, pages, elapsedMs, lastSyncedTime}", async () => {
  const opDir = tmpDir();
  const sessionDir = makeFixture({});
  const { restore } = withOpencodeEnv(opDir, { OPENCODE_AUTH: "dummy_auth", OPENCODE_WORKSPACE_ID: "wrk_test" });
  // mock client.getUsageInfo
  const origGetUsageInfo = OpenCodeClient.prototype.getUsageInfo;
  const pages: Record<number, OpenCodeUsageRecord[]> = {
    0: [rec({ id: "usg_new1", timeCreated: "2026-08-22T00:00:00.000Z" }), rec({ id: "usg_new2", timeCreated: "2026-08-21T00:00:00.000Z" })],
    1: [],
  };
  // @ts-ignore mock
  OpenCodeClient.prototype.getUsageInfo = async (_wid: string, page: number) => pages[page] ?? [];
  try {
    const res = await handleApi("POST", "/api/opencode/sync", new URLSearchParams(""), sessionDir, "");
    assert.equal(res.status, 200);
    const body = res.body as { added: number; pages: number; elapsedMs: number; lastSyncedTime: string | null };
    assert.equal(body.added, 2);
    assert.ok(body.pages >= 2);
    assert.equal(typeof body.elapsedMs, "number");
    assert.ok(body.lastSyncedTime !== null);
    // 验证持久化
    const storage = new OpenCodeStorage(opDir);
    const hist = await storage.loadHistory();
    assert.equal(hist.records.length, 2);
  } finally {
    OpenCodeClient.prototype.getUsageInfo = origGetUsageInfo;
    removeFixture(sessionDir); restore(); cleanup(opDir); __resetOpencodeSyncLockForTest();
  }
});

test("T4 sync body 凭证覆盖：body 中 auth/workspace 优先于 env", async () => {
  const opDir = tmpDir();
  const sessionDir = makeFixture({});
  // 不设 env，靠 body
  const { restore } = withOpencodeEnv(opDir);
  const orig = OpenCodeClient.prototype.getUsageInfo;
  OpenCodeClient.prototype.getUsageInfo = async (_wid: string, page: number) => page === 0 ? [rec({ id: "usg_body", timeCreated: "2026-08-22T00:00:00.000Z" })] : [];
  try {
    const body = JSON.stringify({ auth: "body_auth", workspaceId: "wrk_body" });
    const res = await handleApi("POST", "/api/opencode/sync", new URLSearchParams(""), sessionDir, body);
    assert.equal(res.status, 200);
    assert.equal((res.body as any).added, 1);
  } finally {
    OpenCodeClient.prototype.getUsageInfo = orig;
    removeFixture(sessionDir); restore(); cleanup(opDir); __resetOpencodeSyncLockForTest();
  }
});

test("T4 sync 认证缺失返回 500 友好 {error, detail} 含认证失效", async () => {
  const opDir = tmpDir();
  const sessionDir = makeFixture({});
  const { restore } = withOpencodeEnv(opDir);
  // 确保无 env auth
  delete process.env.OPENCODE_AUTH;
  delete process.env.OPENCODE_WORKSPACE_ID;
  // 确保 .env 不存在影响：tmp 操作不影响 cwd .env，若 cwd 含 .env 需清理? 假设未含 OPENCODE_AUTH
  try {
    const res = await handleApi("POST", "/api/opencode/sync", new URLSearchParams(""), sessionDir, "");
    assert.equal(res.status, 500);
    const body = res.body as { error: string; detail: string };
    assert.ok(body.error.length > 0);
    assert.match(body.detail, /认证失效|凭证/);
  } finally { removeFixture(sessionDir); restore(); cleanup(opDir); __resetOpencodeSyncLockForTest(); }
});

test("T4 sync 网络/认证异常返回 500 友好提示", async () => {
  const opDir = tmpDir();
  const sessionDir = makeFixture({});
  const { restore } = withOpencodeEnv(opDir, { OPENCODE_AUTH: "bad", OPENCODE_WORKSPACE_ID: "wrk_test" });
  const orig = OpenCodeClient.prototype.getUsageInfo;
  OpenCodeClient.prototype.getUsageInfo = async () => { throw new Error("认证失效: OpenCode 凭证已过期或无效（HTTP 401）"); };
  try {
    const res = await handleApi("POST", "/api/opencode/sync", new URLSearchParams(""), sessionDir, "");
    assert.equal(res.status, 500);
    const body = res.body as { detail: string };
    assert.match(body.detail, /认证失效/);
  } finally {
    OpenCodeClient.prototype.getUsageInfo = orig;
    removeFixture(sessionDir); restore(); cleanup(opDir); __resetOpencodeSyncLockForTest();
  }
});

test("T4 sync 自动发现工作区：未提供 workspaceId 时调用 getWorkspaces", async () => {
  const opDir = tmpDir();
  const sessionDir = makeFixture({});
  const { restore } = withOpencodeEnv(opDir, { OPENCODE_AUTH: "dummy" });
  const origWs = OpenCodeClient.prototype.getWorkspaces;
  const origUsage = OpenCodeClient.prototype.getUsageInfo;
  OpenCodeClient.prototype.getWorkspaces = async () => [{ id: "wrk_auto", name: "auto" }];
  OpenCodeClient.prototype.getUsageInfo = async (_wid: string, page: number) => page === 0 ? [rec({ id: "usg_auto", timeCreated: "2026-08-22T00:00:00.000Z" })] : [];
  try {
    const res = await handleApi("POST", "/api/opencode/sync", new URLSearchParams(""), sessionDir, "");
    assert.equal(res.status, 200);
    assert.equal((res.body as any).added, 1);
  } finally {
    OpenCodeClient.prototype.getWorkspaces = origWs;
    OpenCodeClient.prototype.getUsageInfo = origUsage;
    removeFixture(sessionDir); restore(); cleanup(opDir); __resetOpencodeSyncLockForTest();
  }
});

test("T4 sync 单锁或串行：并发第二次返回 409", async () => {
  const opDir = tmpDir();
  const sessionDir = makeFixture({});
  const { restore } = withOpencodeEnv(opDir, { OPENCODE_AUTH: "dummy", OPENCODE_WORKSPACE_ID: "wrk_test" });
  const orig = OpenCodeClient.prototype.getUsageInfo;
  // 让第一次 sync 延迟 150ms
  OpenCodeClient.prototype.getUsageInfo = async (_wid: string, page: number) => {
    if (page === 0) {
      await new Promise((r) => setTimeout(r, 150));
      return [rec({ id: "usg_delay", timeCreated: "2026-08-22T00:00:00.000Z" })];
    }
    return [];
  };
  try {
    const p1 = handleApi("POST", "/api/opencode/sync", new URLSearchParams(""), sessionDir, "");
    // 稍微错开，确保 p1 已拿到锁
    await new Promise((r) => setTimeout(r, 10));
    const p2 = await handleApi("POST", "/api/opencode/sync", new URLSearchParams(""), sessionDir, "");
    assert.equal(p2.status, 409);
    assert.match((p2.body as any).detail, /同步进行中/);
    const r1 = await p1;
    assert.equal(r1.status, 200);
  } finally {
    OpenCodeClient.prototype.getUsageInfo = orig;
    removeFixture(sessionDir); restore(); cleanup(opDir); __resetOpencodeSyncLockForTest();
  }
});

// ---------- T5 统一错误格式 ----------
test("T5 统一错误体 {error, detail}：未知 openCode 路径 404，方法不匹配 404", async () => {
  const opDir = tmpDir();
  const { restore } = withOpencodeEnv(opDir);
  const sessionDir = makeFixture({});
  try {
    const res404 = await handleApi("GET", "/api/opencode/unknown", new URLSearchParams(""), sessionDir);
    assert.equal(res404.status, 404);
    assert.equal(typeof (res404.body as any).error, "string");
    assert.equal(typeof (res404.body as any).detail, "string");

    const resMethod = await handleApi("GET", "/api/opencode/sync", new URLSearchParams(""), sessionDir);
    assert.equal(resMethod.status, 404);
    assert.ok((resMethod.body as any).detail.length > 0);
  } finally { removeFixture(sessionDir); restore(); cleanup(opDir); }
});
