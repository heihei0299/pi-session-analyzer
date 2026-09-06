import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage, parseTable } from "./helpers.ts";

test("S4 四载体全部计入：toolResult/compaction/branch_summary 与 assistant 同口径", async () => {
  const dir = makeFixture({
    "session.jsonl": [
      sessionHeader(),
      // 1 条合法 assistant usage（input=100）
      messageEntry({ role: "assistant", model: "m", usage: assistantUsage({ input: 100 }) }),
      // toolResult 带 usage → 计入（四载体）
      messageEntry({ role: "toolResult", model: "m", usage: assistantUsage({ input: 500 }) }),
      // user 消息带 usage → 忽略（非四载体）
      messageEntry({ role: "user", usage: assistantUsage({ input: 600 }) }),
      // compaction 条目（顶层带 usage，非 message）→ 计入
      { type: "compaction", id: "c1", parentId: null, timestamp: "2026-07-31T02:00:00.000Z", usage: assistantUsage({ input: 700 }) },
      // branch_summary 条目 → 计入
      { type: "branch_summary", id: "b1", parentId: null, timestamp: "2026-07-31T02:00:00.000Z", usage: assistantUsage({ input: 800 }) },
    ],
  });
  try {
    const out = await runCli(["--dir", dir]);
    const row = parseTable(out);

    // 四载体计入：assistant 100 + toolResult 500 + compaction 700 + branch_summary 800 = 2100，请求数 4（user 忽略）
    assert.equal(row["请求数"], "4", "请求数应为 4（四载体）");
    assert.equal(row["输入"], "2,100", "输入应为 2100（100+500+700+800）");
  } finally {
    removeFixture(dir);
  }
});
