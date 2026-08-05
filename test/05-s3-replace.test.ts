import { test } from "node:test";
import assert from "node:assert/strict";
import { appendFileSync, writeFileSync, rmSync } from "node:fs";
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

test("S3 文件替换重同步：文件被替换（新 inode）→ 整体重读，不丢不重", async () => {
  const dir = makeFixture({
    "sess.jsonl": [
      sessionHeader({ id: "s1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    ],
  });
  const file = join(dir, "sess.jsonl");
  const reader = new IncrementalReader(dir);
  const totals = emptyTotals();
  try {
    // 初始：1 条（input 100）
    await applyIncrements(reader, totals);
    assert.equal(totals.requests, 1);
    assert.equal(totals.input, 100);

    // 模拟文件替换：删除原文件，写入全新文件（新 inode，内容不同）
    rmSync(file);
    writeFileSync(
      file,
      JSON.stringify(sessionHeader({ id: "s1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" })) + "\n" +
        JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 500 }) })) + "\n" +
        JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 600 }) })) + "\n",
    );

    // 重同步：旧贡献被扣减，新内容累加 → 净 = 500+600
    await applyIncrements(reader, totals);
    assert.equal(totals.requests, 2, "替换后净请求数 = 新文件 2 条");
    assert.equal(totals.input, 1100, "替换后净 input = 500+600 = 1100");

    // 后续正常增量
    appendFileSync(file, JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 700 }) })) + "\n");
    await applyIncrements(reader, totals);
    assert.equal(totals.requests, 3, "替换后正常追加");
    assert.equal(totals.input, 1800);
  } finally {
    removeFixture(dir);
  }
});
