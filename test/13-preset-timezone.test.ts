/**
 * Bugfix — 时间预设时区错位回归测试。
 *
 * 背景：applyPreset（今天/7天/30天）用本地时区日期（dateStr → getFullYear/getMonth/getDate）
 * 生成 since/until，但后端/CLI 按 UTC 解释日期参数（analyze.ts parseTimestamp 用 Date.UTC，
 * spec 明确「直接映射 CLI 语义」）。东八区凌晨（本地已进入新一天、UTC 尚未）时，
 * 「今天」查 UTC 新一天 → 数据为空；7天/30天边界也错 8 小时。
 *
 * 修复：预设按钮改用 utcDateStr（getUTC* 字段）按 UTC 日期推算，与后端语义对齐。
 * 断言：utcDateStr 存在且取 UTC 字段；applyPreset 预设分支使用 utcDateStr（不复用本地 dateStr）。
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

test("时间预设按 UTC 日期生成 since/until（东八区凌晨不丢当天数据）", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);

    // utcDateStr 定义存在且取 UTC 字段（getUTCFullYear/getUTCMonth/getUTCDate）
    const utcDef = body.match(/function utcDateStr\(d\) \{([\s\S]*?)\n\}/)?.[1] ?? "";
    assert.ok(utcDef.length > 0, "JS 应定义 utcDateStr");
    assert.match(utcDef, /getUTCFullYear\(\)/, "utcDateStr 应取 UTC 年份");
    assert.match(utcDef, /getUTCMonth\(\)/, "utcDateStr 应取 UTC 月份");
    assert.match(utcDef, /getUTCDate\(\)/, "utcDateStr 应取 UTC 日期");

    // applyPreset 的预设分支（今天/7天/30天）必须用 utcDateStr，且按 UTC 字段推算天数
    const applyPreset = body.match(/function applyPreset\(preset\) \{([\s\S]*?)\n\}/)?.[1] ?? "";
    assert.ok(applyPreset.length > 0, "JS 应定义 applyPreset");
    assert.match(applyPreset, /state\.since = utcDateStr\(since\);/, "预设 since 应经 utcDateStr");
    assert.match(applyPreset, /state\.until = utcDateStr\(now\);/, "预设 until 应经 utcDateStr");
    assert.match(applyPreset, /Date\.UTC\(now\.getUTCFullYear\(\), now\.getUTCMonth\(\), now\.getUTCDate\(\) - days\)/, "预设天数应整体按 UTC 推算");
    assert.doesNotMatch(applyPreset, /state\.since = dateStr\(/, "预设不应回退用本地 dateStr");
  } finally {
    removeFixture(dir);
  }
});

test("东八区凌晨的 UTC 日期语义：本地 08-06 03:41 → UTC 日期 08-05", () => {
  // 锁定 bug 场景：本地（UTC+8）已进入 08-06，但 UTC 仍是 08-05。
  // 后端按 UTC 解释日期参数，因此预设生成的 since/until 必须是 08-05（UTC）才能命中当天数据。
  const local = new Date("2026-08-06T03:41:00+08:00");
  const y = local.getUTCFullYear();
  const m = local.getUTCMonth();
  const d = local.getUTCDate();
  const pad = (n: number) => String(n).padStart(2, "0");
  const utcDate = `${y}-${pad(m + 1)}-${pad(d)}`;
  assert.equal(utcDate, "2026-08-05", "东八区凌晨取 UTC 日期应为前一天");

  // 7天边界按 UTC 推算（跨月进位正确）：UTC 08-05 - 6 天 = 07-30
  const since = new Date(Date.UTC(y, m, d - 6));
  const sinceStr = `${since.getUTCFullYear()}-${pad(since.getUTCMonth() + 1)}-${pad(since.getUTCDate())}`;
  assert.equal(sinceStr, "2026-07-30");
});
