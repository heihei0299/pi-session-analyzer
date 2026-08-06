import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import {
  makeFixture,
  removeFixture,
  sessionHeader,
  messageEntry,
  assistantUsage,
  parseTable,
  parseTableRows,
} from "./helpers.ts";

test("S1 --model 过滤：三窗口仅指定模型计入", async () => {
  const dir = makeFixture({
    // 同一会话混合两个模型（请求级归属）
    "mixed.jsonl": [
      sessionHeader({ id: "mixed-sess", timestamp: "2026-07-31T01:00:00.000Z", cwd: "/proj/a" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
      messageEntry({ role: "assistant", model: "m2", usage: assistantUsage({ input: 300 }) }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }),
    ],
  });
  try {
    // totals 窗口：--model m1 → 仅 m1 的两条（input 100+200=300），请求数 2
    const totals = parseTable(await runCli(["totals", "--dir", dir, "--model", "m1"]));
    assert.equal(totals["请求数"], "2", "totals 过滤后请求数应为 2");
    assert.equal(totals["输入"], "300", "totals 过滤后输入应为 300");
    assert.equal(totals["总 token"], "800", "总 token 应为 m1 两条消息总输入+输出：300+400+100 = 800");

    // sessions 窗口：--model m1 → 会话行仅含 m1 数据
    const sessions = parseTableRows(await runCli(["sessions", "--dir", dir, "--model", "m1"]));
    assert.equal(sessions.length, 1);
    assert.equal(sessions[0]["请求数"], "2", "sessions 过滤后请求数应为 2");

    // requests 窗口：--model m1 → 仅 2 行
    const requests = parseTableRows(await runCli(["requests", "--dir", dir, "--model", "m1"]));
    assert.equal(requests.length, 2, "requests 过滤后应为 2 行");
    for (const r of requests) {
      assert.equal(r["模型"], "m1");
    }
  } finally {
    removeFixture(dir);
  }
});
