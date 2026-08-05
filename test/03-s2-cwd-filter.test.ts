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

test("S2 --cwd 过滤：仅 header cwd 匹配的会话计入，三窗口一致", async () => {
  const dir = makeFixture({
    // 目录名是有损编码（-- 包裹、/→-），但 header cwd 是权威
    "--proj-a--.jsonl": [
      sessionHeader({ id: "sess-a", timestamp: "2026-07-31T01:00:00.000Z", cwd: "/proj/a" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    ],
    "2026-07-31T02-00-00-000Z_b.jsonl": [
      sessionHeader({ id: "sess-b", timestamp: "2026-07-31T02:00:00.000Z", cwd: "/proj/b" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 500 }) }),
    ],
  });
  try {
    // totals：--cwd /proj/a → 仅 sess-a 计入
    const totals = parseTable(await runCli(["totals", "--dir", dir, "--cwd", "/proj/a"]));
    assert.equal(totals["请求数"], "1", "totals 过滤后请求数应为 1");
    assert.equal(totals["输入"], "100", "totals 过滤后输入应为 100");

    // sessions：仅 sess-a 一行
    const sessions = parseTableRows(await runCli(["sessions", "--dir", dir, "--cwd", "/proj/a"]));
    assert.equal(sessions.length, 1, "sessions 过滤后应为 1 行");
    assert.equal(sessions[0]["会话ID"], "sess-a");

    // requests：仅 sess-a 的消息
    const requests = parseTableRows(await runCli(["requests", "--dir", dir, "--cwd", "/proj/a"]));
    assert.equal(requests.length, 1, "requests 过滤后应为 1 行");
    assert.equal(requests[0]["会话ID"], "sess-a");

    // cwd 过滤与有损目录名无关：--cwd 传目录名编码形式不匹配任何会话
    const none = parseTable(await runCli(["totals", "--dir", dir, "--cwd", "--proj-a--"]));
    assert.equal(none["请求数"], "0", "目录名编码不是 cwd 键，不应匹配");
  } finally {
    removeFixture(dir);
  }
});
