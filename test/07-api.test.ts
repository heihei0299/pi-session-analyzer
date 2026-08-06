/**
 * Ticket 02 — HTTP API 端点集。
 * 六个只读 JSON 端点与 CLI 结构化输出一致；筛选参数映射 CLI 语义；统一 JSON 错误体。
 * Seam：fixture 目录启动 serve（随机端口）→ fetch 端点断言 JSON 响应与状态码。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import { startWebServer } from "../src/server.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

/** 多会话多模型多项目 fixture：两个会话、两个模型、两个 cwd、两个时间戳 */
function buildDir(): string {
  return makeFixture({
    "2026-08-01T10-00-00-000Z_u1.jsonl": [
      sessionHeader({ id: "u1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/home/shial/Project/alpha" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
      messageEntry({ role: "assistant", model: "m2", usage: assistantUsage({ input: 200 }) }),
    ],
    "2026-08-15T12-00-00-000Z_u2.jsonl": [
      sessionHeader({ id: "u2", timestamp: "2026-08-15T12:00:00.000Z", cwd: "/home/shial/Project/beta" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 300 }) }),
    ],
  });
}

test("S6 /api/totals 与 CLI --format json totals 输出完全一致", async () => {
  const dir = buildDir();
  const cliOut = await runCli(["--dir", dir, "--format", "json"]);
  const cliTotals = JSON.parse(cliOut) as Record<string, unknown>;
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const res = await fetch(new URL("/api/totals", server.url));
    assert.equal(res.status, 200);
    assert.match(res.headers.get("content-type") ?? "", /^application\/json/);
    const api = (await res.json()) as Record<string, unknown>;
    assert.deepEqual(api, cliTotals, "API totals 应与 CLI totals json 逐字段一致");
    assert.equal(api.window, "totals");
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

test("S7 /api/sessions 与 /api/requests 行结构与 CLI json 对应 rows 一致", async () => {
  const dir = buildDir();
  const cliSessions = JSON.parse(await runCli(["--dir", dir, "sessions", "--format", "json"])) as {
    window: string;
    rows: Record<string, unknown>[];
  };
  const cliRequests = JSON.parse(await runCli(["--dir", dir, "requests", "--format", "json"])) as {
    window: string;
    rows: Record<string, unknown>[];
  };
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const resS = await fetch(new URL("/api/sessions", server.url));
    assert.equal(resS.status, 200);
    const apiS = (await resS.json()) as { window: string; rows: Record<string, unknown>[] };
    assert.equal(apiS.window, "sessions");
    // 与 CLI rows 逐字段一致（API 行可含 fileName/displayName/cwdNorm 扩展字段，见 ticket 06）
    assert.equal(apiS.rows.length, cliSessions.rows.length);
    for (let i = 0; i < apiS.rows.length; i++) {
      for (const k of Object.keys(cliSessions.rows[i])) {
        assert.deepEqual(apiS.rows[i][k], cliSessions.rows[i][k], `sessions row[${i}] 字段 ${k} 应与 CLI 一致`);
      }
    }
    // 每行字段：sessionId/timestamp/cwd/model + 9 指标
    for (const row of apiS.rows) {
      for (const key of ["sessionId", "timestamp", "cwd", "model", "requests", "input", "output", "cacheRead", "cacheWrite", "reasoning", "totalTokens", "cost", "cacheRate"]) {
        assert.ok(key in row, `sessions 行应含字段 ${key}`);
      }
    }

    const resR = await fetch(new URL("/api/requests", server.url));
    assert.equal(resR.status, 200);
    const apiR = (await resR.json()) as { window: string; rows: Record<string, unknown>[] };
    assert.equal(apiR.window, "requests");
    // API 行 = CLI 全部字段 + displayName 扩展（会话名称列数据源，见会话名称需求）
    assert.equal(apiR.rows.length, cliRequests.rows.length);
    for (let i = 0; i < apiR.rows.length; i++) {
      for (const k of Object.keys(cliRequests.rows[i])) {
        assert.deepEqual(apiR.rows[i][k], cliRequests.rows[i][k], `requests row[${i}] 字段 ${k} 应与 CLI 一致`);
      }
    }
    for (const row of apiR.rows) {
      for (const key of ["sessionId", "timestamp", "model", "displayName", "requests", "input", "output", "cacheRead", "cacheWrite", "reasoning", "totalTokens", "cost", "cacheRate"]) {
        assert.ok(key in row, `requests 行应含字段 ${key}`);
      }
    }
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

test("S8 /api/groups?by=model|cwd|model,cwd 与 CLI --by json rows 一致", async () => {
  const dir = buildDir();
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    for (const by of ["model", "cwd", "model,cwd"] as const) {
      const cliOut = JSON.parse(await runCli(["--dir", dir, "--by", by, "--format", "json"])) as {
        window: string;
        by: string;
        rows: Record<string, unknown>[];
      };
      const res = await fetch(new URL(`/api/groups?by=${encodeURIComponent(by)}`, server.url));
      assert.equal(res.status, 200);
      const api = (await res.json()) as { window: string; by: string; rows: Record<string, unknown>[] };
      assert.equal(api.window, "totals");
      assert.equal(api.by, by);
      assert.deepEqual(api.rows, cliOut.rows, `by=${by} 分组行应与 CLI 一致`);
    }
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

test("S9 /api/period?period=day|week|month 与 CLI --period json rows 一致", async () => {
  const dir = buildDir();
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    for (const period of ["day", "week", "month"] as const) {
      const cliOut = JSON.parse(await runCli(["--dir", dir, "--period", period, "--format", "json"])) as {
        window: string;
        period: string;
        rows: Record<string, unknown>[];
      };
      const res = await fetch(new URL(`/api/period?period=${period}`, server.url));
      assert.equal(res.status, 200);
      const api = (await res.json()) as { window: string; period: string; rows: Record<string, unknown>[] };
      assert.equal(api.window, "totals");
      assert.equal(api.period, period);
      assert.deepEqual(api.rows, cliOut.rows, `period=${period} 周期行应与 CLI 一致`);
    }
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

test("S10 /api/meta 返回 dir/sessionCount/dataRange（可解析时间戳 min/max）", async () => {
  const dir = buildDir();
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const res = await fetch(new URL("/api/meta", server.url));
    assert.equal(res.status, 200);
    const meta = (await res.json()) as {
      dir: string;
      sessionCount: number;
      dataRange: { since: string; until: string };
    };
    assert.equal(meta.dir, dir);
    assert.equal(meta.sessionCount, 2);
    assert.equal(meta.dataRange.since, "2026-08-01T10:00:00.000Z");
    assert.equal(meta.dataRange.until, "2026-08-15T12:00:00.000Z");
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

test("S11 model/cwd/since/until 筛选与 CLI 一致；非法 since / 未知 by / 未知 period → 400", async () => {
  const dir = buildDir();
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    // model 筛选
    const cliModel = JSON.parse(await runCli(["--dir", dir, "--model", "m1", "--format", "json"])) as Record<string, unknown>;
    const apiModel = (await (await fetch(new URL("/api/totals?model=m1", server.url))).json()) as Record<string, unknown>;
    assert.deepEqual(apiModel, cliModel);

    // cwd 筛选
    const cliCwd = JSON.parse(await runCli(["--dir", dir, "--cwd", "/home/shial/Project/alpha", "--format", "json"])) as Record<string, unknown>;
    const apiCwd = (await (await fetch(new URL("/api/totals?cwd=/home/shial/Project/alpha", server.url))).json()) as Record<string, unknown>;
    assert.deepEqual(apiCwd, cliCwd);

    // since/until 筛选（ticket 23：webui totals 消息级，与 CLI 会话级不同——spec 记录差异；fixture 消息 timestamp 均为 07-31）
    const apiSince = (await (await fetch(new URL("/api/totals?since=2026-08-10", server.url))).json()) as { requests: number };
    assert.equal(apiSince.requests, 0, "webui since 消息级：07-31 消息全部 < 8/10 → 排除");
    const apiUntil = (await (await fetch(new URL("/api/totals?until=2026-08-10", server.url))).json()) as { requests: number };
    assert.equal(apiUntil.requests, 3, "webui until 消息级：3 条 07-31 消息均 ≤ 8/10 → 全保留（CLI 会话级仅 header 08-01 的 2 条）");
    // 非法参数 → 400 统一错误体
    for (const url of [
      "/api/totals?since=abc",
      "/api/totals?until=abc",
      "/api/groups?by=xyz",
      "/api/groups",
      "/api/period?period=xyz",
      "/api/period",
    ]) {
      const res = await fetch(new URL(url, server.url));
      assert.equal(res.status, 400, `${url} 应返回 400`);
      const body = (await res.json()) as { error: string; detail: string };
      assert.equal(typeof body.error, "string");
      assert.ok(body.error.length > 0);
      assert.equal(typeof body.detail, "string");
      assert.ok(body.detail.length > 0);
    }
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

test("S12 空目录 500 + detail；未知 API 路径 404；非 GET 方法 404", async () => {
  const dir = makeFixture({});
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    // 空目录（无合法会话）→ 各数据端点 500（含 meta）
    for (const url of [
      "/api/totals",
      "/api/sessions",
      "/api/requests",
      "/api/groups?by=model",
      "/api/period?period=day",
      "/api/meta",
    ]) {
      const res = await fetch(new URL(url, server.url));
      assert.equal(res.status, 500, `${url} 应返回 500`);
      const body = (await res.json()) as { error: string; detail: string };
      assert.equal(typeof body.error, "string");
      assert.ok(body.error.length > 0);
      assert.equal(typeof body.detail, "string");
      assert.ok(body.detail.length > 0, "500 响应应附 detail 原因");
    }

    // 未知 API 路径 → 404 统一错误体
    const res404 = await fetch(new URL("/api/unknown", server.url));
    assert.equal(res404.status, 404);
    const body404 = (await res404.json()) as { error: string; detail: string };
    assert.ok(body404.error.length > 0 && body404.detail.length > 0);

    // 非 GET 方法 → 404（GET 端点方法不匹配；rename 为合法 POST 端点，ticket 06 起不再 404）
    const resPost = await fetch(new URL("/api/totals", server.url), { method: "POST" });
    assert.equal(resPost.status, 404);
    const resPut = await fetch(new URL("/api/sessions", server.url), { method: "PUT" });
    assert.equal(resPut.status, 404);
  } finally {
    await server.close();
    removeFixture(dir);
  }
});
