/**
 * Bugfix — 服务端 readSessionFiles 缓存（问题汇总 #3）。
 *
 * 背景：webui 每次请求全量 readSessionFiles（222 文件逐行 JSON.parse，~0.5s 单核 CPU），
 * 打开页面 3 个并发请求 ≈ 1.5s 单核 100%，频繁刷新/自动刷新导致 CPU 温度飙升。
 * 修复：readSessionFilesCached 按目录文件 (path, mtimeMs, size) 快照失效，数据不变时
 * 直接返回缓存（零重读）；数据变化（新增/删除/修改文件）自动失效。
 *
 * 断言：无变化时两次调用返回同一数组引用（= 缓存命中未重读）；增删改后返回新引用。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { writeFileSync, appendFileSync, rmSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { readSessionFilesCached } from "../src/analyze.ts";
import { sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

function makeDir(): string {
  return mkdtempSync(join(tmpdir(), "token-analyzer-cache-"));
}
function writeSession(dir: string, name: string, input: number): void {
  const lines = [
    sessionHeader({ timestamp: "2026-07-31T01:55:30.577Z" }),
    messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input }) }),
  ]
    .map((e) => JSON.stringify(e))
    .join("\n") + "\n";
  writeFileSync(join(dir, name), lines);
}

test("缓存命中：目录无变化时两次调用返回同一引用（不重读）", async () => {
  const dir = makeDir();
  writeSession(dir, "a.jsonl", 100);
  try {
    const first = await readSessionFilesCached(dir);
    const second = await readSessionFilesCached(dir);
    assert.equal(first.length, 1);
    assert.equal(first, second, "无变化时应命中缓存（同一数组引用，未重读）");
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("缓存失效：新增文件后返回新数据（新引用）", async () => {
  const dir = makeDir();
  writeSession(dir, "a.jsonl", 100);
  try {
    const first = await readSessionFilesCached(dir);
    writeSession(dir, "b.jsonl", 200);
    const third = await readSessionFilesCached(dir);
    assert.notEqual(first, third, "新增文件后应重读（新引用）");
    assert.equal(third.length, 2, "新数据应包含新增会话");
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("缓存失效：修改文件内容后返回新数据", async () => {
  const dir = makeDir();
  writeSession(dir, "a.jsonl", 100);
  try {
    const first = await readSessionFilesCached(dir);
    // 追加一条消息（size 变化）→ 快照失效
    appendFileSync(
      join(dir, "a.jsonl"),
      JSON.stringify(messageEntry({ role: "assistant", model: "m2", usage: assistantUsage({ input: 999 }) })) + "\n",
    );
    const third = await readSessionFilesCached(dir);
    assert.notEqual(first, third, "文件内容变化后应重读（新引用）");
    assert.equal(third[0].items.length, 2, "重读后应包含追加消息");
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("并发去重：同一快照的并发调用共享一次重读（打开页面 3 请求场景）", async () => {
  const dir = makeDir();
  writeSession(dir, "a.jsonl", 100);
  try {
    // 首次并发 3 个调用：应共享同一 in-flight Promise（只重读一次）
    const [r1, r2, r3] = await Promise.all([readSessionFilesCached(dir), readSessionFilesCached(dir), readSessionFilesCached(dir)]);
    assert.equal(r1, r2, "并发调用应共享同一读取结果（同一引用）");
    assert.equal(r2, r3, "并发调用应共享同一读取结果（同一引用）");
    assert.equal(r1.length, 1);
    // 完成后缓存生效：后续调用命中缓存
    const after = await readSessionFilesCached(dir);
    assert.equal(after, r1, "完成后应命中缓存");
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});
