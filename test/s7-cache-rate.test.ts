import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage, zeroUsage, parseTable } from "./helpers.ts";

test("S7 缓存率聚合：先求和分子分母再除；分母为 0 的请求记 0 不报错", async () => {
  const dir = makeFixture({
    "session.jsonl": [
      sessionHeader(),
      // 请求 A：cacheRead=200，分母 = 100+200 = 300（不含缓存写）
      messageEntry({ role: "assistant", model: "m", usage: assistantUsage({ input: 100, cacheRead: 200, cacheWrite: 10 }) }),
      // 请求 B：cacheRead=100，分母 = 400+100 = 500
      messageEntry({ role: "assistant", model: "m", usage: assistantUsage({ input: 400, cacheRead: 100, cacheWrite: 0 }) }),
      // 请求 C：全 0 usage（分母 0）→ 计入请求数但不贡献分子分母
      messageEntry({ role: "assistant", model: "m", usage: zeroUsage() }),
    ],
  });
  try {
    const out = await runCli(["--dir", dir]);
    const row = parseTable(out);

    // 聚合缓存率 = ΣcacheRead / Σ(input+cacheRead)
    //   = (200+100) / (100+400+200+100) = 300/800 = 37.50%
    //   （逐请求平均会得 (66.67%+20%)/2 = 43.33%，可区分）
    assert.equal(row["缓存率"], "37.50%", "聚合缓存率应为 300/800 = 37.50%");

    // 请求数 = 3（含全 0 失败消息）
    assert.equal(row["请求数"], "3", "请求数应为 3");
    // 总 token = 输入500 + 缓存读300 + 输出100 = 900（不含缓存写 10）
    assert.equal(row["总 token"], "900", "总 token 应为总输入+输出 900");
    // 输入 = 500
    assert.equal(row["输入"], "500", "输入应为 500");
  } finally {
    removeFixture(dir);
  }
});
