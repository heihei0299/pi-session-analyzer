import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli, parseArgs } from "../src/cli.ts";
import {
  makeFixture,
  removeFixture,
  sessionHeader,
  messageEntry,
  assistantUsage,
  parseTable,
} from "./helpers.ts";

test("S7 向后兼容：无子命令默认 totals 窗口，issue 01 行为不变", async () => {
  // parseArgs 默认窗口
  const args = parseArgs(["--dir", "/tmp/x"]);
  assert.equal(args.window, "totals");
  assert.equal(args.format, "table");

  // 无子命令运行 = totals 表格（9 列）
  const dir = makeFixture({
    "s.jsonl": [
      sessionHeader({ id: "s1", timestamp: "2026-07-31T01:00:00.000Z", cwd: "/p" }),
      messageEntry({ role: "assistant", model: "m", usage: assistantUsage() }),
    ],
  });
  try {
    const out = await runCli(["--dir", dir]);
    const row = parseTable(out);
    // totals 表头无会话ID/时间戳列，9 指标列
    assert.ok(!("会话ID" in row), "totals 表格不应有会话ID列");
    assert.equal(row["请求数"], "1");
    assert.equal(row["输入"], "100");
    assert.equal(row["总 token"], "350");
    assert.equal(row["花费"], "0.1");
  } finally {
    removeFixture(dir);
  }
});
