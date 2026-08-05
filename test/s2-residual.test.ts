import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage, parseTable } from "./helpers.ts";

test("S2 残留文件跳过：type:message / type:custom 首行文件不纳入统计", async () => {
  const dir = makeFixture({
    // 合法会话：1 条 assistant usage
    "valid.jsonl": [
      sessionHeader(),
      messageEntry({ role: "assistant", model: "m", usage: assistantUsage() }),
    ],
    // 残留：首行 type=message（单条记录导出）
    "residual-message.jsonl": [
      messageEntry({ role: "assistant", model: "m", usage: assistantUsage({ input: 999, cacheRead: 999 }) }),
    ],
    // 残留：首行 type=custom
    "residual-custom.jsonl": [
      { type: "custom", customType: "foo", data: { usage: assistantUsage({ input: 777, cacheRead: 777 }) }, id: "c1", parentId: null, timestamp: "2026-07-31T01:00:00.000Z" },
    ],
  });
  try {
    const out = await runCli(["--dir", dir]);
    const row = parseTable(out);

    // 只有合法会话的 1 条消息计入
    assert.equal(row["请求数"], "1", "请求数应为 1");
    assert.equal(row["输入"], "100", "输入应为 100（不含残留的 999/777）");
    assert.equal(row["缓存读"], "200", "缓存读应为 200（不含残留数据）");
  } finally {
    removeFixture(dir);
  }
});
