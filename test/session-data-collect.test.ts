import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, chmodSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { SessionData } from "../src/session-data.ts";
import { runCli } from "../src/cli.ts";

function isRoot(): boolean {
  try { return typeof process.getuid === "function" && process.getuid() === 0; } catch { return false; }
}

describe("collectJsonlFiles 容错：不可读子目录跳过（CLI 崩溃回归）", () => {
  test("unreadable subdir 不抛 EACCES，且仍返回可读目录下的会话文件", async () => {
    if (isRoot()) {
      console.log("  # SKIP: root 用户可读 000 目录，EACCES 不触发，跳过本用例");
      return;
    }
    const dir = mkdtempSync(join(tmpdir(), "token-analyzer-collect-"));
    try {
      writeFileSync(join(dir, "a.jsonl"), JSON.stringify({ type: "session", id: "s1", timestamp: "2026-08-10T10:00:00.000Z", cwd: "/proj" }) + "\n");
      const unreadable = join(dir, "unreadable");
      mkdirSync(unreadable);
      chmodSync(unreadable, 0o000);
      const sd = new SessionData();
      let files: string[] = [];
      assert.doesNotThrow(() => { files = sd.collectJsonlFiles(dir); }, "collectJsonlFiles 不应对不可读子目录抛 EACCES");
      assert.ok(files.some((f) => f.endsWith("a.jsonl")), "应返回可读目录下的 .jsonl");
      assert.ok(!files.some((f) => f.includes("unreadable")), "不可读目录下的文件应被跳过");
    } finally {
      try { chmodSync(join(dir, "unreadable"), 0o700); } catch {}
      rmSync(dir, { recursive: true, force: true });
    }
  });

  test("runCli 对不可读子目录不崩溃（友好 totals 输出）", async () => {
    if (isRoot()) {
      console.log("  # SKIP: root 跳过");
      return;
    }
    const dir = mkdtempSync(join(tmpdir(), "token-analyzer-cli-"));
    try {
      writeFileSync(join(dir, "a.jsonl"), [
        JSON.stringify({ type: "session", id: "s1", timestamp: "2026-08-10T10:00:00.000Z", cwd: "/proj" }),
        JSON.stringify({ type: "message", id: "m1", timestamp: "2026-08-10T10:01:00.000Z", message: { role: "assistant", model: "m1", usage: { input: 10, output: 5, cacheRead: 0, cacheWrite: 0, reasoning: 0, cost: { total: 0.01 } } } }),
      ].join("\n") + "\n");
      const unreadable = join(dir, "unreadable");
      mkdirSync(unreadable);
      chmodSync(unreadable, 0o000);
      let out = "";
      await assert.doesNotReject(async () => { out = await runCli(["totals", "--dir", dir, "--format", "json"]); }, "runCli 不应对不可读子目录抛 EACCES");
      const body = JSON.parse(out);
      assert.equal(body.requests, 1, "应正常统计可读会话");
    } finally {
      try { chmodSync(join(dir, "unreadable"), 0o700); } catch {}
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
