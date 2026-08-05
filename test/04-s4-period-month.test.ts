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
} from "./helpers.ts";

test("S4 --period month 汇总：跨月会话归属各自月份，月末/月初边界正确", async () => {
  const dir = makeFixture({
    // 7/31：input 100
    "2026-07-31T10-00-00-000Z_a.jsonl": [
      sessionHeader({ id: "s-a", timestamp: "2026-07-31T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    ],
    // 8/1：input 200
    "2026-08-01T10-00-00-000Z_b.jsonl": [
      sessionHeader({ id: "s-b", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }),
    ],
    // 8/15：input 400
    "2026-08-15T10-00-00-000Z_c.jsonl": [
      sessionHeader({ id: "s-c", timestamp: "2026-08-15T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 400 }) }),
    ],
    // 9/1：input 800
    "2026-09-01T10-00-00-000Z_d.jsonl": [
      sessionHeader({ id: "s-d", timestamp: "2026-09-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 800 }) }),
    ],
  });
  try {
    const rows = parseTableRows(await runCli(["totals", "--dir", dir, "--period", "month"]));
    assert.equal(rows.length, 3, "三个月各一行");

    const m7 = rows.find((r) => r["月份"] === "2026-07-01");
    const m8 = rows.find((r) => r["月份"] === "2026-08-01");
    const m9 = rows.find((r) => r["月份"] === "2026-09-01");
    assert.ok(m7 && m8 && m9, "月份键为当月 1 日");

    // 7 月：input 100；8 月：200+400=600；9 月：800
    assert.equal(m7!["输入"], "100");
    assert.equal(m8!["请求数"], "2");
    assert.equal(m8!["输入"], "600");
    assert.equal(m9!["输入"], "800");

    // 各月之和 = 总窗口
    const sum = (k: string) => rows.reduce((acc, r) => acc + Number(r[k]), 0);
    assert.equal(sum("请求数"), 4);
    assert.equal(sum("输入"), 1500);

    // CSV 输出
    const csv = await runCli(["totals", "--dir", dir, "--period", "month", "--format", "csv"]);
    assert.ok(csv.startsWith("period,requests,input"), "CSV 表头含 period 与指标列");
    assert.equal(csv.trim().split("\n").length, 4, "CSV 表头 + 3 行");
  } finally {
    removeFixture(dir);
  }
});
