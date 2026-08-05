/**
 * Bugfix — 自定义时间范围：本地时间语义 + 时分下拉选择。
 * 1. parseTimestamp 完整时间戳与纯日期均按本地时区解释（东八区：本地 14:00 = UTC 06:00，
 *    本地 8/5 全天 = UTC 8/4 16:00 ~ 8/5 15:59:59.999）——与预设按钮（本地自然日）一致。
 * 2. 自定义输入 UI 为 date + 时分下拉（时 00-23 / 分 00/15/30/45）：Firefox 原生 time
 *    输入框点击时分字段无反应（平台限制），下拉选择稳定可点。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { parseTimestamp } from "../src/analyze.ts";
import { startWebServer } from "../src/server.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

// 固定东八区（node --test 每文件独立进程，TZ 不影响其他文件）
process.env.TZ = "Asia/Shanghai";

// ---------- parseTimestamp 本地时间语义 ----------

test("parseTimestamp 完整时间戳按本地时区解释（东八区：本地 14:00 = UTC 06:00）", () => {
  const ms = parseTimestamp("2026-08-05T14:00", false);
  assert.equal(new Date(ms).toISOString(), "2026-08-05T06:00:00.000Z", "无时区完整时间戳按本地解释（不再是 UTC 补 Z）");
});

test("parseTimestamp 纯日期按本地自然日（东八区：8/5 = UTC 8/4 16:00 起）", () => {
  assert.equal(new Date(parseTimestamp("2026-08-05", false)).toISOString(), "2026-08-04T16:00:00.000Z");
  assert.equal(new Date(parseTimestamp("2026-08-05", true)).toISOString(), "2026-08-05T15:59:59.999Z");
});

test("parseTimestamp 带时区后缀原样解析", () => {
  assert.equal(new Date(parseTimestamp("2026-08-05T14:00Z", false)).toISOString(), "2026-08-05T14:00:00.000Z");
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

test("自定义时间输入为 date + 时分下拉，input 事件输入即生效", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);

    // 6 个输入控件：since/until × (date + hour + minute)
    for (const id of ["since-date", "since-hour", "since-minute", "until-date", "until-hour", "until-minute"]) {
      assert.match(body, new RegExp(`id="${id}"`), `HTML 应含 ${id}`);
    }
    assert.match(body, /<input type="date" id="since-date"/, "since 应为 date 输入框");
    assert.match(body, /<select id="since-hour"/, "since 应为小时下拉");
    assert.match(body, /<select id="since-minute"/, "since 应为分钟下拉");
    assert.doesNotMatch(body, /type="time"/, "不应再使用原生 time 输入框（Firefox 点击时分字段无反应）");

    // 时 00-23、分 00/15/30/45 选项
    const hourSel = body.match(/<select id="since-hour"[^>]*>([\s\S]*?)<\/select>/)?.[1] ?? "";
    assert.ok(hourSel.includes("00") && hourSel.includes("23"), "小时下拉应含 00-23");
    const minSel = body.match(/<select id="since-minute"[^>]*>([\s\S]*?)<\/select>/)?.[1] ?? "";
    assert.match(minSel, /value="(00|15|30|45)"/, "分钟下拉应为 00/15/30/45 档位");

    // 拼接逻辑：只有日期 → 纯日期；日期+时分 → YYYY-MM-DDTHH:MM（本地时间，后端按本地解释）
    assert.match(body, /timeOf\(sh, sm\)/, "应有时分拼接辅助");
    assert.match(body, /since: sd \? \(timeOf\(sh, sm\) \? `\$\{sd\}T\$\{timeOf\(sh, sm\)\}` : sd\) : null/, "since 拼接：有日期必传，有时分拼 T");

    // input/change 事件绑定（输入即生效，无需失焦）
    assert.match(body, /\["since-date", "since-hour", "since-minute", "until-date", "until-hour", "until-minute"\]/, "应遍历 6 个输入控件绑定");
    assert.match(body, /addEventListener\(id\.endsWith\("-date"\) \? "input" : "change", applyCustomRange\)/, "date 绑 input、select 绑 change，均触发 applyCustomRange");

    // 不应残留旧 datetime-local 引用
    assert.doesNotMatch(body, /id="since-input"/, "不应残留旧 since-input");
    assert.doesNotMatch(body, /id="until-input"/, "不应残留旧 until-input");
  } finally {
    removeFixture(dir);
  }
});
