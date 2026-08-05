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

test("S6 model_change 行不干扰请求级模型归属（真实数据形态）", async () => {
  const dir = makeFixture({
    "mixed.jsonl": [
      sessionHeader({ id: "sess", timestamp: "2026-07-31T01:00:00.000Z", cwd: "/proj/a" }),
      // 真实数据中的 model_change 事件行（无 usage，不得计入任何模型）
      { type: "model_change", id: "mc1", parentId: null, timestamp: "2026-07-31T01:01:00.000Z", provider: "cpa", modelId: "m1" },
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
      { type: "model_change", id: "mc2", parentId: null, timestamp: "2026-07-31T01:02:00.000Z", provider: "cpa", modelId: "m2" },
      messageEntry({ role: "assistant", model: "m2", usage: assistantUsage({ input: 300 }) }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }),
    ],
  });
  try {
    // 模型分组：m1×2 (100+200)、m2×1 (300)；model_change 不影响归属
    const rows = parseTableRows(await runCli(["totals", "--dir", dir, "--by", "model"]));
    assert.equal(rows.length, 2, "仅 m1/m2 两组（model_change 无 usage 不产生分组）");
    const m1 = rows.find((r) => r["模型"] === "m1");
    const m2 = rows.find((r) => r["模型"] === "m2");
    assert.ok(m1 && m2);
    assert.equal(m1!["请求数"], "2");
    assert.equal(m1!["输入"], "300");
    assert.equal(m2!["请求数"], "1");
    assert.equal(m2!["输入"], "300");

    // --model 过滤同样不受 model_change 影响
    const filtered = parseTableRows(await runCli(["totals", "--dir", dir, "--by", "model", "--model", "m1"]));
    assert.equal(filtered.length, 1);
    assert.equal(filtered[0]["模型"], "m1");
    assert.equal(filtered[0]["输入"], "300");
  } finally {
    removeFixture(dir);
  }
});
