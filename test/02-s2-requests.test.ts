import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import {
  makeFixture,
  removeFixture,
  sessionHeader,
  messageEntry,
  assistantUsage,
  zeroUsage,
  parseTableRows,
} from "./helpers.ts";

test("S2 单请求级窗口：逐 assistant 消息一行，含全 0 失败消息", async () => {
  const dir = makeFixture({
    "2026-07-31T01-00-00-000Z_a.jsonl": [
      sessionHeader({ id: "sess-a", timestamp: "2026-07-31T01:00:00.000Z", cwd: "/proj/a" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 300 }) }),
      // 全 0 失败消息：计入请求数（本行 requests=1），token 为 0
      messageEntry({ role: "assistant", model: "m1", stopReason: "aborted", usage: zeroUsage() }),
    ],
  });
  try {
    const out = await runCli(["requests", "--dir", dir]);
    const rows = parseTableRows(out);

    // 3 条消息 → 3 行（含失败消息）
    assert.equal(rows.length, 3, "应有 3 个请求行（含全 0 失败消息）");

    // 每行请求数恒 1
    for (const r of rows) {
      assert.equal(r["请求数"], "1", "每行请求数应为 1");
      assert.equal(r["会话ID"], "sess-a");
      assert.equal(r["模型"], "m1");
    }

    // 各行输入分别为 100 / 300 / 0
    const inputs = rows.map((r) => r["输入"]);
    assert.deepEqual(inputs.sort(), ["0", "100", "300"]);

    // 失败消息行：token 全 0
    const failed = rows.find((r) => r["输入"] === "0");
    assert.equal(failed!["输出"], "0");
    assert.equal(failed!["缓存读"], "0");
    assert.equal(failed!["总 token"], "0");
  } finally {
    removeFixture(dir);
  }
});
