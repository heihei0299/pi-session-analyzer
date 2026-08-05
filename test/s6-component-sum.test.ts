import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage, parseTable } from "./helpers.ts";

test("S6 组件和计算：totalTokens 字段与组件和不一致时按组件和", async () => {
  const dir = makeFixture({
    "session.jsonl": [
      sessionHeader(),
      // 组件和 = 100+50+200+10 = 360，但 totalTokens 字段谎报 9999
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

    // 总 token 按组件和 = 360
    assert.equal(row["总 token"], "360", "总 token 应为组件和 360");
  } finally {
    removeFixture(dir);
  }
});
