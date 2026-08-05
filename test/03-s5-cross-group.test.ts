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

test("S5 交叉分组 + 过滤：--by model,cwd 汇总，--model/--cwd 过滤交集", async () => {
  const dir = makeFixture({
    // cwd /proj/a：m1×1 (input 100)、m2×1 (input 300)
    "2026-07-31T01-00-00-000Z_a.jsonl": [
      sessionHeader({ id: "sess-a", timestamp: "2026-07-31T01:00:00.000Z", cwd: "/proj/a" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
      messageEntry({ role: "assistant", model: "m2", usage: assistantUsage({ input: 300 }) }),
    ],
    // cwd /proj/b：m1×1 (input 200)、m2×1 (input 400)
    "2026-07-31T02-00-00-000Z_b.jsonl": [
      sessionHeader({ id: "sess-b", timestamp: "2026-07-31T02:00:00.000Z", cwd: "/proj/b" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }),
      messageEntry({ role: "assistant", model: "m2", usage: assistantUsage({ input: 400 }) }),
    ],
  });
  try {
    // 交叉分组：4 个 model×cwd 组合
    const rows = parseTableRows(await runCli(["totals", "--dir", dir, "--by", "model,cwd"]));
    assert.equal(rows.length, 4, "model×cwd 四组合各一行");
    const m1a = rows.find((r) => r["模型"] === "m1" && r["cwd"] === "/proj/a");
    assert.ok(m1a);
    assert.equal(m1a!["请求数"], "1");
    assert.equal(m1a!["输入"], "100");
    const m2b = rows.find((r) => r["模型"] === "m2" && r["cwd"] === "/proj/b");
    assert.ok(m2b);
    assert.equal(m2b!["输入"], "400");

    // 过滤交集：--by model,cwd + --model m1 → 仅 (m1,/proj/a) 与 (m1,/proj/b)
    const filtered = parseTableRows(await runCli(["totals", "--dir", dir, "--by", "model,cwd", "--model", "m1"]));
    assert.equal(filtered.length, 2, "过滤后仅 m1 的两个组合");
    for (const r of filtered) {
      assert.equal(r["模型"], "m1");
    }

    // 过滤交集：--by model,cwd + --cwd /proj/a → 仅 (m1,/proj/a) 与 (m2,/proj/a)
    const filteredCwd = parseTableRows(await runCli(["totals", "--dir", dir, "--by", "model,cwd", "--cwd", "/proj/a"]));
    assert.equal(filteredCwd.length, 2, "过滤后仅 /proj/a 的两个组合");
    for (const r of filteredCwd) {
      assert.equal(r["cwd"], "/proj/a");
    }

    // 双重过滤：--model m1 --cwd /proj/a → 仅 (m1,/proj/a)
    const both = parseTableRows(await runCli(["totals", "--dir", dir, "--by", "model,cwd", "--model", "m1", "--cwd", "/proj/a"]));
    assert.equal(both.length, 1, "双重过滤后仅一个组合");
    assert.equal(both[0]["模型"], "m1");
    assert.equal(both[0]["cwd"], "/proj/a");
    assert.equal(both[0]["输入"], "100");

    // 分组键总和 = 未过滤总窗口（无分组）
    const sumInput = rows.reduce((acc, r) => acc + Number(r["输入"]), 0);
    assert.equal(sumInput, 1000, "交叉分组输入之和 = 100+300+200+400 = 1000");

    // CSV 输出
    const csv = await runCli(["totals", "--dir", dir, "--by", "model,cwd", "--format", "csv"]);
    assert.ok(csv.startsWith("model,cwd,requests,input"), "CSV 表头含 model,cwd 与指标列");
    assert.equal(csv.trim().split("\n").length, 5, "CSV 表头 + 4 行");
  } finally {
    removeFixture(dir);
  }
});
