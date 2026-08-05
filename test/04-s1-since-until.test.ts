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
} from "./helpers.ts";

/** 跨 8/1~8/16 的四个会话 fixture（input 100/200/300/400） */
function buildDir(): string {
  return makeFixture({
    "2026-08-01T10-00-00-000Z_a.jsonl": [
      sessionHeader({ id: "s-a", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    ],
    "2026-08-10T10-00-00-000Z_b.jsonl": [
      sessionHeader({ id: "s-b", timestamp: "2026-08-10T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }),
    ],
    "2026-08-15T23-59-59-000Z_c.jsonl": [
      sessionHeader({ id: "s-c", timestamp: "2026-08-15T23:59:59.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 300 }) }),
    ],
    "2026-08-16T00-00-00-000Z_d.jsonl": [
      sessionHeader({ id: "s-d", timestamp: "2026-08-16T00:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 400 }) }),
    ],
  });
}

test("S1a --since 闭区间含端点：--since 2026-08-10 含 8/10 当天", async () => {
  const dir = buildDir();
  try {
    const since = parseTable(await runCli(["totals", "--dir", dir, "--since", "2026-08-10"]));
    assert.equal(since["请求数"], "3", "含 8/10：b、c、d 三个会话");
    assert.equal(since["输入"], "900", "200+300+400");
  } finally {
    removeFixture(dir);
  }
});

test("S1b --until 闭区间含端点：--until 2026-08-15 含 8/15 全天（23:59:59）", async () => {
  const dir = buildDir();
  try {
    const until = parseTable(await runCli(["totals", "--dir", dir, "--until", "2026-08-15"]));
    assert.equal(until["请求数"], "3", "含 8/15 23:59:59：a、b、c 三个会话");
    assert.equal(until["输入"], "600", "100+200+300");
  } finally {
    removeFixture(dir);
  }
});

test("S1c 闭区间组合：--since 2026-08-10 --until 2026-08-15", async () => {
  const dir = buildDir();
  try {
    const both = parseTable(await runCli(["totals", "--dir", dir, "--since", "2026-08-10", "--until", "2026-08-15"]));
    assert.equal(both["请求数"], "2", "b、c 两个会话");
    assert.equal(both["输入"], "500", "200+300");
  } finally {
    removeFixture(dir);
  }
});

test("S1d 时间筛选对 sessions 窗口生效", async () => {
  const dir = buildDir();
  try {
    const out = await runCli(["sessions", "--dir", dir, "--since", "2026-08-01", "--until", "2026-08-15"]);
    const rows = out.trim().split("\n").length - 1;
    assert.equal(rows, 3, "a/b/c 三个会话行");
  } finally {
    removeFixture(dir);
  }
});

test("S1e 时间筛选对 requests 窗口生效", async () => {
  const dir = buildDir();
  try {
    const out = await runCli(["requests", "--dir", dir, "--since", "2026-08-01", "--until", "2026-08-15"]);
    const rows = out.trim().split("\n").length - 1;
    assert.equal(rows, 3, "a/b/c 三条请求");
  } finally {
    removeFixture(dir);
  }
});

test("S1f 无时区后缀时间戳按 UTC 解析（统一基准）", async () => {
  const dir = makeFixture({
    // 无 Z 后缀（模拟本地时间写入），应仍按 UTC 解释
    "2026-08-10T10-00-00_noz.jsonl": [
      sessionHeader({ id: "s-noz", timestamp: "2026-08-10T10:00:00", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 700 }) }),
    ],
  });
  try {
    const since = parseTable(await runCli(["totals", "--dir", dir, "--since", "2026-08-10"]));
    assert.equal(since["请求数"], "1", "无 Z 时间戳按 UTC 解析后仍落在 8/10");
    assert.equal(since["输入"], "700");
  } finally {
    removeFixture(dir);
  }
});
