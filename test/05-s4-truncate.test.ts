import { test } from "node:test";
import assert from "node:assert/strict";
import { truncateSync, appendFileSync } from "node:fs";
import { join } from "node:path";
import {
  makeFixture,
  removeFixture,
  sessionHeader,
  messageEntry,
  assistantUsage,
} from "./helpers.ts";
import { IncrementalReader, applyIncrements } from "../src/watch.ts";
import { emptyTotals } from "../src/aggregate.ts";

test("S4 文件截断重同步：文件被 truncate → 重读新内容，不丢不重", async () => {
  const dir = makeFixture({
    "sess.jsonl": [
      sessionHeader({ id: "s1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }),
    ],
  });
  const file = join(dir, "sess.jsonl");
  const reader = new IncrementalReader(dir);
  const totals = emptyTotals();
  try {
    // 初始：2 条
    await applyIncrements(reader, totals);
    assert.equal(totals.requests, 2);
    assert.equal(totals.input, 300);

    // 模拟截断 + 重写（同 inode）：truncate 到 0 后写入新 header + 新消息
    truncateSync(file, 0);
    appendFileSync(
      file,
      JSON.stringify(sessionHeader({ id: "s1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" })) + "\n" +
        JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 300 }) })) + "\n",
    );

    // 截断检测（size < offset）→ 重读：净 = 300
    await applyIncrements(reader, totals);
    assert.equal(totals.requests, 1, "截断重写后净请求数 = 新内容 1 条");
    assert.equal(totals.input, 300, "截断重写后净 input = 300");

    // 后续正常增量
    appendFileSync(file, JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 400 }) })) + "\n");
    await applyIncrements(reader, totals);
    assert.equal(totals.requests, 2);
    assert.equal(totals.input, 700);
  } finally {
    removeFixture(dir);
  }
});
