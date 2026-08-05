import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage, parseTable } from "./helpers.ts";

test("S3 非标准文件名纳入：无尾 UUID 的合法会话照常统计", async () => {
  const dir = makeFixture({
    // 标准命名（timestamp_UUID.jsonl）
    "2026-07-31T01-55-30-577Z_019fb5e2-3c91-76bd-b12c-c8d2ab31c532.jsonl": [
      sessionHeader(),
      messageEntry({ role: "assistant", model: "m", usage: assistantUsage({ input: 100 }) }),
    ],
    // 非标准命名（用户导出/扩展产物）但首行是合法 session header
    "my-backup-export.jsonl": [
      sessionHeader({ id: "custom-id-1", cwd: "/home/shial/other" }),
      messageEntry({ role: "assistant", model: "m", usage: assistantUsage({ input: 300 }) }),
    ],
  });
  try {
    const out = await runCli(["--dir", dir]);
    const row = parseTable(out);

    // 两个合法会话都纳入：请求数 2，输入 100+300=400
    assert.equal(row["请求数"], "2", "请求数应为 2");
    assert.equal(row["输入"], "400", "输入应为 100+300=400");
  } finally {
    removeFixture(dir);
  }
});
