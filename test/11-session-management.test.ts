/**
 * Ticket 06 — 会话管理（/api/sessions 扩展字段 + POST /api/sessions/rename + UI 骨架）。
 * Seam：HTTP 层（fetch）+ 文件系统副作用断言（文件名变化、UUID 保留、header 未改、统计不变）。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync, utimesSync } from "node:fs";
import { join } from "node:path";
import { startWebServer } from "../src/server.ts";
import {
  makeFixture,
  removeFixture,
  sessionHeader,
  messageEntry,
  assistantUsage,
} from "./helpers.ts";

const UUID = "019fb5e2-3c91-76bd-b12c-c8d2ab31c532";

/** 一个非活跃（mtime 10 分钟前）的合法会话 fixture */
function makeInactiveSessionDir(): string {
  const dir = makeFixture({
    [`2026-07-31T01-55-30-577Z_${UUID}.jsonl`]: [
      sessionHeader({ id: UUID }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    ],
  });
  const old = new Date(Date.now() - 10 * 60 * 1000);
  utimesSync(join(dir, `2026-07-31T01-55-30-577Z_${UUID}.jsonl`), old, old);
  return dir;
}

async function renameReq(serverUrl: string, sessionId: string, name: string): Promise<Response> {
  return fetch(new URL("/api/sessions/rename", serverUrl), {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ sessionId, name }),
  });
}

test("S17 /api/sessions 扩展字段 fileName/displayName/cwdNorm，显示名派生正确", async () => {
  const dir = makeFixture({
    [`2026-07-31T01-55-30-577Z_${UUID}.jsonl`]: [sessionHeader({ id: UUID, cwd: "/nonexistent/proj-a" })],
    "ni_aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa.jsonl": [sessionHeader({ id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", cwd: "/nonexistent/proj-b" })],
    "周末复盘_bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb.jsonl": [sessionHeader({ id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", cwd: "/nonexistent/proj-c" })],
    "plain.jsonl": [sessionHeader({ id: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", cwd: "/nonexistent/proj-d" })],
  });
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const res = await fetch(new URL("/api/sessions", server.url));
    assert.equal(res.status, 200);
    const api = (await res.json()) as { rows: Record<string, unknown>[] };
    const byId = new Map(api.rows.map((r) => [r.sessionId as string, r]));
    const byName = (n: string): Record<string, unknown> => {
      const row = [...api.rows].find((r) => r.fileName === n);
      assert.ok(row, `应存在文件 ${n}`);
      return row!;
    };

    // 标准时间戳名 → 显示名 = 时间戳前缀
    const std = byName(`2026-07-31T01-55-30-577Z_${UUID}.jsonl`);
    assert.equal(std.displayName, "2026-07-31T01-55-30-577Z");
    assert.equal(std.cwdNorm, "/nonexistent/proj-a");

    // ni_<uuid> → ni
    assert.equal(byName("ni_aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa.jsonl").displayName, "ni");

    // <中文名>_<uuid> → 中文名
    assert.equal(byName("周末复盘_bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb.jsonl").displayName, "周末复盘");

    // 无 _ 尾缀 → 原始文件名
    assert.equal(byName("plain.jsonl").displayName, "plain.jsonl");

    // 原始 cwd 字段保留
    assert.equal(std.cwd, "/nonexistent/proj-a");
    assert.ok(byId.has(UUID));
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

test("S18 重命名成功：尾 UUID 保留且等于 header id、header 未改、/api/totals 统计不变", async () => {
  const dir = makeInactiveSessionDir();
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const before = await (await fetch(new URL("/api/totals", server.url))).json();
    const res = await renameReq(server.url, UUID, "周末复盘");
    assert.equal(res.status, 200);
    const body = (await res.json()) as { ok: boolean; fileName: string };
    assert.equal(body.ok, true);
    assert.equal(body.fileName, `周末复盘_${UUID}.jsonl`);

    // 文件系统副作用：旧名消失、新名出现、尾 UUID 保留且等于 header id
    const names = readdirSync(dir);
    assert.ok(names.includes(`周末复盘_${UUID}.jsonl`), "新文件名应存在");
    assert.ok(!names.includes(`2026-07-31T01-55-30-577Z_${UUID}.jsonl`), "旧文件名应消失");
    assert.ok(names[0].endsWith(`_${UUID}.jsonl`), "尾 UUID 应保留");
    const headerLine = readFileSync(join(dir, names[0]), "utf8").split("\n")[0];
    assert.deepEqual(JSON.parse(headerLine), sessionHeader({ id: UUID }), "header 内容未改");

    // 统计口径不受影响
    const after = await (await fetch(new URL("/api/totals", server.url))).json();
    assert.deepEqual(after, before, "rename 前后 /api/totals 数字一致");
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

test("S19 非法显示名 → 400：空名 / 纯空白 / 纯非法字符（去除后为空）", async () => {
  const dir = makeInactiveSessionDir();
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    for (const name of ["", "   ", "///", "\\/:*?\"<>|"]) {
      const res = await renameReq(server.url, UUID, name);
      assert.equal(res.status, 400, `name=${JSON.stringify(name)} 应 400`);
      const body = (await res.json()) as { error: string; detail: string };
      assert.ok(body.error.length > 0 && body.detail.length > 0);
    }
    // 混合非法字符的合法名照常改名（去除非法字符）
    const ok = await renameReq(server.url, UUID, "a/b:c");
    assert.equal(ok.status, 200);
    assert.equal(((await ok.json()) as { fileName: string }).fileName, `abc_${UUID}.jsonl`);
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

test("S20 会话不存在 → 404", async () => {
  const dir = makeInactiveSessionDir();
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const res = await renameReq(server.url, "00000000-0000-4000-8000-000000000000", "测试");
    assert.equal(res.status, 404);
    const body = (await res.json()) as { error: string; detail: string };
    assert.ok(body.error.length > 0 && body.detail.length > 0);
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

test("S21 活跃会话 409 + 目标重名 409", async () => {
  // 活跃会话（mtime=now）：makeFixture 默认 mtime 为当前时间
  const activeDir = makeFixture({
    [`2026-07-31T01-55-30-577Z_${UUID}.jsonl`]: [sessionHeader({ id: UUID })],
  });
  const activeServer = await startWebServer({ dir: activeDir, host: "127.0.0.1", port: 0 });
  try {
    const res = await renameReq(activeServer.url, UUID, "新名");
    assert.equal(res.status, 409, "mtime ≤ 5min 的活跃会话应 409");
    const body = (await res.json()) as { detail: string };
    assert.match(body.detail, /会话活跃中/);
  } finally {
    await activeServer.close();
    removeFixture(activeDir);
  }

  // 目标同名文件已存在 → 409「同名文件已存在」
  const conflictDir = makeFixture({
    [`2026-07-31T01-55-30-577Z_${UUID}.jsonl`]: [sessionHeader({ id: UUID })],
    [`占用_${UUID}.jsonl`]: [sessionHeader({ id: UUID, timestamp: "2026-07-30T00:00:00.000Z" })],
  });
  const old = new Date(Date.now() - 10 * 60 * 1000);
  utimesSync(join(conflictDir, `2026-07-31T01-55-30-577Z_${UUID}.jsonl`), old, old);
  const conflictServer = await startWebServer({ dir: conflictDir, host: "127.0.0.1", port: 0 });
  try {
    const res = await renameReq(conflictServer.url, UUID, "占用");
    assert.equal(res.status, 409);
    const body = (await res.json()) as { detail: string };
    assert.match(body.detail, /同名文件已存在/);
  } finally {
    await conflictServer.close();
    removeFixture(conflictDir);
  }
});

test("S22 会话管理 UI 骨架：按 cwd 分组容器 / 折叠控件 / 点击名称行内编辑", async () => {
  const dir = makeInactiveSessionDir();
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const res = await fetch(server.url);
    assert.equal(res.status, 200);
    const body = await res.text();
    assert.match(body, /id="session-groups"/);
    assert.match(body, /session-group/);
    assert.match(body, /collapsed/);
    // 点击名称进入行内编辑（无独立重命名按钮/输入框）
    assert.match(body, /session-row \.name/);
    assert.match(body, /startSessionRename\(nameEl\)/);
    assert.match(body, /rename-input/);
    assert.doesNotMatch(body, /data-rename-btn/, "不应有独立重命名按钮");
    assert.doesNotMatch(body, /data-rename-input/, "不应有独立新显示名输入框");
    assert.match(body, /fetch\("\/api\/sessions\/rename"/);
  } finally {
    await server.close();
    removeFixture(dir);
  }
});
