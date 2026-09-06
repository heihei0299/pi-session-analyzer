/**
 * 05 — 指纹增量同步
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, readFileSync, appendFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { piFileRevision } from "../src/pi-sync.ts";
import { Database } from "../src/db.ts";

function tempDir(): string {
  return mkdtempSync(join(tmpdir(), "ta-incr-"));
}

function sessionHeader(id = "session-a"): string {
  return JSON.stringify({ type: "session", version: 3, id, timestamp: "2026-07-31T01:55:30.577Z", cwd: "/tmp" });
}

function assistantEntry(id: string): string {
  return JSON.stringify({
    type: "message",
    id,
    timestamp: "2026-07-31T01:58:29.810Z",
    message: {
      role: "assistant",
      provider: "p",
      model: "m",
      usage: { input: 10, output: 5, cacheRead: 3, cacheWrite: 2, totalTokens: 20, cost: { total: 0.1 } },
      stopReason: "stop",
    },
  });
}

test("S5-1 piFileRevision 指纹与 complete 标记", () => {
  const dir = tempDir();
  try {
    const file = join(dir, "s.jsonl");
    writeFileSync(file, sessionHeader() + "\n" + assistantEntry("a1") + "\n");
    const rev1 = piFileRevision(file);
    assert.ok(rev1.fileSize > 0);
    assert.equal(rev1.complete, true);
    assert.ok(typeof rev1.tailFingerprint === "number");
    // 追加后指纹变化
    appendFileSync(file, assistantEntry("a2") + "\n");
    const rev2 = piFileRevision(file);
    assert.notEqual(rev1.tailFingerprint, rev2.tailFingerprint);
    assert.ok(rev2.fileSize > rev1.fileSize);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("S5-2 未换行残段 complete=false", () => {
  const dir = tempDir();
  try {
    const file = join(dir, "s.jsonl");
    writeFileSync(file, sessionHeader() + "\n" + assistantEntry("a1")); // 无尾换行
    const rev = piFileRevision(file);
    assert.equal(rev.complete, false);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("S5-3 sync 增量：首轮导入，次轮 0，追加后 1", async () => {
  const dir = tempDir();
  const dbDir = tempDir();
  const dbPath = join(dbDir, "test.db");
  try {
    const file = join(dir, "s.jsonl");
    writeFileSync(file, sessionHeader("sess-1") + "\n" + assistantEntry("a1") + "\n");
    const db = await Database.getInstance(dbPath);
    const { syncPiUsage } = await import("../src/pi-sync.ts");
    const r1 = await syncPiUsage(db, [file]);
    assert.equal(r1.imported, 1);
    const r2 = await syncPiUsage(db, [file]);
    assert.equal(r2.imported, 0);
    appendFileSync(file, assistantEntry("a2") + "\n");
    const r3 = await syncPiUsage(db, [file]);
    assert.equal(r3.imported, 1);
    await db.close();
  } finally {
    rmSync(dir, { recursive: true, force: true });
    rmSync(dbDir, { recursive: true, force: true });
  }
});
