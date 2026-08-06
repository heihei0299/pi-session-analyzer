/**
 * Bugfix — 明细端点服务端分页（问题汇总 #3：明细 tab 刷新 CPU 飙升 + Firefox aborted）。
 *
 * 背景：/api/sessions、/api/requests 每次全量派生并传输（真实数据 /api/requests 26.7MB），
 * 明细 tab 连点刷新导致 server 端反复 JSON.stringify 全量 + 浏览器反复下载解析，
 * CPU 飙升（峰值 152%）且 26.7MB 传输窗口易被刷新导航中止（"The operation was aborted."）。
 * 修复：两端点支持 page/size/sortKey/sortDir 可选参数（须成对），响应含 total；
 * 前端明细 tab 每页请求（~20KB）。
 *
 * 断言：分页行为 / 服务端排序（数字列、字符串列、cache 别名）/ 参数校验 400 / 无参兼容全量。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { startWebServer } from "../src/server.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

/** 3 会话 × 5 条 assistant 消息：cost 递增、cacheRead+cacheWrite 递增、文件名时间戳 08-01~08-03 */
function buildDir(): string {
  return makeFixture({
    "2026-08-01T10-00-00-000Z_a.jsonl": [
      sessionHeader({ id: "a1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj/a" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100, cacheRead: 0, cacheWrite: 0, cost: { total: 0.1 } }) }),
    ],
    "2026-08-02T10-00-00-000Z_b.jsonl": [
      sessionHeader({ id: "b2", timestamp: "2026-08-02T10:00:00.000Z", cwd: "/proj/b" }),
      messageEntry({ role: "assistant", model: "m2", usage: assistantUsage({ input: 300, cacheRead: 10, cacheWrite: 5, cost: { total: 0.3 } }) }),
      messageEntry({ role: "assistant", model: "m2", usage: assistantUsage({ input: 400, cacheRead: 20, cacheWrite: 6, cost: { total: 0.4 } }) }),
    ],
    "2026-08-03T10-00-00-000Z_c.jsonl": [
      sessionHeader({ id: "c3", timestamp: "2026-08-03T10:00:00.000Z", cwd: "/proj/c" }),
      messageEntry({ role: "assistant", model: "m3", usage: assistantUsage({ input: 500, cacheRead: 30, cacheWrite: 7, cost: { total: 0.5 } }) }),
      messageEntry({ role: "assistant", model: "m3", usage: assistantUsage({ input: 200, cacheRead: 0, cacheWrite: 0, cost: { total: 0.2 } }) }),
    ],
  });
}

async function withServer(fn: (url: string) => Promise<void>): Promise<void> {
  const dir = buildDir();
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    await fn(server.url);
  } finally {
    await server.close();
    removeFixture(dir);
  }
}

test("分页：page/size 只返回当前页 + total 为全量行数；越界页返回空 rows", async () => {
  await withServer(async (url) => {
    // 全量 5 条
    const all = (await (await fetch(new URL("/api/requests", url))).json()) as {
      rows: Record<string, unknown>[];
      total: number;
      page?: number;
      size?: number;
    };
    assert.equal(all.rows.length, 5);
    assert.equal(all.total, 5);
    assert.equal(all.page, undefined, "无参数不分页：不返回 page");
    assert.equal(all.size, undefined, "无参数不分页：不返回 size");

    // page=1&size=2 → 2 行
    const p1 = (await (await fetch(new URL("/api/requests?page=1&size=2", url))).json()) as {
      rows: Record<string, unknown>[];
      total: number;
      page: number;
      size: number;
    };
    assert.equal(p1.rows.length, 2);
    assert.equal(p1.total, 5);
    assert.equal(p1.page, 1);
    assert.equal(p1.size, 2);

    // page=3&size=2 → 剩 1 行
    const p3 = (await (await fetch(new URL("/api/requests?page=3&size=2", url))).json()) as {
      rows: Record<string, unknown>[];
      total: number;
    };
    assert.equal(p3.rows.length, 1);

    // page=9 越界 → 0 行（total 不变）
    const p9 = (await (await fetch(new URL("/api/requests?page=9&size=2", url))).json()) as {
      rows: Record<string, unknown>[];
      total: number;
    };
    assert.equal(p9.rows.length, 0);
    assert.equal(p9.total, 5);

    // sessions 同样分页
    const s1 = (await (await fetch(new URL("/api/sessions?page=1&size=2", url))).json()) as {
      rows: Record<string, unknown>[];
      total: number;
    };
    assert.equal(s1.rows.length, 2);
    assert.equal(s1.total, 3);
  });
});

test("分页不改变行内容：分页行集 = 全量行集（无排序时按派生顺序切分）", async () => {
  await withServer(async (url) => {
    const all = (await (await fetch(new URL("/api/requests", url))).json()) as { rows: Record<string, unknown>[] };
    const p1 = (await (await fetch(new URL("/api/requests?page=1&size=2", url))).json()) as { rows: Record<string, unknown>[] };
    const p2 = (await (await fetch(new URL("/api/requests?page=2&size=2", url))).json()) as { rows: Record<string, unknown>[] };
    const p3 = (await (await fetch(new URL("/api/requests?page=3&size=2", url))).json()) as { rows: Record<string, unknown>[] };
    const merged = [...p1.rows, ...p2.rows, ...p3.rows];
    assert.deepEqual(
      merged.map((r) => r.timestamp),
      all.rows.map((r) => r.timestamp),
      "分页重组应等于全量顺序",
    );
  });
});

test("排序：数字列 cost 升降序；cache 别名 = cacheRead+cacheWrite；字符串列 displayName", async () => {
  await withServer(async (url) => {
    // cost desc → 0.5, 0.4, 0.3, 0.2, 0.1
    const costDesc = (await (await fetch(new URL("/api/requests?sortKey=cost&sortDir=desc", url))).json()) as {
      rows: Record<string, unknown>[];
    };
    assert.deepEqual(
      costDesc.rows.map((r) => r.cost),
      [0.5, 0.4, 0.3, 0.2, 0.1],
      "cost 数字列应降序",
    );

    // cache 别名（cacheRead+cacheWrite）：m5:37, m4:26, m3:15, 其余 0
    const cacheDesc = (await (await fetch(new URL("/api/requests?sortKey=cache&sortDir=desc", url))).json()) as {
      rows: Record<string, unknown>[];
    };
    assert.deepEqual(
      cacheDesc.rows.slice(0, 3).map((r) => (r.cacheRead as number) + (r.cacheWrite as number)),
      [37, 26, 15],
      "cache 别名应按 cacheRead+cacheWrite 排序",
    );

    // displayName 字符串 localeCompare：08-01 < 08-02 < 08-03（时间戳前缀）
    const nameAsc = (await (await fetch(new URL("/api/sessions?sortKey=displayName&sortDir=asc", url))).json()) as {
      rows: Record<string, unknown>[];
    };
    assert.equal(nameAsc.rows.length, 3);
    assert.equal(nameAsc.rows[0].displayName, "2026-08-01T10-00-00-000Z");
    assert.equal(nameAsc.rows[2].displayName, "2026-08-03T10-00-00-000Z");

    // asc / desc 顺序相反（timestamp）
    const tsAsc = (await (await fetch(new URL("/api/requests?sortKey=timestamp&sortDir=asc", url))).json()) as {
      rows: Record<string, unknown>[];
    };
    const tsDesc = (await (await fetch(new URL("/api/requests?sortKey=timestamp&sortDir=desc", url))).json()) as {
      rows: Record<string, unknown>[];
    };
    assert.deepEqual(
      [...tsDesc.rows].reverse().map((r) => r.timestamp),
      tsAsc.rows.map((r) => r.timestamp),
      "desc 反转应等于 asc",
    );
  });
});

test("排序 + 分页组合：先排序后切页", async () => {
  await withServer(async (url) => {
    // cost desc + size=2 → 0.5, 0.4
    const r = (await (await fetch(new URL("/api/requests?sortKey=cost&sortDir=desc&page=1&size=2", url))).json()) as {
      rows: Record<string, unknown>[];
      total: number;
    };
    assert.deepEqual(
      r.rows.map((x) => x.cost),
      [0.5, 0.4],
    );
    assert.equal(r.total, 5);
  });
});

test("参数校验：非法 page/size/sortKey/sortDir 及不成对参数 → 400 统一错误体", async () => {
  await withServer(async (url) => {
    const bad = [
      "/api/requests?page=0&size=2",
      "/api/requests?page=-1&size=2",
      "/api/requests?page=abc&size=2",
      "/api/requests?page=1&size=0",
      "/api/requests?page=1&size=201",
      "/api/requests?page=1&size=abc",
      "/api/requests?page=1", // 缺 size
      "/api/requests?size=20", // 缺 page
      "/api/requests?sortKey=bogus&sortDir=asc",
      "/api/requests?sortKey=timestamp&sortDir=up",
      "/api/requests?sortKey=timestamp", // 缺 sortDir
      "/api/sessions?page=1", // sessions 同样校验
      "/api/sessions?sortKey=bogus&sortDir=desc",
    ];
    for (const path of bad) {
      const res = await fetch(new URL(path, url));
      assert.equal(res.status, 400, `${path} 应返回 400`);
      const body = (await res.json()) as { error: string; detail: string };
      assert.equal(typeof body.error, "string");
      assert.ok(body.error.length > 0);
      assert.equal(typeof body.detail, "string");
      assert.ok(body.detail.length > 0);
    }
  });
});
