/**
 * Bugfix — 会话时间显示转本地时间。
 *
 * 背景：pi 会话文件的时间戳是 UTC ISO（如 2026-08-05T19:31:49.265Z），筛选参数已按
 * 本地时间语义（parseTimestamp 本地解释），但明细表/会话管理/状态行显示的是原始 UTC
 * 字符串——用户输入本地时间筛选、看到 UTC 时间显示，差 8 小时对不上。
 * 修复：新增 fmtTimestamp（UTC ISO → 本地 YYYY-MM-DD HH:MM:SS），应用于明细表
 * timestamp 列、会话管理行时间、状态行数据范围；导出 JSON/CSV 保持原始 UTC ISO。
 *
 * 断言（静态）：fmtTimestamp 定义用本地字段；三处显示位置均经 fmtTimestamp；
 * 明细表格式化表（METRIC_FMTS）挂接 timestamp。
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

test("时间显示全部转本地：fmtTimestamp 定义 + 明细表/会话管理/状态行挂接", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);

    // fmtTimestamp 定义：UTC ISO → 本地字段（getFullYear/getMonth/getDate/getHours/getMinutes/getSeconds）
    const fmtDef = body.match(/function fmtTimestamp\(iso\) \{([\s\S]*?)\n\}/)?.[1] ?? "";
    assert.ok(fmtDef.length > 0, "JS 应定义 fmtTimestamp");
    assert.match(fmtDef, /getFullYear\(\)/, "应取本地年份");
    assert.match(fmtDef, /getMonth\(\)/, "应取本地月份");
    assert.match(fmtDef, /getHours\(\)/, "应取本地小时（非 getUTCHours）");
    assert.match(fmtDef, /getMinutes\(\)/, "应取本地分钟");
    assert.match(fmtDef, /getSeconds\(\)/, "应取本地秒");
    assert.doesNotMatch(fmtDef, /getUTCFullYear|getUTCHours/, "不应取 UTC 字段");

    // 明细表：METRIC_FMTS 挂接 timestamp
    const metricFmts = body.match(/const METRIC_FMTS = \{([\s\S]*?)\n\}/)?.[1] ?? "";
    assert.match(metricFmts, /timestamp: fmtTimestamp/, "明细表 timestamp 列应经 fmtTimestamp");

    // 会话管理行时间
    assert.match(body, /<span class="ts">\$\{escapeHtml\(fmtTimestamp\(r\.timestamp\)\)\}<\/span>/, "会话管理行时间应经 fmtTimestamp");

    // 状态行数据范围（无筛选兜底；ticket 22 起筛选时优先显示筛选范围）
    const updateRange = body.match(/function updateStatusRange\(\) \{([\s\S]*?)\n\}/)?.[1] ?? "";
    assert.match(updateRange, /fmtTimestamp\(lastMeta\.dataRange\.since\)/, "状态行 since 应经 fmtTimestamp");
    assert.match(updateRange, /fmtTimestamp\(lastMeta\.dataRange\.until\)/, "状态行 until 应经 fmtTimestamp");

    // 导出（JSON/CSV）保持原始 UTC：导出路径不应出现 fmtTimestamp
    const exportData = body.match(/async function exportData\(format\) \{([\s\S]*?)\n\}/)?.[1] ?? "";
    assert.doesNotMatch(exportData, /fmtTimestamp/, "导出应保持原始 UTC ISO，不转本地");
  } finally {
    removeFixture(dir);
  }
});
