import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage, parseTable } from "./helpers.ts";

test("S4b 健壮性：usage 为 null / 缺失的消息不崩溃、不计入", async () => {
  const dir = makeFixture({
    "session.jsonl": [
      sessionHeader(),
      // 合法 assistant usage（input=100）
      messageEntry({ role: "assistant", model: "m", usage: assistantUsage({ input: 100 }) }),
      // assistant 消息但 usage: null → 不计入、不崩溃
      messageEntry({ role: "assistant", model: "m", usage: null }),
      // assistant 消息但 usage 缺失 → 不计入（口径 A：携带 usage 才计）
      messageEntry({ role: "assistant", model: "m" }),
      // usage 是非法 JSON 字符串 → 坏行跳过
      "{ this is not json",
    ],
  });
  try {
    const out = await runCli(["--dir", dir]);
    const row = parseTable(out);

    // 只有 1 条带 usage 的计入
    assert.equal(row["请求数"], "1", "请求数应为 1（usage null/缺失不计入）");
    assert.equal(row["输入"], "100", "输入应为 100");
  } finally {
    removeFixture(dir);
  }
});
