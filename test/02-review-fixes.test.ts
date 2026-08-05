import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli, parseArgs } from "../src/cli.ts";
import {
  makeFixture,
  removeFixture,
  sessionHeader,
  messageEntry,
  assistantUsage,
  parseTableRows,
} from "./helpers.ts";

test("review-1 空会话（合法 header 无计入消息）model 为 '-'，不误标 mixed", async () => {
  const dir = makeFixture({
    "empty.jsonl": [
      sessionHeader({ id: "empty-sess", timestamp: "2026-07-31T01:00:00.000Z", cwd: "/p" }),
      messageEntry({ role: "user", content: [{ type: "text", text: "hi" }] }), // 非 assistant，不计入
    ],
    "normal.jsonl": [
      sessionHeader({ id: "normal-sess", timestamp: "2026-07-31T02:00:00.000Z", cwd: "/q" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() }),
    ],
  });
  try {
    // 表格
    const table = parseTableRows(await runCli(["sessions", "--dir", dir]));
    const empty = table.find((r) => r["会话ID"] === "empty-sess");
    assert.ok(empty, "空会话行存在");
    assert.equal(empty!["模型"], "-", "空会话 model 应为 '-'");

    // JSON
    const json = JSON.parse(await runCli(["sessions", "--dir", dir, "--format", "json"]));
    const emptyJson = json.rows.find((r: Record<string, unknown>) => r.sessionId === "empty-sess");
    assert.equal(emptyJson.model, "-");
    assert.equal(emptyJson.requests, 0);

    // 空会话也纳入会话级窗口（合法会话）
    assert.equal(json.rows.length, 2, "合法会话（含空会话）都应纳入");
  } finally {
    removeFixture(dir);
  }
});

test("review-2 CSV 字段转义：cwd/model 含逗号时不破损", async () => {
  const dir = makeFixture({
    "s.jsonl": [
      sessionHeader({ id: "s1", timestamp: "2026-07-31T01:00:00.000Z", cwd: '/proj/a,b' }),
      messageEntry({ role: "assistant", model: 'm"odel', usage: assistantUsage() }),
    ],
  });
  try {
    const csv = await runCli(["sessions", "--dir", dir, "--format", "csv"]);
    const line = csv.split("\n").find((l) => l.includes("s1"));
    assert.ok(line, "CSV 含 s1 行");
    // 含逗号/引号的字段被引号包裹
    assert.ok(line!.includes('"/proj/a,b"'), "cwd 含逗号应被引号包裹");
    assert.ok(line!.includes('"m""odel"'), "model 含引号应双写转义");
  } finally {
    removeFixture(dir);
  }
});

test("review-3 非法 --format 值报错而非静默回退", () => {
  assert.throws(() => parseArgs(["--format", "yaml"]), /未知格式/);
});

test("review-4 空数据 CSV 仍输出表头（脚本可消费）", async () => {
  const dir = makeFixture({
    "residual.jsonl": [
      { type: "message", id: "r1", parentId: null, timestamp: "2026-07-31T01:00:00.000Z", message: { role: "user", content: [] } },
    ],
  });
  try {
    const csv = await runCli(["sessions", "--dir", dir, "--format", "csv"]);
    assert.ok(csv.includes("sessionId,timestamp,cwd,model,requests,input"), "空数据也应输出表头");
    const json = JSON.parse(await runCli(["sessions", "--dir", dir, "--format", "json"]));
    assert.deepEqual(json.rows, [], "JSON 空数据 rows 为空数组");
  } finally {
    removeFixture(dir);
  }
});
