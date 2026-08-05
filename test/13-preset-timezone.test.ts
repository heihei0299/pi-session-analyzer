/**
 * 回归测试 — 时间预设（今天/7天/30天）按本地自然日语义。
 *
 * 背景：预设按钮曾用 UTC 日期生成 since/until（utcDateStr + Date.UTC），东八区凌晨
 * 「今天」查 UTC 新一天为空；后改为与自定义/CLI 统一按 UTC。现需求变更：全部时间
 * 筛选（预设 + 自定义）按本地时间适配——预设按钮按本地自然日生成日期参数
 * （dateStr 本地字段），后端 parseTimestamp 按本地时区解释。
 *
 * 断言：dateStr 取本地字段；applyPreset 预设分支用 dateStr（不复用 utcDateStr）；
 * 东八区凌晨本地 08-06 03:41 → 日期应为本地 08-06（不再是 UTC 前一天）。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { startWebServer } from "../src/server.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

async function fetchHtml(dir: string): Promise<string> {
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const res = await fetch(server.url);
    assert.equal(res.status, 200);
    return await res.text();
  } finally {
    await server.close();
  }
}

test("时间预设按本地自然日生成 since/until（dateStr 本地字段，不复用 utcDateStr）", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);

    // dateStr 用本地字段（getFullYear/getMonth/getDate）
    const dateDef = body.match(/function dateStr\(d\) \{([\s\S]*?)\n\}/)?.[1] ?? "";
    assert.ok(dateDef.length > 0, "JS 应定义 dateStr");
    assert.match(dateDef, /getFullYear\(\)/, "dateStr 应取本地年份");
    assert.match(dateDef, /getMonth\(\)/, "dateStr 应取本地月份");
    assert.match(dateDef, /getDate\(\)/, "dateStr 应取本地日期");

    // applyPreset 预设分支用 dateStr（本地自然日），按本地日期字段推算天数
    const applyPreset = body.match(/function applyPreset\(preset\) \{([\s\S]*?)\n\}/)?.[1] ?? "";
    assert.ok(applyPreset.length > 0, "JS 应定义 applyPreset");
    assert.match(applyPreset, /state\.since = dateStr\(since\);/, "预设 since 应经本地 dateStr");
    assert.match(applyPreset, /state\.until = dateStr\(now\);/, "预设 until 应经本地 dateStr");
    assert.match(applyPreset, /since\.setDate\(now\.getDate\(\) - days\)/, "预设天数应按本地日期推算");

    // 不应残留 UTC 语义的 utcDateStr / Date.UTC 推算
    assert.doesNotMatch(body, /utcDateStr/, "不应残留 utcDateStr（UTC 语义已废弃）");
    assert.doesNotMatch(applyPreset, /Date\.UTC\(now\.getUTCFullYear/, "预设不应按 UTC 字段推算");
  } finally {
    removeFixture(dir);
  }
});

test("东八区凌晨的本地日期语义：本地 08-06 03:41 → 日期应为本地 08-06", () => {
  // 锁定场景：本地（UTC+8）已进入 08-06、UTC 仍是 08-05。
  // 预设按钮按本地自然日（dateStr 本地字段）→ since/until 应为本地 08-06。
  const local = new Date("2026-08-06T03:41:00+08:00");
  const y = local.getFullYear();
  const m = local.getMonth();
  const d = local.getDate();
  const pad = (n: number) => String(n).padStart(2, "0");
  const localDate = `${y}-${pad(m + 1)}-${pad(d)}`;
  assert.equal(localDate, "2026-08-06", "东八区凌晨取本地日期应为当天（非 UTC 前一天）");

  // 7天边界按本地日期推算（跨月进位）：本地 08-06 - 6 天 = 07-31
  const since = new Date(y, m, d - 6);
  const sinceStr = `${since.getFullYear()}-${pad(since.getMonth() + 1)}-${pad(since.getDate())}`;
  assert.equal(sinceStr, "2026-07-31");
});
