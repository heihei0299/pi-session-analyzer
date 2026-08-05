import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage, zeroUsage, parseTable } from "./helpers.ts";

test("S5 全0失败消息：全 0 usage 的 assistant 消息计入请求数、token 为 0", async () => {
  const dir = makeFixture({
    "session.jsonl": [
      sessionHeader(),
      // 1 条正常 assistant usage（input=100）
      messageEntry({ role: "assistant", model: "m", usage: assistantUsage({ input: 100 }) }),
      // 1 条全 0 usage 的失败/中止消息
      messageEntry({ role: "assistant", model: "m", stopReason: "aborted", usage: zeroUsage() }),
    ],
  });
  try {
    const out = await runCli(["--dir", dir]);
    const row = parseTable(out);

    // 请求数 = 2（含失败消息）；token 各列仍只有正常消息的值
    assert.equal(row["请求数"], "2", "请求数应为 2（含全 0 失败消息）");
    assert.equal(row["输入"], "100", "输入应为 100（失败消息 input=0）");
    assert.equal(row["输出"], "50", "输出应为 50");
    assert.equal(row["推理"], "20", "推理应为 20");
  } finally {
    removeFixture(dir);
  }
});
