/**
 * Ticket 04 — 展示口径收口。
 * 覆盖：period 按 message timestamp 归属；WebUI 保留 All 已知成本并标注含 unpriced 源。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { periodRowsFromFiles, type SessionFileData } from "../src/session-data.ts";

test("T4-1 periodRowsFromFiles 按消息 timestamp 拆分跨天 rollout", () => {
  const files = [{
    sessionId: "s1",
    timestamp: "2026-09-08T23:59:00.000Z",
    cwd: "/synthetic",
    fileName: "s1.jsonl",
    isTask: false,
    items: [
      { timestamp: "2026-09-08T23:59:40.000Z", model: "m1", usage: { input: 10, output: 3, cacheRead: 2, cacheWrite: 0, cost: { total: 0 } } },
      { timestamp: "2026-09-09T00:00:30.000Z", model: "m1", usage: { input: 20, output: 6, cacheRead: 4, cacheWrite: 0, cost: { total: 0 } } },
    ],
  }] as unknown as SessionFileData[];

  const rows = periodRowsFromFiles(files, "day");
  assert.equal(rows.length, 2, "跨天 rollout 必须拆成两天");
  assert.equal(rows[0].period, "2026-09-08");
  assert.equal(rows[0].totalTokens, 15, "input 10 + cacheRead 2 + output 3");
  assert.equal(rows[1].period, "2026-09-09");
  assert.equal(rows[1].totalTokens, 30, "input 20 + cacheRead 4 + output 6");
});

test("T4-2 WebUI 对 All 的部分可用成本保留金额并标注含 unpriced 源", () => {
  const html = readFileSync(join("src", "webui.html"), "utf8");
  assert.match(html, /function fmtCostCell/, "应有统一的 costStatus 展示函数");
  assert.match(html, /含 unpriced 源/, "必须显示含 unpriced 源标注");
  assert.match(html, /部分可用/, "必须显示部分可用标注");
  assert.match(html, /costStatus === "unpriced" && cost > 0/, "只有含未知源且已知金额非零时才走部分可用分支");
  assert.doesNotMatch(html, /cost:(v,r)=\(r\?\.costStatus==="unpriced"\|\|v===0\?UNPRICED:fmtCost\(v\)\)/, "分组/明细成本不能再直接吞掉部分可用金额");
});
