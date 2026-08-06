import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import {
  makeFixture,
  removeFixture,
  sessionHeader,
  messageEntry,
  assistantUsage,
  zeroUsage,
} from "./helpers.ts";

const SESSIONS = {
  "2026-07-31T01-00-00-000Z_a.jsonl": [
    sessionHeader({ id: "sess-a", timestamp: "2026-07-31T01:00:00.000Z", cwd: "/proj/a" }),
    messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 300 }) }),
  ],
  "2026-07-31T02-00-00-000Z_b.jsonl": [
    sessionHeader({ id: "sess-b", timestamp: "2026-07-31T02:00:00.000Z", cwd: "/proj/b" }),
    messageEntry({ role: "assistant", model: "m2", usage: assistantUsage({ input: 500 }) }),
  ],
};

test("S4a JSON totals：窗口字段 + 原始数值（cacheRate 小数）", async () => {
  const dir = makeFixture(SESSIONS);
  try {
    const totals = JSON.parse(await runCli(["totals", "--dir", dir, "--format", "json"]));
    assert.equal(totals.window, "totals");
    assert.equal(totals.requests, 3);
    assert.equal(totals.input, 900);
    assert.equal(totals.output, 150); // 50×3
    assert.equal(totals.cacheRate, 600 / (900 + 600)); // 原始小数非百分比（分母不含缓存写）
  } finally {
    removeFixture(dir);
  }
});

test("S4b JSON sessions：行含元数据与指标，值为原始数值", async () => {
  const dir = makeFixture(SESSIONS);
  try {
    const sessions = JSON.parse(await runCli(["sessions", "--dir", dir, "--format", "json"]));
    assert.equal(sessions.window, "sessions");
    assert.equal(sessions.rows.length, 2);
    const a = sessions.rows.find((r: Record<string, unknown>) => r.sessionId === "sess-a");
    assert.ok(a);
    assert.equal(a.timestamp, "2026-07-31T01:00:00.000Z");
    assert.equal(a.cwd, "/proj/a");
    assert.equal(a.model, "m1");
    assert.equal(a.requests, 2);
    assert.equal(a.input, 400);
    assert.equal(a.cost, 0.2); // 0.1×2 原始数值
  } finally {
    removeFixture(dir);
  }
});

test("S4c JSON requests：逐消息一行，请求数恒 1", async () => {
  const dir = makeFixture(SESSIONS);
  try {
    const requests = JSON.parse(await runCli(["requests", "--dir", dir, "--format", "json"]));
    assert.equal(requests.window, "requests");
    assert.equal(requests.rows.length, 3);
    const r2 = requests.rows.find((r: Record<string, unknown>) => r.input === 300);
    assert.ok(r2);
    assert.equal(r2.sessionId, "sess-a");
    assert.equal(r2.model, "m1");
    assert.equal(r2.requests, 1);
  } finally {
    removeFixture(dir);
  }
});

test("S4d JSON 全 0 失败消息：值为数字 0（非标注文本）", async () => {
  const dir = makeFixture({
    "s.jsonl": [
      sessionHeader({ id: "s1", timestamp: "2026-07-31T01:00:00.000Z", cwd: "/p" }),
      messageEntry({ role: "assistant", model: "m", stopReason: "aborted", usage: zeroUsage() }),
    ],
  });
  try {
    const out = await runCli(["requests", "--dir", dir, "--format", "json"]);
    const parsed = JSON.parse(out);
    assert.equal(parsed.rows[0].cost, 0);
    assert.equal(typeof parsed.rows[0].cost, "number");
    assert.equal(parsed.rows[0].totalTokens, 0);
  } finally {
    removeFixture(dir);
  }
});
