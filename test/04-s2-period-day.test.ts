import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import {
  makeFixture,
  removeFixture,
  sessionHeader,
  messageEntry,
  assistantUsage,
  parseTableRows,
  parseTable,
} from "./helpers.ts";

test("S2 --period day 汇总：每天一行，数字=期内会话之和，与总窗口对得上", async () => {
  const dir = makeFixture({
    // 8/1：input 100
    "2026-08-01T10-00-00-000Z_a.jsonl": [
      sessionHeader({ id: "s-a", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    ],
    // 8/1：input 200（同一天两个会话）
    "2026-08-01T18-00-00-000Z_b.jsonl": [
      sessionHeader({ id: "s-b", timestamp: "2026-08-01T18:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }),
    ],
    // 8/2：input 400
    "2026-08-02T09-00-00-000Z_c.jsonl": [
      sessionHeader({ id: "s-c", timestamp: "2026-08-02T09:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 400 }) }),
    ],
  });
  try {
    const rows = parseTableRows(await runCli(["totals", "--dir", dir, "--period", "day"]));
    assert.equal(rows.length, 2, "两天各一行");

    const d1 = rows.find((r) => r["日期"] === "2026-08-01");
    const d2 = rows.find((r) => r["日期"] === "2026-08-02");
    assert.ok(d1 && d2, "两行日期键正确");

    // 8/1：两个会话合并 → input 100+200=300，请求数 2
    assert.equal(d1!["请求数"], "2");
    assert.equal(d1!["输入"], "300");
    // 8/2：1 个会话 → input 400
    assert.equal(d2!["请求数"], "1");
    assert.equal(d2!["输入"], "400");

    // 各日之和 = 未分组 totals
    const sum = (k: string) => rows.reduce((acc, r) => acc + Number(r[k]), 0);
    assert.equal(sum("请求数"), 3, "按日汇总请求数之和 = 总窗口");
    assert.equal(sum("输入"), 700, "按日汇总输入之和 = 100+200+400");

    // 与 totals 窗口对比（精确断言）
    const totalsRow = parseTable(await runCli(["totals", "--dir", dir]));
    assert.equal(totalsRow["输入"], "700", "totals 输入 700");
    assert.equal(totalsRow["请求数"], "3", "totals 请求数 3");

    // JSON 输出
    const json = JSON.parse(await runCli(["totals", "--dir", dir, "--period", "day", "--format", "json"]));
    assert.equal(json.window, "totals");
    assert.equal(json.period, "day");
    assert.equal(json.rows.length, 2);
    const jd1 = json.rows.find((r: Record<string, unknown>) => r.period === "2026-08-01");
    assert.equal(jd1.requests, 2);
    assert.equal(jd1.input, 300);
  } finally {
    removeFixture(dir);
  }
});
