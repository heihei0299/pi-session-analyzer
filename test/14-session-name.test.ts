/**
 * Bugfix — 会话管理显示名派生（问题汇总 #2）。
 *
 * 背景：displayNameOf 仅取文件名前缀；pi 默认文件名 `<时间戳>_<UUID>.jsonl` 前缀是
 * 时间戳（如 2026-08-03T12-54-19-759Z），会话管理页显示一堆时间戳，无法直观看出会话内容。
 * 需求：重命名过的（文件名前缀非时间戳格式）用前缀；未重命名的用首条 user 消息文本
 * （analyzeFile 新增 firstUserText 提取）。
 *
 * 断言（HTTP 层）：/api/sessions 的 displayName 字段按上述规则派生。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { startWebServer } from "../src/server.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

async function fetchSessions(dir: string): Promise<Array<Record<string, unknown>>> {
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const res = await fetch(new URL("/api/sessions", server.url));
    assert.equal(res.status, 200);
    const body = (await res.json()) as { rows: Array<Record<string, unknown>> };
    return body.rows;
  } finally {
    await server.close();
  }
}

async function fetchRequests(dir: string): Promise<Array<Record<string, unknown>>> {
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const res = await fetch(new URL("/api/requests", server.url));
    assert.equal(res.status, 200);
    const body = (await res.json()) as { rows: Array<Record<string, unknown>> };
    return body.rows;
  } finally {
    await server.close();
  }
}

const TS_FILE = "2026-08-03T12-54-19-759Z_019fc7b0-7b6f-78d3-bed5-8be6b23edff6.jsonl";
const RENAMED_FILE = "生成规范化的commit_019faaaa-f24d-7548-b92b-7c3eace406a0.jsonl";

function userMsg(text: string): Record<string, unknown> {
  return messageEntry({ role: "user", content: [{ type: "text", text }] });
}

test("默认时间戳文件名：displayName 取首条 user 消息文本", async () => {
  const dir = makeFixture({
    [TS_FILE]: [
      sessionHeader(),
      userMsg("生成规范化的commit"),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() }),
    ],
  });
  try {
    const rows = await fetchSessions(dir);
    assert.equal(rows.length, 1);
    assert.equal(rows[0].displayName, "生成规范化的commit");
  } finally {
    removeFixture(dir);
  }
});

test("默认时间戳文件名：首条 user 消息取 content 第一个 text（跳过非 text 段）", async () => {
  const dir = makeFixture({
    [TS_FILE]: [
      sessionHeader(),
      messageEntry({
        role: "user",
        content: [
          { type: "image", url: "x.png" },
          { type: "text", text: "  帮我看看这个报错  " },
        ],
      }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() }),
    ],
  });
  try {
    const rows = await fetchSessions(dir);
    assert.equal(rows[0].displayName, "帮我看看这个报错");
  } finally {
    removeFixture(dir);
  }
});

test("默认时间戳文件名且无 user 消息：回退时间戳前缀", async () => {
  const dir = makeFixture({
    [TS_FILE]: [
      sessionHeader(),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() }),
    ],
  });
  try {
    const rows = await fetchSessions(dir);
    assert.equal(rows[0].displayName, "2026-08-03T12-54-19-759Z");
  } finally {
    removeFixture(dir);
  }
});

test("重命名过的文件名：displayName 取文件名前缀（首条 user 消息被忽略）", async () => {
  const dir = makeFixture({
    [RENAMED_FILE]: [
      sessionHeader(),
      userMsg("别的消息内容"),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() }),
    ],
  });
  try {
    const rows = await fetchSessions(dir);
    assert.equal(rows[0].displayName, "生成规范化的commit");
  } finally {
    removeFixture(dir);
  }
});

// ---------- /api/requests 行含 displayName（会话名称列数据源） ----------

test("请求明细行含 displayName：默认时间戳文件名取首条 user 消息", async () => {
  const dir = makeFixture({
    [TS_FILE]: [
      sessionHeader(),
      userMsg("生成规范化的commit"),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() }),
    ],
  });
  try {
    const rows = await fetchRequests(dir);
    assert.equal(rows.length, 1);
    assert.equal(rows[0].displayName, "生成规范化的commit", "请求行应带会话名称");
  } finally {
    removeFixture(dir);
  }
});

test("请求明细行含 displayName：重命名过的取文件名前缀，多请求行同会话名", async () => {
  const dir = makeFixture({
    [RENAMED_FILE]: [
      sessionHeader(),
      userMsg("别的消息内容"),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 1 }) }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 2 }) }),
    ],
  });
  try {
    const rows = await fetchRequests(dir);
    assert.equal(rows.length, 2);
    assert.equal(rows[0].displayName, "生成规范化的commit");
    assert.equal(rows[1].displayName, "生成规范化的commit", "同一会话的所有请求行同名称");
  } finally {
    removeFixture(dir);
  }
});
