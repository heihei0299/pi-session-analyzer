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

test("S2 非 assistant 行过滤：user/toolResult/model_change 等不触发统计变化", async () => {
  const dir = makeFixture({
    "sess.jsonl": [
      sessionHeader({ id: "s1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    ],
  });
  const file = join(dir, "sess.jsonl");
  const reader = new IncrementalReader(dir);
  try {
    // 初始：1 条
    const first = await reader.readIncrements();
    assert.equal(first.length, 1);

    // 追加各种非 assistant 行（含带 usage 的 toolResult / user / model_change / 坏 JSON）
    appendFileSync(
      file,
      JSON.stringify(messageEntry({ role: "user", content: [{ type: "text", text: "hi" }], usage: assistantUsage({ input: 999 }) })) + "\n" +
        JSON.stringify(messageEntry({ role: "toolResult", usage: assistantUsage({ input: 888 }) })) + "\n" +
        JSON.stringify({ type: "model_change", id: "mc1", parentId: null, timestamp: "2026-08-01T10:01:00.000Z", provider: "cpa", modelId: "m2" }) + "\n" +
        "{ this is not valid json\n",
    );

    // 增量应为空：非 assistant 行全部过滤
    const second = await reader.readIncrements();
    assert.equal(second.length, 0, "非 assistant 行不触发统计变化");

    // 追加一条合法 assistant 行：增量只含它
    appendFileSync(file, JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) })) + "\n");
    const third = await reader.readIncrements();
    assert.equal(third.length, 1, "只有合法 assistant 行被计入");
    assert.equal(third[0].input, 200);
  } finally {
    removeFixture(dir);
  }
});
