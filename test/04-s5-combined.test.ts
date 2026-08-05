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

test("S5 时间×维度组合：--period + --since/--until + --model/--cwd 共同生效", async () => {
  const dir = makeFixture({
    // 8/1 cwd=/proj/a：m1 (input 100)、m2 (input 300)
    "2026-08-01T10-00-00-000Z_a.jsonl": [
      sessionHeader({ id: "s-a", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj/a" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
      messageEntry({ role: "assistant", model: "m2", usage: assistantUsage({ input: 300 }) }),
    ],
    // 8/2 cwd=/proj/a：m1 (input 200)
    "2026-08-02T10-00-00-000Z_b.jsonl": [
      sessionHeader({ id: "s-b", timestamp: "2026-08-02T10:00:00.000Z", cwd: "/proj/a" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }),
    ],
    // 8/3 cwd=/proj/b：m1 (input 500)
    "2026-08-03T10-00-00-000Z_c.jsonl": [
      sessionHeader({ id: "s-c", timestamp: "2026-08-03T10:00:00.000Z", cwd: "/proj/b" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 500 }) }),
    ],
  });
  try {
    // 组合：--period day + --model m1 + --cwd /proj/a + --since 2026-08-01
    // → 仅 /proj/a 中 m1 的会话：8/1 (100)、8/2 (200)
    const rows = parseTableRows(
      await runCli(["totals", "--dir", dir, "--period", "day", "--model", "m1", "--cwd", "/proj/a", "--since", "2026-08-01"]),
    );
    assert.equal(rows.length, 2, "两天的 m1×/proj/a 会话");
    const d1 = rows.find((r) => r["日期"] === "2026-08-01");
    const d2 = rows.find((r) => r["日期"] === "2026-08-02");
    assert.ok(d1 && d2);
    assert.equal(d1!["输入"], "100");
    assert.equal(d2!["输入"], "200");

    // 组合与 --until：--until 2026-08-01 → 仅 8/1
    const untilRows = parseTableRows(
      await runCli(["totals", "--dir", dir, "--period", "day", "--model", "m1", "--cwd", "/proj/a", "--since", "2026-08-01", "--until", "2026-08-01"]),
    );
    assert.equal(untilRows.length, 1, "--until 8/1 后仅一天");
    assert.equal(untilRows[0]["日期"], "2026-08-01");
    assert.equal(untilRows[0]["输入"], "100");

    // --by model 与 --since 组合（维度分组 × 时间筛选）
    const byRows = parseTableRows(
      await runCli(["totals", "--dir", dir, "--by", "model", "--since", "2026-08-02"]),
    );
    // 8/2 起：m1 (8/2 200 + 8/3 500 = 700)、m2 无（8/1 被排除）→ m2 不出现
    assert.equal(byRows.length, 1, "8/2 起仅 m1 有请求");
    assert.equal(byRows[0]["模型"], "m1");
    assert.equal(byRows[0]["输入"], "700");

    // sessions 窗口 + --since/--until（时间筛选对全部窗口生效）
    const sessions = await runCli(["sessions", "--dir", dir, "--since", "2026-08-02", "--until", "2026-08-02"]);
    assert.equal(sessions.trim().split("\n").length - 1, 1, "仅 8/2 会话一行");
  } finally {
    removeFixture(dir);
  }
});
