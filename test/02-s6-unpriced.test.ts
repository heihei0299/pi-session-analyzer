import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import {
  makeFixture,
  removeFixture,
  sessionHeader,
  messageEntry,
  assistantUsage,
  zeroUsage,
  parseTable,
  parseTableRows,
} from "./helpers.ts";

test("S6 零花费标注：cost 全 0 的会话/请求在表格显示「费率未配置（免费/未定价）」，JSON/CSV 保持数字 0", async () => {
  const dir = makeFixture({
    // 有花费会话：cost.total = 0.2
    "paid.jsonl": [
      sessionHeader({ id: "paid-sess", timestamp: "2026-07-31T01:00:00.000Z", cwd: "/p" }),
      messageEntry({ role: "assistant", model: "m", usage: assistantUsage() }),
      messageEntry({ role: "assistant", model: "m", usage: assistantUsage() }),
    ],
    // 零花费会话：全 0 cost（失败消息 + 正常但费率为 0 的消息）
    "free.jsonl": [
      sessionHeader({ id: "free-sess", timestamp: "2026-07-31T02:00:00.000Z", cwd: "/f" }),
      messageEntry({
        role: "assistant",
        model: "m",
        usage: {
          input: 50,
          output: 20,
          cacheRead: 10,
          cacheWrite: 0,
          totalTokens: 80,
          cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
        },
      }),
      messageEntry({ role: "assistant", model: "m", stopReason: "aborted", usage: zeroUsage() }),
    ],
  });
  try {
    // 表格：零花费会话标注
    const table = parseTableRows(await runCli(["sessions", "--dir", dir]));
    const free = table.find((r) => r["会话ID"] === "free-sess");
    const paid = table.find((r) => r["会话ID"] === "paid-sess");
    assert.ok(free && paid);
    assert.equal(free!["花费"], "费率未配置（免费/未定价）", "零花费会话应标注费率未配置（免费/未定价）");
    assert.equal(paid!["花费"], "0.2", "有花费会话显示数字");

    // totals 窗口同样标注（全部会话零花费）
    const totalsAllZero = parseTable(await runCli(["totals", "--dir", dir]));
    assert.notEqual(totalsAllZero["花费"], "0", "总窗口非全零");

    // JSON：保持数字 0
    const json = JSON.parse(await runCli(["sessions", "--dir", dir, "--format", "json"]));
    const freeJson = json.rows.find((r: Record<string, unknown>) => r.sessionId === "free-sess");
    assert.equal(freeJson.cost, 0);
    assert.equal(typeof freeJson.cost, "number");

    // CSV：保持数字 0
    const csv = await runCli(["sessions", "--dir", dir, "--format", "csv"]);
    const csvFreeLine = csv.split("\n").find((l) => l.includes("free-sess"));
    assert.ok(csvFreeLine, "CSV 含 free-sess 行");
    assert.ok(csvFreeLine!.includes(",0,") || csvFreeLine!.endsWith(",0"), "CSV 花费为数字 0");
    assert.ok(!csvFreeLine!.includes("费率未配置"), "CSV 不含标注文本");
  } finally {
    removeFixture(dir);
  }
});
