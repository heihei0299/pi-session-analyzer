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

test("S3 --period week 汇总：ISO 周归属（周一起始），跨周会话分属各自周", async () => {
  // 2026-08-01 是周六 → 属 ISO 周 2026-07-27（周一）~ 08-02（周日）
  // 2026-08-03 是周一 → 属 ISO 周 2026-08-03 起
  const dir = makeFixture({
    // 周六（2026-07-27 周）：input 100
    "2026-08-01T10-00-00-000Z_a.jsonl": [
      sessionHeader({ id: "s-a", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    ],
    // 周日（2026-07-27 周）：input 200
    "2026-08-02T10-00-00-000Z_b.jsonl": [
      sessionHeader({ id: "s-b", timestamp: "2026-08-02T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }),
    ],
    // 周一（2026-08-03 周）：input 400
    "2026-08-03T10-00-00-000Z_c.jsonl": [
      sessionHeader({ id: "s-c", timestamp: "2026-08-03T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 400 }) }),
    ],
  });
  try {
    const rows = parseTableRows(await runCli(["totals", "--dir", dir, "--period", "week"]));
    assert.equal(rows.length, 2, "两周各一行");

    const w1 = rows.find((r) => r["周起始"] === "2026-07-27");
    const w2 = rows.find((r) => r["周起始"] === "2026-08-03");
    assert.ok(w1 && w2, "周起始键为周一日期");

    // 2026-07-27 周：8/1 + 8/2 → input 100+200=300
    assert.equal(w1!["请求数"], "2");
    assert.equal(w1!["输入"], "300");
    // 2026-08-03 周：8/3 → input 400
    assert.equal(w2!["请求数"], "1");
    assert.equal(w2!["输入"], "400");

    // 各周之和 = 总窗口
    const sum = (k: string) => rows.reduce((acc, r) => acc + Number(r[k]), 0);
    assert.equal(sum("请求数"), 3);
    assert.equal(sum("输入"), 700);
  } finally {
    removeFixture(dir);
  }
});
