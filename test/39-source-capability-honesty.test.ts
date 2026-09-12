/**
 * Ticket 03 — 源能力声明与出口诚实性（TS/npm 侧）。
 * 覆盖：meta.sources 只声明 Pi；TS CLI 对 --source/--codex-dir 明确报错；
 * HTTP source=codex/all 不被静默忽略；details/rename 返回 unsupported 而不是 404。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { join } from "node:path";
import { parseArgs } from "../src/cli.ts";
import { startWebServer } from "../src/server.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

test("T3-1 TS CLI 收到 --source / --codex-dir 明确报错并提示 Go 版本", () => {
  for (const argv of [
    ["totals", "--source", "codex"],
    ["totals", "--source=codex"],
    ["serve", "--source", "all"],
    ["sessions", "--codex-dir", "/tmp/codex"],
    ["totals", "--codex-dir=/tmp/codex"],
  ]) {
    assert.throws(() => parseArgs(argv), /Go 原生版本|Go 版本/, `应显式拒绝: ${argv.join(" ")}`);
    assert.throws(() => parseArgs(argv), /仅 Go 原生版本支持/, `错误应指向 Go 版本: ${argv.join(" ")}`);
  }
});

test("T3-2 TS /api/meta.sources 只声明 Pi，且未被声明 source 不会返回 Pi 数据", async () => {
  const dir = makeFixture({
    "2026-08-01T10-00-00-000Z_s1.jsonl": [
      sessionHeader({ id: "s1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 10, output: 5, cacheRead: 0, cacheWrite: 0, cost: { total: 0 } }) }),
    ],
  });
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const metaRes = await fetch(new URL("/api/meta", server.url));
    assert.equal(metaRes.status, 200);
    const meta = (await metaRes.json()) as { sources: string[] };
    assert.deepEqual(meta.sources, ["pi"], "TS 后端只能声明 Pi");

    for (const source of ["codex", "all"]) {
      const totalsRes = await fetch(new URL(`/api/totals?source=${source}`, server.url));
      assert.equal(totalsRes.status, 400, `source=${source} 不得静默返回 Pi 数据`);
      const body = (await totalsRes.json()) as { error: string; detail: string };
      assert.equal(body.error, "Unsupported");
      assert.match(body.detail, /Go 原生版本/);

      const reqRes = await fetch(new URL(`/api/requests?source=${source}`, server.url));
      assert.equal(reqRes.status, 400, `source=${source} requests 必须拒绝`);
      assert.match(await reqRes.text(), /Unsupported/);

      const detailRes = await fetch(new URL(`/api/sessions/s1/detail?source=${source}`, server.url));
      assert.equal(detailRes.status, 400, `source=${source} 详情必须 unsupported 而非 404`);
      assert.match(await detailRes.text(), /Go 原生版本/);
    }
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

test("T3-3 TS rename 对未声明 source 返回 unsupported 且不写 Pi 目录", async () => {
  const dir = makeFixture({
    "2026-08-01T10-00-00-000Z_s1.jsonl": [
      sessionHeader({ id: "s1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() }),
    ],
  });
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const res = await fetch(new URL("/api/sessions/rename?source=codex", server.url), {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ sessionId: "s1", name: "should-not-write" }),
    });
    assert.equal(res.status, 400, "rename 必须 unsupported 而非 404/写盘");
    assert.match(await res.text(), /Unsupported/);
    assert.ok(existsSync(join(dir, "2026-08-01T10-00-00-000Z_s1.jsonl")), "原 Pi 文件不得被移动");
    assert.ok(!existsSync(join(dir, "should-not-write_s1.jsonl")), "不得写入 Pi 目录");
  } finally {
    await server.close();
    removeFixture(dir);
  }
});
