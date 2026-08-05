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
  parseTable,
} from "./helpers.ts";

test("S1 会话级分组：每会话一行，指标=会话内求和，总和与 totals 一致", async () => {
  const dir = makeFixture({
    // 会话 A：2 条消息（input 100 + 300）
    "2026-07-31T01-00-00-000Z_a.jsonl": [
      sessionHeader({ id: "sess-a", timestamp: "2026-07-31T01:00:00.000Z", cwd: "/proj/a" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 300 }) }),
    ],
    // 会话 B：1 条消息（input 500）
    "2026-07-31T02-00-00-000Z_b.jsonl": [
      sessionHeader({ id: "sess-b", timestamp: "2026-07-31T02:00:00.000Z", cwd: "/proj/b" }),
      messageEntry({ role: "assistant", model: "m2", usage: assistantUsage({ input: 500 }) }),
    ],
  });
  try {
    const out = await runCli(["sessions", "--dir", dir]);
    const rows = parseTableRows(out);

    // 两行，每行含会话 ID
    assert.equal(rows.length, 2, "应有 2 个会话行");
    const a = rows.find((r) => r["会话ID"] === "sess-a");
    const b = rows.find((r) => r["会话ID"] === "sess-b");
    assert.ok(a, "会话 A 行存在");
    assert.ok(b, "会话 B 行存在");

    // 会话 A：input = 100+300 = 400，请求数 = 2
    assert.equal(a!["请求数"], "2");
    assert.equal(a!["输入"], "400");
    assert.equal(a!["输出"], "100"); // 50×2
    // 会话 B：input = 500，请求数 = 1
    assert.equal(b!["请求数"], "1");
    assert.equal(b!["输入"], "500");

    // 与 totals 窗口一致：总 = 各会话之和
    const totals = parseTable(await runCli(["--dir", dir]));
    assert.equal(totals["请求数"], "3", "总请求数 = 2+1");
    assert.equal(totals["输入"], "900", "总输入 = 400+500");
  } finally {
    removeFixture(dir);
  }
});
