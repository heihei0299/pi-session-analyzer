import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync, readFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { OpenCodeStorage } from "../src/opencode/storage.ts";
import type { OpenCodeCostsResult, OpenCodeUsageRecord } from "../src/opencode/types.ts";

// ---------- helpers ----------
function tmpDir(): string {
  return mkdtempSync(join(tmpdir(), "opencode-storage-test-"));
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

// ---------- T1 ensureDataDir + 初始文件结构 ----------
test("T1 ensureDataDir 创建 data/opencode/costs.json 与 history.json 空结构", async () => {
  const dir = tmpDir();
  try {
    const storage = new OpenCodeStorage(dir);
    await storage.ensureDataDir();
    assert.ok(existsSync(join(dir, "costs.json")), "costs.json 应存在");
    assert.ok(existsSync(join(dir, "history.json")), "history.json 应存在");
    const costs = JSON.parse(readFileSync(join(dir, "costs.json"), "utf-8"));
    assert.deepEqual(costs, {}, "初始 costs.json 应为空对象");
    const hist = JSON.parse(readFileSync(join(dir, "history.json"), "utf-8"));
    assert.ok(Array.isArray(hist.records), "history.records 应为数组");
    assert.equal(hist.records.length, 0);
    assert.equal(hist.lastSyncedTime, null);
    assert.ok(typeof hist.updatedAt === "string");
  } finally { cleanup(dir); }
});

test("T1 ensureDataDir 幂等：已存在数据不被覆盖", async () => {
  const dir = tmpDir();
  try {
    const storage = new OpenCodeStorage(dir);
    await storage.ensureDataDir();
    const r = rec({ id: "usg_keep", timeCreated: "2026-08-20T01:00:00.000Z" });
    await storage.mergeHistory([r]);
    // 再次 ensure
    await storage.ensureDataDir();
    const loaded = await storage.loadHistory();
    assert.equal(loaded.records.length, 1);
    assert.equal(loaded.records[0].id, "usg_keep");
  } finally { cleanup(dir); }
});

// ---------- T2 costs 按 year-month 存储与查询 ----------
test("T2 saveCosts / getCosts 往返", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const result = costsResult();
    await s.saveCosts(2026, 8, result);
    const got = await s.getCosts(2026, 8);
    assert.deepEqual(got, result);
  } finally { cleanup(dir); }
});

test("T2 getCosts 未存储返回 null", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const got = await s.getCosts(2026, 9);
    assert.equal(got, null);
  } finally { cleanup(dir); }
});

test("T2 saveCosts 同月覆盖", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    await s.saveCosts(2026, 8, costsResult({ usage: [{ date: "2026-08-01", model: "m1", totalCost: 100, keyId: "k1", plan: "lite" }] }));
    await s.saveCosts(2026, 8, costsResult({ usage: [{ date: "2026-08-01", model: "m2", totalCost: 200, keyId: "k1", plan: "lite" }] }));
    const got = await s.getCosts(2026, 8);
    assert.equal(got!.usage[0].model, "m2");
    assert.equal(got!.usage.length, 1);
  } finally { cleanup(dir); }
});

test("T2 listCosts 返回所有月份排序", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    await s.saveCosts(2026, 8, costsResult({ usage: [{ date: "2026-08-01", model: "m8", totalCost: 8, keyId: "k1", plan: "lite" }] }));
    await s.saveCosts(2026, 7, costsResult({ usage: [{ date: "2026-07-01", model: "m7", totalCost: 7, keyId: "k1", plan: "lite" }] }));
    await s.saveCosts(2026, 9, costsResult({ usage: [{ date: "2026-09-01", model: "m9", totalCost: 9, keyId: "k1", plan: "lite" }] }));
    const list = await s.listCosts();
    assert.equal(list.length, 3);
    assert.equal(list[0].month, 7);
    assert.equal(list[1].month, 8);
    assert.equal(list[2].month, 9);
    assert.equal(list[0].year, 2026);
  } finally { cleanup(dir); }
});

test("T2 costs 存储区分年", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    await s.saveCosts(2025, 12, costsResult({ usage: [{ date: "2025-12-01", model: "old", totalCost: 1, keyId: "k1", plan: "lite" }] }));
    await s.saveCosts(2026, 1, costsResult({ usage: [{ date: "2026-01-01", model: "new", totalCost: 2, keyId: "k1", plan: "lite" }] }));
    assert.equal((await s.getCosts(2025, 12))!.usage[0].model, "old");
    assert.equal((await s.getCosts(2026, 1))!.usage[0].model, "new");
  } finally { cleanup(dir); }
});

// ---------- T3 history 去重合并与排序 ----------
test("T3 mergeHistory 按 id 去重保留最新", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const r1 = rec({ id: "usg_1", timeCreated: "2026-08-20T10:00:00.000Z", model: "m1", cost: 0.001 });
    const r1b = rec({ id: "usg_1", timeCreated: "2026-08-20T10:00:00.000Z", model: "m1-updated", cost: 0.999 });
    await s.mergeHistory([r1]);
    const res = await s.mergeHistory([r1b]);
    assert.equal(res.added, 0, "重复 id 不应计为 added");
    const loaded = await s.loadHistory();
    assert.equal(loaded.records.length, 1);
    assert.equal(loaded.records[0].model, "m1-updated");
    assert.equal(loaded.records[0].cost, 0.999);
  } finally { cleanup(dir); }
});

test("T3 mergeHistory 按 timeCreated 逆序排序", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const r1 = rec({ id: "usg_a", timeCreated: "2026-08-18T00:00:00.000Z" });
    const r2 = rec({ id: "usg_b", timeCreated: "2026-08-20T00:00:00.000Z" });
    const r3 = rec({ id: "usg_c", timeCreated: "2026-08-19T00:00:00.000Z" });
    // 乱序入
    await s.mergeHistory([r1, r2, r3]);
    const loaded = await s.loadHistory();
    assert.equal(loaded.records[0].id, "usg_b");
    assert.equal(loaded.records[1].id, "usg_c");
    assert.equal(loaded.records[2].id, "usg_a");
  } finally { cleanup(dir); }
});

test("T3 mergeHistory 批量去重与排序综合", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const batch1 = [
      rec({ id: "usg_1", timeCreated: "2026-08-20T02:00:00.000Z" }),
      rec({ id: "usg_2", timeCreated: "2026-08-20T01:00:00.000Z" }),
    ];
    const batch2 = [
      rec({ id: "usg_2", timeCreated: "2026-08-20T01:00:00.000Z", model: "new-model" }), // duplicate
      rec({ id: "usg_3", timeCreated: "2026-08-20T03:00:00.000Z" }),
    ];
    await s.mergeHistory(batch1);
    const res = await s.mergeHistory(batch2);
    assert.equal(res.added, 1);
    assert.equal(res.total, 3);
    const loaded = await s.loadHistory();
    assert.equal(loaded.records.length, 3);
    // 逆序
    assert.equal(loaded.records[0].id, "usg_3");
    // 去重保留最新 model
    const r2 = loaded.records.find(r => r.id === "usg_2")!;
    assert.equal(r2.model, "new-model");
  } finally { cleanup(dir); }
});

test("T3 mergeHistory 更新 lastSyncedTime 为最新记录 timeCreated", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const r1 = rec({ id: "usg_1", timeCreated: "2026-08-19T00:00:00.000Z" });
    const r2 = rec({ id: "usg_2", timeCreated: "2026-08-21T00:00:00.000Z" });
    await s.mergeHistory([r1, r2]);
    const loaded = await s.loadHistory();
    assert.equal(loaded.lastSyncedTime, "2026-08-21T00:00:00.000Z");
  } finally { cleanup(dir); }
});

test("T3 mergeHistory 空输入不破坏现有数据", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const r = rec({ id: "usg_1", timeCreated: "2026-08-20T00:00:00.000Z" });
    await s.mergeHistory([r]);
    await s.mergeHistory([]);
    const loaded = await s.loadHistory();
    assert.equal(loaded.records.length, 1);
  } finally { cleanup(dir); }
});

// ---------- T4 exportCsv ----------
test("T4 exportCsv 生成标准表头与行", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const r1 = rec({ id: "usg_1", timeCreated: "2026-08-20T10:00:00.000Z", timeUpdated: "2026-08-20T10:00:00.000Z", model: "m1", provider: "inf.oa-compat", inputTokens: 1200, outputTokens: 800, reasoningTokens: 200, cacheReadTokens: 100, cacheWrite5mTokens: 10, cacheWrite1hTokens: null, cost: 0.0023, keyID: "key_1", sessionID: "sess_001", enrichment: null });
    const r2 = rec({ id: "usg_2", timeCreated: "2026-08-19T10:00:00.000Z", timeUpdated: "2026-08-19T10:00:00.000Z", model: "m2", provider: "inf.oa-compat", inputTokens: 500, outputTokens: 300, reasoningTokens: null, cacheReadTokens: null, cacheWrite5mTokens: null, cacheWrite1hTokens: null, cost: 0.001, keyID: "key_2", sessionID: null, enrichment: null });
    await s.mergeHistory([r1, r2]);
    const csv = await s.exportCsv();
    const lines = csv.trim().split("\n");
    // 表头
    const header = lines[0];
    assert.equal(header, "id,workspaceID,timeCreated,timeUpdated,timeDeleted,model,provider,inputTokens,outputTokens,reasoningTokens,cacheReadTokens,cacheWrite5mTokens,cacheWrite1hTokens,cost,keyID,sessionID,enrichment");
    assert.equal(lines.length, 3, "应有表头+2 行");
    // 检查首行是逆序最新
    assert.ok(lines[1].startsWith("usg_1,"));
    assert.ok(lines[2].startsWith("usg_2,"));
  } finally { cleanup(dir); }
});

test("T4 exportCsv 正确转义逗号/引号/换行", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const r = rec({
      id: "usg_escape",
      timeCreated: "2026-08-20T10:00:00.000Z",
      model: 'model,with,comma',
      provider: 'prov"quote',
      enrichment: { note: 'a,b"c\nd' },
    } as unknown as Partial<OpenCodeUsageRecord> & { id: string; timeCreated: string });
    // 额外通过原始覆盖制造逗号/引号/换行
    // 用 enrichment 验证 JSON 转义，原样通过 csvEscape 会包含逗号与引号需包引号
    await s.mergeHistory([r]);
    const csv = await s.exportCsv();
    // 含逗号的 model 应被引号包裹
    assert.ok(csv.includes('"model,with,comma"'), "逗号应被转义");
    // 含引号的 provider -> 引号双写并包引号
    assert.ok(csv.includes('"prov""quote"'), "引号应双写转义");
    // enrichment JSON 含逗号/引号/换行应被包引号且内部引号双写
    assert.ok(csv.includes('""'), "enrichment 中引号应双写");
  } finally { cleanup(dir); }
});

test("T4 exportCsv 空历史仅表头", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const csv = await s.exportCsv();
    const lines = csv.trim().split("\n");
    assert.equal(lines.length, 1);
    assert.ok(lines[0].startsWith("id,"));
  } finally { cleanup(dir); }
});

test("T4 exportCsv(filePath) 写入指定路径并返回内容一致", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const r = rec({ id: "usg_1", timeCreated: "2026-08-20T00:00:00.000Z" });
    await s.mergeHistory([r]);
    const custom = join(dir, "custom.csv");
    const csv = await s.exportCsv(custom);
    assert.ok(existsSync(custom));
    const onDisk = readFileSync(custom, "utf-8");
    assert.equal(csv, onDisk);
  } finally { cleanup(dir); }
});

test("T4 exportCsv 默认写入 data/opencode/history.csv", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const r = rec({ id: "usg_1", timeCreated: "2026-08-20T00:00:00.000Z" });
    await s.mergeHistory([r]);
    await s.exportCsv();
    assert.ok(existsSync(join(dir, "history.csv")));
  } finally { cleanup(dir); }
});

// ---------- T5 sync ----------
function makeMockClient(pages: Record<number, OpenCodeUsageRecord[]>) {
  return {
    async getUsageInfo(_workspaceId: string, page: number): Promise<OpenCodeUsageRecord[]> {
      return pages[page] ?? [];
    },
  };
}

test("T5 sync 从 page 0 起遍历，直到空页，返回 added/pages/lastSyncedTime", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const p0 = [rec({ id: "usg_3", timeCreated: "2026-08-22T00:00:00.000Z" }), rec({ id: "usg_2", timeCreated: "2026-08-21T00:00:00.000Z" })];
    const p1 = [rec({ id: "usg_1", timeCreated: "2026-08-20T00:00:00.000Z" })];
    const p2: OpenCodeUsageRecord[] = [];
    const client = makeMockClient({ 0: p0, 1: p1, 2: p2 });
    const res = await s.sync(client, { workspaceId: "wrk_test" });
    assert.equal(res.added, 3);
    assert.equal(res.pages, 3, "应抓到第 2 页空页为止");
    assert.equal(res.lastSyncedTime, "2026-08-22T00:00:00.000Z");
    assert.ok(typeof res.elapsedMs === "number");
    const loaded = await s.loadHistory();
    assert.equal(loaded.records.length, 3);
  } finally { cleanup(dir); }
});

test("T5 sync 增量截断：遇到已存在 id 即停止", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    // 预置一条旧记录
    const old = rec({ id: "usg_old", timeCreated: "2026-08-20T00:00:00.000Z" });
    await s.mergeHistory([old]);
    // mock 返回 page0 含新 + 旧，page1 不应被请求
    let page1Called = false;
    const client = {
      async getUsageInfo(_wid: string, page: number) {
        if (page === 0) return [rec({ id: "usg_new", timeCreated: "2026-08-21T00:00:00.000Z" }), old];
        if (page === 1) { page1Called = true; return [rec({ id: "usg_older", timeCreated: "2026-08-19T00:00:00.000Z" })]; }
        return [];
      },
    };
    const res = await s.sync(client, { workspaceId: "wrk_test" });
    assert.equal(res.added, 1);
    assert.equal(page1Called, false, "截断后不应请求下一页");
    const loaded = await s.loadHistory();
    assert.equal(loaded.records.length, 2);
  } finally { cleanup(dir); }
});

test("T5 sync 增量截断：timeCreated <= lastSyncedTime 即停止（含混合页截断）", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    await s.mergeHistory([rec({ id: "usg_base", timeCreated: "2026-08-20T00:00:00.000Z" })]);
    // lastSyncedTime = 2026-08-20
    const client = makeMockClient({
      0: [rec({ id: "usg_new1", timeCreated: "2026-08-22T00:00:00.000Z" }), rec({ id: "usg_new2", timeCreated: "2026-08-21T00:00:00.000Z" }), rec({ id: "usg_at_cursor", timeCreated: "2026-08-20T00:00:00.000Z" }), rec({ id: "usg_older", timeCreated: "2026-08-19T00:00:00.000Z" })],
      1: [rec({ id: "usg_should_not_fetch", timeCreated: "2026-08-18T00:00:00.000Z" })],
    });
    let calledPage1 = false;
    const tracked = {
      async getUsageInfo(wid: string, page: number) {
        if (page === 1) calledPage1 = true;
        return client.getUsageInfo(wid, page);
      },
    };
    const res = await s.sync(tracked, { workspaceId: "wrk_test" });
    assert.equal(res.added, 2, "只应新增截断点之前的 2 条");
    assert.equal(calledPage1, false);
    const hist = await s.loadHistory();
    // 应包含 new1 new2 base 共 3
    assert.equal(hist.records.length, 3);
    assert.ok(!hist.records.some(r => r.id === "usg_at_cursor" || r.id === "usg_older"));
  } finally { cleanup(dir); }
});

test("T5 sync full=true 忽略 lastSyncedTime 拉到底", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    await s.mergeHistory([rec({ id: "usg_old", timeCreated: "2026-08-20T00:00:00.000Z" })]);
    const client = makeMockClient({
      0: [rec({ id: "usg_new", timeCreated: "2026-08-21T00:00:00.000Z" }), rec({ id: "usg_old", timeCreated: "2026-08-20T00:00:00.000Z" })],
      1: [rec({ id: "usg_older", timeCreated: "2026-08-19T00:00:00.000Z" })],
      2: [],
    });
    const res = await s.sync(client, { workspaceId: "wrk_test", full: true });
    // full 忽略截断，应拉到空页，新增仅非重复
    assert.equal(res.pages, 3);
    assert.equal(res.added, 2, "usg_new + usg_older 新增，usg_old 重复去重");
    const hist = await s.loadHistory();
    assert.equal(hist.records.length, 3);
  } finally { cleanup(dir); }
});

test("T5 sync limit 限制最大页数", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const client = makeMockClient({
      0: [rec({ id: "usg_3", timeCreated: "2026-08-22T00:00:00.000Z" })],
      1: [rec({ id: "usg_2", timeCreated: "2026-08-21T00:00:00.000Z" })],
      2: [rec({ id: "usg_1", timeCreated: "2026-08-20T00:00:00.000Z" })],
      3: [],
    });
    const res = await s.sync(client, { workspaceId: "wrk_test", limit: 2 });
    assert.equal(res.pages, 2);
    assert.equal(res.added, 2);
    const hist = await s.loadHistory();
    assert.equal(hist.records.length, 2);
  } finally { cleanup(dir); }
});

test("T5 sync 支持 sync(workspaceId, client, opts) 重载", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const client = makeMockClient({ 0: [rec({ id: "usg_1", timeCreated: "2026-08-20T00:00:00.000Z" })], 1: [] });
    const res = await s.sync("wrk_test", client, {});
    assert.equal(res.added, 1);
  } finally { cleanup(dir); }
});

test("T5 sync 支持 sync(client, workspaceIdString) 重载", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const client = makeMockClient({ 0: [rec({ id: "usg_1", timeCreated: "2026-08-20T00:00:00.000Z" })], 1: [] });
    const res = await (s as any).sync(client, "wrk_test");
    assert.equal(res.added, 1);
  } finally { cleanup(dir); }
});

test("T5 sync 无 workspaceId 抛错", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const client = makeMockClient({ 0: [] });
    await assert.rejects(() => (s as any).sync(client, {}), /workspaceId/);
  } finally { cleanup(dir); }
});

test("T5 sync 空页立即结束且不写重复", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const client = makeMockClient({ 0: [] });
    const res = await s.sync(client, { workspaceId: "wrk_test" });
    assert.equal(res.added, 0);
    assert.equal(res.pages, 1);
  } finally { cleanup(dir); }
});

test("T5 sync 更新 lastSyncedTime 与 elapsedMs", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const client = makeMockClient({ 0: [rec({ id: "usg_1", timeCreated: "2026-08-25T12:00:00.000Z" })], 1: [] });
    const res = await s.sync(client, { workspaceId: "wrk_test" });
    assert.equal(res.lastSyncedTime, "2026-08-25T12:00:00.000Z");
    assert.ok(res.elapsedMs >= 0);
    assert.ok(res.elapsedMs < 5000);
  } finally { cleanup(dir); }
});

// ---------- T6 持久化往返与查询过滤 ----------
test("T6 loadHistory 空与非空往返", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    const empty = await s.loadHistory();
    assert.equal(empty.records.length, 0);
    assert.equal(empty.lastSyncedTime, null);
    const r = rec({ id: "usg_1", timeCreated: "2026-08-20T00:00:00.000Z" });
    await s.mergeHistory([r]);
    const after = await s.loadHistory();
    assert.equal(after.records.length, 1);
    assert.equal(after.records[0].id, "usg_1");
  } finally { cleanup(dir); }
});

test("T6 getHistory 按 model 过滤", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    await s.mergeHistory([
      rec({ id: "usg_1", timeCreated: "2026-08-20T00:00:00.000Z", model: "m1" }),
      rec({ id: "usg_2", timeCreated: "2026-08-21T00:00:00.000Z", model: "m2" }),
      rec({ id: "usg_3", timeCreated: "2026-08-22T00:00:00.000Z", model: "m1" }),
    ]);
    const filtered = await s.getHistory({ model: "m1" });
    assert.equal(filtered.length, 2);
    assert.ok(filtered.every(r => r.model === "m1"));
  } finally { cleanup(dir); }
});

test("T6 getHistory 按 sessionID 过滤（兼容 sessionId 别名）", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    await s.mergeHistory([
      rec({ id: "usg_1", timeCreated: "2026-08-20T00:00:00.000Z", sessionID: "sess_A" }),
      rec({ id: "usg_2", timeCreated: "2026-08-21T00:00:00.000Z", sessionID: "sess_B" }),
      rec({ id: "usg_3", timeCreated: "2026-08-22T00:00:00.000Z", sessionID: "sess_A" }),
    ]);
    const byAlias = await s.getHistory({ sessionId: "sess_A" } as any);
    assert.equal(byAlias.length, 2);
    const byCanonical = await s.getHistory({ sessionID: "sess_A" } as any);
    assert.equal(byCanonical.length, 2);
  } finally { cleanup(dir); }
});

test("T6 getHistory 按 timeRange (since/until) 过滤", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    await s.mergeHistory([
      rec({ id: "usg_1", timeCreated: "2026-08-19T00:00:00.000Z" }),
      rec({ id: "usg_2", timeCreated: "2026-08-20T00:00:00.000Z" }),
      rec({ id: "usg_3", timeCreated: "2026-08-21T00:00:00.000Z" }),
    ]);
    const sinceOnly = await s.getHistory({ since: "2026-08-20T00:00:00.000Z" });
    assert.equal(sinceOnly.length, 2);
    const untilOnly = await s.getHistory({ until: "2026-08-20T00:00:00.000Z" });
    assert.equal(untilOnly.length, 2);
    const both = await s.getHistory({ since: "2026-08-20T00:00:00.000Z", until: "2026-08-20T23:59:59.999Z" });
    assert.equal(both.length, 1);
    assert.equal(both[0].id, "usg_2");
  } finally { cleanup(dir); }
});

test("T6 getHistory 组合过滤 model+timeRange", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    await s.mergeHistory([
      rec({ id: "usg_1", timeCreated: "2026-08-20T10:00:00.000Z", model: "m1" }),
      rec({ id: "usg_2", timeCreated: "2026-08-20T11:00:00.000Z", model: "m2" }),
      rec({ id: "usg_3", timeCreated: "2026-08-21T10:00:00.000Z", model: "m1" }),
    ]);
    const filtered = await s.getHistory({ model: "m1", since: "2026-08-20T00:00:00.000Z", until: "2026-08-20T23:59:59.999Z" });
    assert.equal(filtered.length, 1);
    assert.equal(filtered[0].id, "usg_1");
  } finally { cleanup(dir); }
});

test("T6 clear 重置 history/costs/csv", async () => {
  const dir = tmpDir();
  try {
    const s = new OpenCodeStorage(dir);
    await s.saveCosts(2026, 8, costsResult());
    await s.mergeHistory([rec({ id: "usg_1", timeCreated: "2026-08-20T00:00:00.000Z" })]);
    await s.clear();
    assert.equal((await s.getCosts(2026, 8)), null);
    const hist = await s.loadHistory();
    assert.equal(hist.records.length, 0);
    assert.equal(hist.lastSyncedTime, null);
    const csv = readFileSync(join(dir, "history.csv"), "utf-8");
    assert.equal(csv.trim().split("\n").length, 1, "clear 后 csv 仅表头");
  } finally { cleanup(dir); }
});

test("T6 持久化往返：跨实例读取", async () => {
  const dir = tmpDir();
  try {
    const s1 = new OpenCodeStorage(dir);
    const r = rec({ id: "usg_cross", timeCreated: "2026-08-20T00:00:00.000Z" });
    await s1.saveCosts(2026, 8, costsResult());
    await s1.mergeHistory([r]);
    const s2 = new OpenCodeStorage(dir);
    assert.equal((await s2.getCosts(2026, 8))!.usage[0].totalCost, 891912946);
    assert.equal((await s2.loadHistory()).records[0].id, "usg_cross");
  } finally { cleanup(dir); }
});
