/**
 * 08 — API 与总览对齐
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, rmSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { startWebServer } from "../src/server.ts";
import { Database } from "../src/db.ts";

function tempDir(): string { return mkdtempSync(join(tmpdir(), "ta-api-")); }
function header(): string { return JSON.stringify({ type: "session", version: 3, id: "sess-1", timestamp: "2026-07-31T01:55:30.577Z", cwd: "/tmp" }); }
function assistant(id: string): string { return JSON.stringify({ type: "message", id, timestamp: "2026-07-31T01:58:29.810Z", message: { role: "assistant", provider: "p", model: "m", usage: { input: 10, output: 5, cacheRead: 3, cacheWrite: 2, totalTokens: 20, cost: { total: 0 } }, stopReason: "stop" } }); }

test("S8-1 GET /api/db/meta 返回 dbPath 与 schemaVersion", async () => {
  const dir = tempDir();
  try {
    writeFileSync(join(dir, "s.jsonl"), header() + "\n" + assistant("a1") + "\n");
    const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
    try {
      const res = await fetch(new URL("/api/db/meta", server.url));
      assert.equal(res.status, 200);
      const body = await res.json() as { dbPath: string; schemaVersion: number };
      assert.ok(typeof body.dbPath === "string" && body.dbPath.length > 0);
      assert.equal(body.schemaVersion, 1);
    } finally { await server.close(); }
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

test("S8-2 scope-note 含四载体文案", () => {
  const html = readFileSync(join("src", "webui.html"), "utf8");
  assert.ok(html.includes("assistant/toolResult/compaction/branch_summary") || html.includes("四载体"), "webui.html 应含四载体说明");
});

test("S8-3 分组表表头“总输入”", () => {
  const html = readFileSync(join("src", "webui.html"), "utf8");
  // 检查是否有“总输入”文案
  assert.ok(html.includes("总输入") || html.includes("input + cacheRead"), "应含总输入");
});
