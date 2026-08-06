import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage, parseTable } from "./helpers.ts";

test("S6 网关口径：总 token 按总输入+输出计算（不含缓存写，不信任 totalTokens 字段）", async () => {
  const dir = makeFixture({
    "session.jsonl": [
      sessionHeader(),
      // 总输入+输出 = 100+200+50 = 350，缓存写 10 不计入；totalTokens 字段谎报 9999
      messageEntry({
        role: "assistant",
        model: "m",
        usage: assistantUsage({ totalTokens: 9999 }),
      }),
    ],
  });
  try {
    const out = await runCli(["--dir", dir]);
    const row = parseTable(out);

    // 总 token 按网关口径 = 350（不含缓存写 10）
    assert.equal(row["总 token"], "350", "总 token 应为总输入+输出 350");
  } finally {
    removeFixture(dir);
  }
});

test("S6 网关口径：缓存率分母不含缓存写", async () => {
  const dir = makeFixture({
    "session.jsonl": [
      sessionHeader(),
      // 缓存率 = 200/(100+200) = 66.67%，分母不含缓存写 10
      messageEntry({ role: "assistant", model: "m", usage: assistantUsage() }),
    ],
  });
  try {
    const out = await runCli(["--dir", dir]);
    const row = parseTable(out);

    assert.equal(row["缓存率"], "66.67%", "缓存率应为 66.67%（分母不含缓存写）");
  } finally {
    removeFixture(dir);
  }
});

test("S6 网关口径回归：cacheWrite=0 时与旧组件和口径数值一致", async () => {
  const dir = makeFixture({
    "session.jsonl": [
      sessionHeader(),
      // cacheWrite=0：总输入+输出 = 组件和 = 350；缓存率分母 input+cacheRead = 旧分母 → 66.67%
      messageEntry({ role: "assistant", model: "m", usage: assistantUsage({ cacheWrite: 0 }) }),
    ],
  });
  try {
    const out = await runCli(["--dir", dir]);
    const row = parseTable(out);

    assert.equal(row["总 token"], "350", "cacheWrite=0 时总 token 与旧组件和一致");
    assert.equal(row["缓存率"], "66.67%", "cacheWrite=0 时缓存率与旧分母一致");
  } finally {
    removeFixture(dir);
  }
});
