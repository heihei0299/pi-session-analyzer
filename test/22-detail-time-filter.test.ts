/**
 * Bugfix — 明细时间筛选（问题汇总新增两条）。
 *
 * 背景：
 * 1. 状态行「范围」固定显示全量数据范围（meta.dataRange），不随「今天/7天/30天」预设变化。
 * 2. 请求明细 tab 的 fetchRows 不带 since/until → 不应用时间筛选（选「今天」后仍显示全量，
 *    8/6 00:00-1:25 的请求藏在第 100+ 页）；且 filterFiles 按会话 header 过滤，跨天会话
 *    （header 8/5 但含 8/6 凌晨请求）整段丢弃。
 *
 * 修复（ticket 22，用户决策）：
 * - 状态行在存在筛选时显示当前筛选范围（纯日期 since 补 00:00 / until 补 23:59:59），否则显示数据范围。
 * - 明细 tab fetchRows/snapshot 携带 since/until（filterParams()）。
 * - /api/requests 时间过滤改为**消息级**（按消息 timestamp，跨天会话保留范围内请求）；
 *   /api/sessions、totals、groups、period 保持会话级（口径不变）。
 *
 * 断言：API 消息级/会话级差异、前端 fetchRows 携带筛选、状态行筛选范围逻辑、非法参数 400。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { startWebServer } from "../src/server.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

// 固定东八区（本地自然日语义，与 04/17/18 同风格）
process.env.TZ = "Asia/Shanghai";

/**
 * 跨天场景 fixture：
 * - 会话 A header 2026-08-05T11:37Z（本地 8/5 19:37）：m1 本地 8/6 00:30（跨天）、m2 本地 8/5 23:00
 * - 会话 B header 2026-08-06T02:00Z（本地 8/6 10:00）：m3 本地 8/6 10:30
 * 「今天 8/6」= since 2026-08-06（本地 00:00 = UTC 8/5 16:00）~ until 2026-08-06（UTC 8/6 15:59:59.999）
 */
function buildDir(): string {
  return makeFixture({
    "2026-08-05T11-37-35-229Z_a.jsonl": [
      sessionHeader({ id: "a1", timestamp: "2026-08-05T11:37:35.229Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }, { timestamp: "2026-08-05T16:30:00.000Z" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }, { timestamp: "2026-08-05T15:00:00.000Z" }),
    ],
    "2026-08-06T02-00-00-000Z_b.jsonl": [
      sessionHeader({ id: "b2", timestamp: "2026-08-06T02:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m2", usage: assistantUsage({ input: 300 }) }, { timestamp: "2026-08-06T02:30:00.000Z" }),
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

test("requests 明细消息级时间过滤：跨天会话保留范围内消息，剔除非范围内消息", async () => {
  await withServer(async (url) => {
    // 今天 8/6：m1（本地 8/6 00:30）保留、m2（本地 8/5 23:00）剔除、m3（本地 8/6 10:30）保留
    const d = (await (await fetch(new URL("/api/requests?since=2026-08-06&until=2026-08-06", url))).json()) as {
      rows: { timestamp: string; sessionId: string }[];
      total: number;
    };
    assert.equal(d.total, 2, "消息级：会话 A 的 m1 + 会话 B 的 m3");
    const ts = d.rows.map((r) => r.timestamp).sort();
    assert.deepEqual(ts, ["2026-08-05T16:30:00.000Z", "2026-08-06T02:30:00.000Z"], "m2（本地 8/5 23:00）应被剔除");
    assert.equal(d.rows.some((r) => r.sessionId === "a1"), true, "跨天会话 A 因含范围内消息而保留");
  });
});

test("sessions 保持会话级；totals 消息级（ticket 23：总览统计含跨天凌晨请求）", async () => {
  await withServer(async (url) => {
    const s = (await (await fetch(new URL("/api/sessions?since=2026-08-06&until=2026-08-06", url))).json()) as {
      rows: { sessionId: string }[];
      total: number;
    };
    assert.equal(s.total, 1, "sessions 会话级：仅会话 B（header 8/6）");
    assert.deepEqual(s.rows.map((r) => r.sessionId), ["b2"]);

    // totals 消息级：m1（本地 8/6 00:30，跨天会话 A）+ m3（本地 8/6 10:30，会话 B）
    const t = (await (await fetch(new URL("/api/totals?since=2026-08-06&until=2026-08-06", url))).json()) as {
      requests: number;
    };
    assert.equal(t.requests, 2, "totals 消息级：m1 + m3（含跨天会话 A 的凌晨请求）");
  });
});

test("明细不带筛选时仍全量（兼容既有行为）", async () => {
  await withServer(async (url) => {
    const d = (await (await fetch(new URL("/api/requests", url))).json()) as { total: number };
    assert.equal(d.total, 3);
    const s = (await (await fetch(new URL("/api/sessions", url))).json()) as { total: number };
    assert.equal(s.total, 2);
  });
});

test("requests 端点非法 since/until 仍 400（filtersFromParams 校验不因消息级绕过）", async () => {
  await withServer(async (url) => {
    for (const q of ["since=abc&until=2026-08-06", "since=2026-08-06&until=abc", "since=abc"]) {
      const res = await fetch(new URL("/api/requests?" + q, url));
      assert.equal(res.status, 400, `${q} 应 400`);
    }
  });
});

async function fetchHtml(): Promise<string> {
  let dir: string | null = null;
  const server = await startWebServer({ dir: (dir = makeFixture({ "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })] })), host: "127.0.0.1", port: 0 });
  try {
    return await (await fetch(server.url)).text();
  } finally {
    await server.close();
    removeFixture(dir);
  }
}

test("前端：fetchRows 携带 since/until（filterParams 并入），明细随时间预设筛选", async () => {
  const body = await fetchHtml();
  const fetchRows = body.match(/async function fetchRows\(kind\) \{([\s\S]*?)\n\}/)?.[1] ?? "";
  assert.match(fetchRows, /const params = filterParams\(\);/, "fetchRows 应携带时间筛选参数");
  assert.match(fetchRows, /params\.set\("page", String\(st\.page\)\);/, "fetchRows 仍传分页参数");

  // poll snapshot 明细分支同样携带筛选
  const snapshot = body.match(/async function snapshot\(\) \{([\s\S]*?)\n\}/)?.[1] ?? "";
  assert.match(snapshot, /const p = filterParams\(\);\s*\n\s*p\.set\("page"/, "snapshot 明细分支应携带筛选");
});

test("前端：状态行存在筛选时显示筛选范围，否则显示数据范围", async () => {
  const body = await fetchHtml();
  // updateStatusRange：纯日期补 00:00/23:59:59 + 兜底数据范围
  const update = body.match(/function updateStatusRange\(\) \{([\s\S]*?)\n\}/)?.[1] ?? "";
  assert.match(update, /fmtTimestamp\(lastMeta\.dataRange\.since\)/, "无筛选时兜底显示数据范围 since");
  assert.match(update, /fmtTimestamp\(lastMeta\.dataRange\.until\)/, "无筛选时兜底显示数据范围 until");
  assert.match(update, /\$\{since \?\? "开始"\} ~ \$\{until \?\? "现在"\}/, "筛选时显示当前范围");
  const fmt = body.match(/function filterRangeText\(\) \{([\s\S]*?)\n\}/)?.[1] ?? "";
  assert.match(fmt, /23:59:59/, "纯日期 until 补 23:59:59");
  assert.match(fmt, /00:00:00/, "纯日期 since 补 00:00:00");
  // 预设/自定义变更时更新状态行
  assert.match(body, /updateStatusRange\(\);\s*\n\s*refreshAll\(\);\s*\n\}/, "applyPreset 应调用 updateStatusRange");
  assert.match(body, /updateStatusRange\(\);\s*\n\s*refreshAll\(\);\s*\n\}/, "applyCustomRange 应调用 updateStatusRange");
});
