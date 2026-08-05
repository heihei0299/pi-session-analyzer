/**
 * Bugfix — 自定义时间范围（问题汇总追加）：
 * 1. parseTimestamp 完整时间戳统一按 UTC 解释（无时区补 Z）——修复 Date.parse 按本地时区
 *    解释导致的「自定义筛选与预设/CLI 语义不一致」（预设按 UTC、自定义按本地，差 8 小时）。
 * 2. 自定义输入 UI 从 datetime-local 改为 date + time 组合（Firefox 的 datetime-local 无法
 *    弹出时分选择器，date 可弹日历、time 有 spinner）；绑定 input 事件输入即生效（原 change
 *    需失焦才触发，用户输入后不点别处会感觉「筛选不生效」）。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { parseTimestamp } from "../src/analyze.ts";
import { startWebServer } from "../src/server.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

// ---------- parseTimestamp UTC 语义 ----------

test("parseTimestamp 完整时间戳按 UTC 解释（无时区补 Z，不再按本地时区）", () => {
  const ms = parseTimestamp("2026-08-05T14:00", false);
  assert.equal(new Date(ms).toISOString(), "2026-08-05T14:00:00.000Z", "无时区完整时间戳应补 Z 按 UTC（东八区下 Date.parse 原按本地=UTC 06:00）");
});

test("parseTimestamp 带时区后缀原样解析 + 纯日期不受影响", () => {
  assert.equal(new Date(parseTimestamp("2026-08-05T14:00Z", false)).toISOString(), "2026-08-05T14:00:00.000Z");
  assert.equal(new Date(parseTimestamp("2026-08-05", false)).toISOString(), "2026-08-05T00:00:00.000Z");
  assert.equal(new Date(parseTimestamp("2026-08-05", true)).toISOString(), "2026-08-05T23:59:59.999Z");
});

// ---------- 前端 date + time 组合静态断言 ----------

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

test("自定义时间输入为 date+time 组合，input 事件输入即生效", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);

    // 4 个输入框：since/until × date/time
    for (const id of ["since-date", "since-time", "until-date", "until-time"]) {
      assert.match(body, new RegExp(`id="${id}"`), `HTML 应含 ${id}`);
    }
    assert.match(body, /<input type="date" id="since-date"/, "since 应为 date 输入框");
    assert.match(body, /<input type="time" id="since-time"/, "since 应为 time 输入框");

    // 拼接逻辑：只有日期 → 纯日期；日期+时间 → YYYY-MM-DDTHH:MM（后端补 Z 按 UTC）
    assert.match(body, /since: sd \? \(st \? `\$\{sd\}T\$\{st\}` : sd\) : null/, "since 拼接：有日期必传，有时分拼 T");
    assert.match(body, /until: ud \? \(ut \? `\$\{ud\}T\$\{ut\}` : ud\) : null/, "until 拼接同上");

    // input 事件绑定（输入即生效，无需失焦）
    assert.match(body, /for \(const id of \["since-date", "since-time", "until-date", "until-time"\]\)/, "应遍历 4 个输入框绑定");
    assert.match(body, /addEventListener\("input", applyCustomRange\)/, "应绑定 input 事件");

    // 不应残留旧 datetime-local 引用
    assert.doesNotMatch(body, /id="since-input"/, "不应残留旧 since-input");
    assert.doesNotMatch(body, /id="until-input"/, "不应残留旧 until-input");
  } finally {
    removeFixture(dir);
  }
});
