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

test("S4 --by cwd 分组：按 header cwd 归属，目录名有损编码不干扰，规范化生效", async () => {
  const dir = makeFixture({
    // 目录名有损编码 --home-shial-project-a--（真实数据形态），header cwd 权威
    "--home-shial-project-a--.jsonl": [
      sessionHeader({ id: "sess-a", timestamp: "2026-07-31T01:00:00.000Z", cwd: "/home/shial/project/a/" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    ],
    // 标准目录名
    "2026-07-31T02-00-00-000Z_b.jsonl": [
      sessionHeader({ id: "sess-b", timestamp: "2026-07-31T02:00:00.000Z", cwd: "/home/shial/project/a" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }),
    ],
    "2026-07-31T03-00-00-000Z_c.jsonl": [
      sessionHeader({ id: "sess-c", timestamp: "2026-07-31T03:00:00.000Z", cwd: "/other/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 500 }) }),
    ],
  });
  try {
    const rows = parseTableRows(await runCli(["totals", "--dir", dir, "--by", "cwd"]));
    // /home/shial/project/a/ 与 /home/shial/project/a 规范化后同键（去尾斜杠）→ 合并
    assert.equal(rows.length, 2, "两个 cwd 键各一行（尾斜杠规范化合并）");

    const projA = rows.find((r) => r["cwd"] === "/home/shial/project/a");
    const other = rows.find((r) => r["cwd"] === "/other/proj");
    assert.ok(projA, "project/a 行存在（规范化去尾斜杠）");
    assert.ok(other, "/other/proj 行存在");

    // project/a：两个会话合并，input 100+200=300，请求数 2
    assert.equal(projA!["请求数"], "2");
    assert.equal(projA!["输入"], "300");
    // /other/proj：1 条，input 500
    assert.equal(other!["请求数"], "1");
    assert.equal(other!["输入"], "500");

    // 不出现有损目录名作为分组键
    assert.ok(!rows.some((r) => r["cwd"] === "--home-shial-project-a--"), "有损目录名不应作为分组键");
  } finally {
    removeFixture(dir);
  }
});
