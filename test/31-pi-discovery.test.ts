/**
 * 02 — Pi 会话发现（双布局 + 环境变量）
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { collectPiJsonlFiles, resolvePiSessionRoot } from "../src/pi-discovery.ts";

function tempDir(): string {
  return mkdtempSync(join(tmpdir(), "ta-discovery-"));
}

test("S2-1 Flat 布局仅枚举根下 *.jsonl", () => {
  const root = tempDir();
  try {
    writeFileSync(join(root, "a.jsonl"), `{}\n`);
    writeFileSync(join(root, "b.jsonl"), `{}\n`);
    mkdirSync(join(root, "sub"));
    writeFileSync(join(root, "sub", "c.jsonl"), `{}\n`);
    const files = collectPiJsonlFiles(root, "flat");
    const names = files.map((f) => f.split("/").pop()).sort();
    assert.deepEqual(names, ["a.jsonl", "b.jsonl"]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("S2-2 ProjectDirectories 仅枚举 <project>/*.jsonl 两层", () => {
  const root = tempDir();
  try {
    mkdirSync(join(root, "projA"), { recursive: true });
    mkdirSync(join(root, "projB"), { recursive: true });
    writeFileSync(join(root, "projA", "a.jsonl"), `{}\n`);
    writeFileSync(join(root, "projB", "b.jsonl"), `{}\n`);
    writeFileSync(join(root, "c.jsonl"), `{}\n`);
    const files = collectPiJsonlFiles(root, "projectDirectories");
    const names = files.map((f) => f.split("/").pop()).sort();
    assert.deepEqual(names, ["a.jsonl", "b.jsonl"]);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
});

test("S2-3 resolvePiSessionRoot 环境变量优先于默认", () => {
  const envRoot = tempDir();
  try {
    mkdirSync(envRoot, { recursive: true });
    const result = resolvePiSessionRoot({ envDb: envRoot, defaultRoot: "/tmp/default", piConfig: undefined });
    assert.equal(result.root, envRoot);
    assert.equal(result.layout, "flat");
  } finally {
    rmSync(envRoot, { recursive: true, force: true });
  }
});

test("S2-4 相对路径 PI_CODING_AGENT_SESSION_DIR 抛 PI_SESSION_DIR_REQUIRES_PROJECT_CONTEXT", () => {
  assert.throws(() => resolvePiSessionRoot({ envDb: ".pi/sessions", defaultRoot: "/tmp/default", piConfig: undefined }), /PI_SESSION_DIR_REQUIRES_PROJECT_CONTEXT/);
  assert.throws(() => resolvePiSessionRoot({ envDb: "relative/path", defaultRoot: "/tmp/default", piConfig: undefined }), /PI_SESSION_DIR_REQUIRES_PROJECT_CONTEXT/);
});

test("S2-5 绝对路径正常返回 flat", () => {
  const abs = "/tmp/abs-pi-sessions";
  const result = resolvePiSessionRoot({ envDb: abs, defaultRoot: "/tmp/default", piConfig: undefined });
  assert.equal(result.root, abs);
  assert.equal(result.layout, "flat");
});
