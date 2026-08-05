import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage, parseTable } from "./helpers.ts";

test("S4 口径A过滤：toolResult/compaction/branch_summary 带 usage 全部忽略", async () => {
  const dir = makeFixture({
    "session.jsonl": [
      sessionHeader(),
      // 1 条合法 assistant usage（input=100）
      messageEntry({ role: "assistant", model: "m", usage: assistantUsage({ input: 100 }) }),
      // toolResult 带 usage → 必须忽略
      messageEntry({ role: "toolResult", model: "m", usage: assistantUsage({ input: 500 }) }),
      // user 消息带 usage → 忽略
      messageEntry({ role: "user", usage: assistantUsage({ input: 600 }) }),
      // compaction 条目（顶层带 usage，非 message）→ 忽略
      { type: "compaction", id: "c1", parentId: null, timestamp: "2026-07-31T02:00:00.000Z", usage: assistantUsage({ input: 700 }) },
      // branch_summary 条目 → 忽略
      { type: "branch_summary", id: "b1", parentId: null, timestamp: "2026-07-31T02:00:00.000Z", usage: assistantUsage({ input: 800 }) },
    ],
  });
  try {
    const out = await runCli(["--dir", dir]);
    const row = parseTable(out);

    // 只有 1 条 assistant 计入：请求数 1、输入 100
    assert.equal(row["请求数"], "1", "请求数应为 1");
    assert.equal(row["输入"], "100", "输入应为 100（不含 500/600/700/800）");
  } finally {
    removeFixture(dir);
  }
});
