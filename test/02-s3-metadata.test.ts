import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import {
  makeFixture,
  removeFixture,
  sessionHeader,
  messageEntry,
  assistantUsage,
  parseTableRows,
} from "./helpers.ts";

test("S3 会话行元数据：timestamp/cwd/model 来自 header 与消息；混合模型会话标 mixed", async () => {
  const dir = makeFixture({
    // 唯一模型会话：model 列 = 该模型名
    "2026-07-31T01-00-00-000Z_a.jsonl": [
      sessionHeader({ id: "sess-a", timestamp: "2026-07-31T01:00:00.000Z", cwd: "/proj/a" }),
      messageEntry({ role: "assistant", model: "deepseek-v4", usage: assistantUsage() }),
      messageEntry({ role: "assistant", model: "deepseek-v4", usage: assistantUsage() }),
    ],
    // 混合模型会话：model 列 = mixed
    "2026-07-31T02-00-00-000Z_b.jsonl": [
      sessionHeader({ id: "sess-b", timestamp: "2026-07-31T02:00:00.000Z", cwd: "/proj/b" }),
      messageEntry({ role: "assistant", model: "claude-sonnet", usage: assistantUsage() }),
      messageEntry({ role: "assistant", model: "gpt-5", usage: assistantUsage() }),
    ],
  });
  try {
    const out = await runCli(["sessions", "--dir", dir]);
    const rows = parseTableRows(out);

    const a = rows.find((r) => r["会话ID"] === "sess-a");
    const b = rows.find((r) => r["会话ID"] === "sess-b");
    assert.ok(a, "会话 A 行存在");
    assert.ok(b, "会话 B 行存在");

    // A：时间戳/cwd/model 均来自 header/消息
    assert.equal(a!["时间戳"], "2026-07-31T01:00:00.000Z");
    assert.equal(a!["cwd"], "/proj/a");
    assert.equal(a!["模型"], "deepseek-v4");

    // B：混合模型 → mixed
    assert.equal(b!["时间戳"], "2026-07-31T02:00:00.000Z");
    assert.equal(b!["cwd"], "/proj/b");
    assert.equal(b!["模型"], "mixed");
  } finally {
    removeFixture(dir);
  }
});
