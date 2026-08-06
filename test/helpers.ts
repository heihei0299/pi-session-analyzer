import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

export type JsonEntry = Record<string, unknown>;
export type FixtureLine = JsonEntry | string;

/** 构造一个临时 fixture 目录，返回其路径（string 行按原文写入，用于坏 JSON 行） */
export function makeFixture(files: Record<string, FixtureLine[]>): string {
  const dir = mkdtempSync(join(tmpdir(), "token-analyzer-test-"));
  for (const [name, entries] of Object.entries(files)) {
    const lines = entries.map((e) => (typeof e === "string" ? e : JSON.stringify(e))).join("\n") + "\n";
    writeFileSync(join(dir, name), lines);
  }
  return dir;
}

export function removeFixture(dir: string): void {
  rmSync(dir, { recursive: true, force: true });
}

/** 合法会话首行 header（默认值与真实数据同构） */
export function sessionHeader(overrides: Record<string, unknown> = {}): JsonEntry {
  return {
    type: "session",
    version: 3,
    id: "019fb5e2-3c91-76bd-b12c-c8d2ab31c532",
    timestamp: "2026-07-31T01:55:30.577Z",
    cwd: "/home/shial",
    ...overrides,
  };
}

/** 一条 message entry（message 对象可整体覆盖） */
export function messageEntry(
  message: Record<string, unknown>,
  overrides: Record<string, unknown> = {},
): JsonEntry {
  return {
    type: "message",
    id: "m1",
    parentId: null,
    timestamp: "2026-07-31T01:58:29.810Z",
    message,
    ...overrides,
  };
}

/** 默认 assistant usage（与真实数据结构同构） */
export function assistantUsage(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    input: 100,
    output: 50,
    cacheRead: 200,
    cacheWrite: 10,
    reasoning: 20,
    totalTokens: 360, // 原始字段保真（组件和值）；聚合不信任此字段，总 token 按网关口径 input+cacheRead+output（ADR-0002）
    cost: { input: 0.01, output: 0.02, cacheRead: 0.03, cacheWrite: 0.04, total: 0.1 },
    ...overrides,
  };
}

/** 全 0 usage（失败/中止的 assistant 消息，与真实数据同构） */
export function zeroUsage(): Record<string, unknown> {
  return {
    input: 0,
    output: 0,
    cacheRead: 0,
    cacheWrite: 0,
    totalTokens: 0,
    cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
  };
}

/** 解析 CLI 表格输出，返回 { 表头: 值 } 映射（按双空格列分隔） */
export function parseTable(out: string): Record<string, string> {
  const rows = parseTableRows(out);
  return rows[0];
}

/** 解析 CLI 表格输出，返回每数据行一个 { 表头: 值 } 映射 */
export function parseTableRows(out: string): Record<string, string>[] {
  const lines = out.trim().split("\n");
  const headers = lines[0].trim().split(/\s{2,}/);
  return lines.slice(1).map((line) => {
    const values = line.trim().split(/\s{2,}/);
    const row: Record<string, string> = {};
    headers.forEach((h, i) => {
      row[h] = values[i];
    });
    return row;
  });
}
