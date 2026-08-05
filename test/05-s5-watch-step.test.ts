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
import { IncrementalReader, applyIncrements } from "../src/watch.ts";
import { runWatch } from "../src/cli.ts";
import { emptyTotals, type Totals } from "../src/aggregate.ts";

test("S5 --watch 单步：增量累加到 totals，数字正确", async () => {
  const dir = makeFixture({
    "sess.jsonl": [
      sessionHeader({ id: "s1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    ],
  });
  const file = join(dir, "sess.jsonl");
  const reader = new IncrementalReader(dir);
  const totals: Totals = emptyTotals();
  try {
    // 第一步：初始扫描 → 累加 1 条
    await applyIncrements(reader, totals);
    assert.equal(totals.requests, 1);
    assert.equal(totals.input, 100);

    // 追加 2 条
    appendFileSync(
      file,
      JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) })) + "\n" +
        JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 300 }) })) + "\n",
    );

    // 第二步：累加新 2 条
    const changed = await applyIncrements(reader, totals);
    assert.equal(changed, true, "有增量时应返回 true");
    assert.equal(totals.requests, 3);
    assert.equal(totals.input, 600);

    // 无增量：返回 false，totals 不变
    const unchanged = await applyIncrements(reader, totals);
    assert.equal(unchanged, false, "无增量时应返回 false");
    assert.equal(totals.requests, 3);
    assert.equal(totals.input, 600);
  } finally {
    removeFixture(dir);
  }
});

test("S5d runWatch 本体：多轮迭代驱动 + 刷新回调计数", async () => {
  const dir = makeFixture({
    "sess.jsonl": [
      sessionHeader({ id: "s1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    ],
  });
  const file = join(dir, "sess.jsonl");
  const reader = new IncrementalReader(dir);
  const totals: Totals = emptyTotals();
  let refreshes = 0;
  const seenInputs: number[] = [];
  try {
    // 2 轮迭代：轮1 初始（changed=false 但 i=0 也刷新），轮2 无增量（不刷新）
    const count = await runWatch(reader, totals, (t) => {
      refreshes++;
      seenInputs.push(t.input);
    }, 1, 2);
    assert.equal(count, 1, "仅首轮刷新（无增量不重复刷新）");
    assert.equal(refreshes, 1);
    assert.deepEqual(seenInputs, [100]);

    // 追加后跑 2 轮：轮1 增量 → 刷新，轮2 无增量 → 不刷新
    appendFileSync(file, JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 250 }) })) + "\n");
    const count2 = await runWatch(reader, totals, (t) => {
      refreshes++;
      seenInputs.push(t.input);
    }, 1, 2);
    assert.equal(count2, 1, "增量出现时刷新一次");
    assert.equal(refreshes, 2);
    assert.deepEqual(seenInputs, [100, 350]);
    assert.equal(totals.requests, 2);
  } finally {
    removeFixture(dir);
  }
});
