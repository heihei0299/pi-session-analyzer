import { test } from "node:test";
import assert from "node:assert/strict";
import { appendFileSync } from "node:fs";
import { join } from "node:path";
import {
  makeFixture,
  removeFixture,
  sessionHeader,
  messageEntry,
  assistantUsage,
} from "./helpers.ts";
import { IncrementalReader } from "../src/watch.ts";

test("S1 增量读取：追加新 assistant 行 → 增量出现，不重复已读行", async () => {
  const dir = makeFixture({
    "sess.jsonl": [
      sessionHeader({ id: "s1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    ],
  });
  const file = join(dir, "sess.jsonl");
  const reader = new IncrementalReader(dir);
  try {
    // 初始扫描：1 条 assistant 消息（input 100）
    const first = await reader.readIncrements();
    assert.equal(first.length, 1, "初始扫描应解析出 1 条");
    assert.equal(first[0].input, 100);

    // 追加 2 条新 assistant 消息
    appendFileSync(
      file,
      JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) })) + "\n" +
        JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 300 }) })) + "\n",
    );

    // 再次增量：只返回新增的 2 条，不重复初始 1 条
    const second = await reader.readIncrements();
    assert.equal(second.length, 2, "增量应返回 2 条新消息");
    assert.equal(second[0].input, 200);
    assert.equal(second[1].input, 300);

    // 无新行：返回空
    const third = await reader.readIncrements();
    assert.equal(third.length, 0, "无新行应返回空");
  } finally {
    removeFixture(dir);
  }
});
