import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import {
  makeFixture,
  removeFixture,
  sessionHeader,
  messageEntry,
  assistantUsage,
  parseTable,
} from "./helpers.ts";

test("S1 最小闭环：单个合法会话 + 一条 assistant usage 消息 → 表格 9 列且数字正确", async () => {
  const dir = makeFixture({
    "2026-07-31T01-55-30-577Z_s1.jsonl": [
      sessionHeader(),
      messageEntry({
        role: "assistant",
        model: "test-model",
        usage: assistantUsage(),
      }),
    ],
  });
  try {
    const out = await runCli(["--dir", dir]);
    const row = parseTable(out);

    // 9 列齐全
    for (const col of ["请求数", "输入", "输出", "缓存读", "缓存写", "推理", "总 token", "花费", "缓存率"]) {
      assert.ok(col in row, `表头应包含列 ${col}`);
    }

    // 数字正确：请求数 1 / 输入 100 / 输出 50 / 缓存读 200 / 缓存写 10 / 推理 20
    assert.equal(row["请求数"], "1");
    assert.equal(row["输入"], "100");
    assert.equal(row["输出"], "50");
    assert.equal(row["缓存读"], "200");
    assert.equal(row["缓存写"], "10");
    assert.equal(row["推理"], "20");

    // 总 token = 总输入+输出 100+200+50 = 350（不含缓存写）
    assert.equal(row["总 token"], "350");

    // 花费 = cost.total = 0.1
    assert.equal(row["花费"], "0.1");

    // 缓存率 = 200 / (100+200) = 66.67%（分母不含缓存写）
    assert.equal(row["缓存率"], "66.67%");
  } finally {
    removeFixture(dir);
  }
});
