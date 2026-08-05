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

test("S3 --by model 分组：同会话混合多模型按请求级归属，每模型一行", async () => {
  const dir = makeFixture({
    // 同一会话混合 3 个模型：m1×2、m2×1、m3×1（input 覆盖区分）
    "mixed.jsonl": [
      sessionHeader({ id: "mixed-sess", timestamp: "2026-07-31T01:00:00.000Z", cwd: "/proj/a" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
      messageEntry({ role: "assistant", model: "m2", usage: assistantUsage({ input: 300 }) }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }),
      messageEntry({ role: "assistant", model: "m3", usage: assistantUsage({ input: 400 }) }),
    ],
  });
  try {
    const rows = parseTableRows(await runCli(["totals", "--dir", dir, "--by", "model"]));
    assert.equal(rows.length, 3, "3 个模型各一行");

    const m1 = rows.find((r) => r["模型"] === "m1");
    const m2 = rows.find((r) => r["模型"] === "m2");
    const m3 = rows.find((r) => r["模型"] === "m3");
    assert.ok(m1 && m2 && m3);

    // m1：2 条请求，input 100+200=300
    assert.equal(m1!["请求数"], "2");
    assert.equal(m1!["输入"], "300");
    // m2：1 条，input 300
    assert.equal(m2!["请求数"], "1");
    assert.equal(m2!["输入"], "300");
    // m3：1 条，input 400
    assert.equal(m3!["请求数"], "1");
    assert.equal(m3!["输入"], "400");

    // 各模型行之和 = 总窗口（分组不丢数据）
    const sum = (k: string) => rows.reduce((acc, r) => acc + Number(r[k]), 0);
    assert.equal(sum("请求数"), 4, "分组请求数之和应为 4");
    assert.equal(sum("输入"), 1000, "分组输入之和应为 100+200+300+400=1000");

    // JSON 输出：rows 含 model 键与指标
    const json = JSON.parse(await runCli(["totals", "--dir", dir, "--by", "model", "--format", "json"]));
    assert.equal(json.window, "totals");
    assert.equal(json.by, "model");
    assert.equal(json.rows.length, 3);
    const jm1 = json.rows.find((r: Record<string, unknown>) => r.model === "m1");
    assert.equal(jm1.requests, 2);
    assert.equal(jm1.input, 300);
  } finally {
    removeFixture(dir);
  }
});
