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
import { parseArgs } from "../src/cli.ts";
import { IncrementalReader, applyIncrements } from "../src/watch.ts";
import { emptyTotals } from "../src/aggregate.ts";

test("S5b --watch 参数解析：--watch 与 --interval 生效，非法值报错", () => {
  const args = parseArgs(["--dir", "/tmp/x", "--watch", "--interval", "500"]);
  assert.equal(args.watch, true);
  assert.equal(args.interval, 500);

  assert.throws(() => parseArgs(["--interval", "0"]), /无效间隔/);
  assert.throws(() => parseArgs(["--interval", "abc"]), /无效间隔/);
  assert.equal(parseArgs(["--dir", "/tmp/x"]).watch, false, "默认非 watch");
});

test("S5c watch 单步循环：多轮 applyIncrements 累加正确（模拟持续追加）", async () => {
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
    // 轮 1：初始
    await applyIncrements(reader, totals);
    assert.equal(totals.requests, 1);
    assert.equal(totals.input, 100);

    // 轮 2：追加 1 条
    appendFileSync(file, JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 250 }) })) + "\n");
    await applyIncrements(reader, totals);
    assert.equal(totals.requests, 2);
    assert.equal(totals.input, 350);

    // 轮 3：追加 2 条
    appendFileSync(
      file,
      JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 60 }) })) + "\n" +
        JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 40 }) })) + "\n",
    );
    await applyIncrements(reader, totals);
    assert.equal(totals.requests, 4);
    assert.equal(totals.input, 450);
  } finally {
    removeFixture(dir);
  }
});
