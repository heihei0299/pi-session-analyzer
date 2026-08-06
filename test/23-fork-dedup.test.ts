/**
 * Bugfix — fork 会话重复统计剔除（ticket 25）。
 *
 * 背景：pi 的 fork 会话（header 含 parentSession）复制原会话历史消息（保留原 timestamp 与 usage），
 * 这些 token 已在原会话统计过，fork 本身未消耗——token-analyzer 之前重复计入。
 * 实测：11 个 fork 会话共 734 条复制历史；019fd362 fork 会话 261 条全为复制（39.5M token 虚高）。
 * 修复（analyze.ts analyzeFile 数据读取层，CLI/webui 一致）：fork 会话中
 * message.timestamp < header.timestamp（fork 创建时间）的消息视为复制快照剔除；
 * fork 后新增消息（ts >= forkTs）保留。
 * 验证：剔除后「今天」total 416.9M vs pi-switch 网关 417.6M（差异 0.2%）。
 *
 * 断言：fork 剔除复制历史 / fork 新消息保留 / 非 fork 不受影响 / CLI 与 webui 一致。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import { analyzeFile } from "../src/analyze.ts";
import { startWebServer } from "../src/server.ts";
import { join } from "node:path";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

function buildDir(): string {
  return makeFixture({
    // 原会话：2 条消息（ts 10:00 / 10:05）
    "2026-08-05T09-00-00-000Z_parent.jsonl": [
      sessionHeader({ id: "p1", timestamp: "2026-08-05T09:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }, { timestamp: "2026-08-05T10:00:00.000Z" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }, { timestamp: "2026-08-05T10:05:00.000Z" }),
    ],
    // fork 会话：header.parentSession 指向原会话、forkTs=10:10；复制 2 条历史 + 1 条新增（10:12）
    "2026-08-05T10-10-00-000Z_fork.jsonl": [
      { ...sessionHeader({ id: "f1", timestamp: "2026-08-05T10:10:00.000Z", cwd: "/proj" }), parentSession: "/proj/2026-08-05T09-00-00-000Z_parent.jsonl" },
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }, { timestamp: "2026-08-05T10:00:00.000Z" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }, { timestamp: "2026-08-05T10:05:00.000Z" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 300 }) }, { timestamp: "2026-08-05T10:12:00.000Z" }),
    ],
  });
}

test("fork 会话剔除复制历史（ts < forkTs），保留 fork 后新增消息", async () => {
  const dir = buildDir();
  try {
    const parent = await analyzeFile(join(dir, "2026-08-05T09-00-00-000Z_parent.jsonl"));
    const fork = await analyzeFile(join(dir, "2026-08-05T10-10-00-000Z_fork.jsonl"));
    assert.equal(parent!.items.length, 2, "原会话 2 条全统计");
    assert.equal(fork!.items.length, 1, "fork 只统计 fork 后新增的 1 条（10:12）");
    assert.equal(fork!.items[0].timestamp, "2026-08-05T10:12:00.000Z", "保留的是新消息");
  } finally {
    removeFixture(dir);
  }
});

test("非 fork 会话（无 parentSession）不受影响", async () => {
  const dir = makeFixture({
    "s.jsonl": [
      sessionHeader({ id: "s1", timestamp: "2026-08-05T09:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() }, { timestamp: "2026-08-05T09:30:00.000Z" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() }, { timestamp: "2026-08-05T10:00:00.000Z" }),
    ],
  });
  try {
    const d = await analyzeFile(join(dir, "s.jsonl"));
    assert.equal(d!.items.length, 2, "无 parentSession 全部统计");
  } finally {
    removeFixture(dir);
  }
});

test("CLI 与 webui 一致剔除 fork 重复（analyzeFile 数据层）", async () => {
  const dir = buildDir();
  try {
    const out = JSON.parse(await runCli(["--dir", dir, "requests", "--format", "json"])) as { rows: unknown[] };
    // parent 2 条 + fork 新增 1 条 = 3（fork 复制 2 条被剔除）
    assert.equal(out.rows.length, 3, "CLI requests 行数 = 3（不含 fork 复制历史）");

    const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
    try {
      const api = (await (await fetch(new URL("/api/requests", server.url))).json()) as { total: number };
      assert.equal(api.total, 3, "webui 明细 total = 3（与 CLI 一致）");
    } finally {
      await server.close();
    }
  } finally {
    removeFixture(dir);
  }
});
