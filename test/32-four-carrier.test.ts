/**
 * 03 — 四载体解析与门控
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { parsePiUsageRecord } from "../src/pi-parse.ts";

function makeAssistantEntry(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    type: "message",
    id: "a1",
    timestamp: "2026-07-31T01:58:29.810Z",
    message: {
      role: "assistant",
      provider: "fixture-provider",
      model: "fixture-model",
      responseModel: "actual-model",
      responseId: "resp-1",
      stopReason: "stop",
      usage: { input: 10, output: 5, cacheRead: 3, cacheWrite: 2, totalTokens: 20, cost: { input: 0.01, output: 0.02, cacheRead: 0.03, cacheWrite: 0.04, total: 0.1 } },
      timestamp: 1700000000000,
      content: [{ type: "text", text: "ok" }],
    },
    ...overrides,
  };
}

test("S3-1 assistant 载体解析成功", () => {
  const rec = parsePiUsageRecord(makeAssistantEntry(), "session-a", 1700000000, 1700000000);
  assert.ok(rec);
  assert.equal(rec!.kind, "assistant");
  assert.equal(rec!.input, 10);
  assert.equal(rec!.output, 5);
  assert.equal(rec!.cacheRead, 3);
  assert.equal(rec!.cacheWrite, 2);
  assert.equal(rec!.provider, "fixture-provider");
  assert.equal(rec!.requestModel, "fixture-model");
  assert.equal(rec!.model, "actual-model");
});

test("S3-2 toolResult 载体解析成功", () => {
  const entry = {
    type: "message",
    id: "t1",
    timestamp: "2026-07-31T01:58:30.810Z",
    message: {
      role: "toolResult",
      toolCallId: "tool-1",
      toolName: "nested",
      content: [],
      usage: { input: 3, output: 4, cacheRead: 1, cacheWrite: 1, totalTokens: 9, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
    },
  };
  const rec = parsePiUsageRecord(entry, "session-a", 1700000000, 1700000000);
  assert.ok(rec);
  assert.equal(rec!.kind, "tool_result");
  assert.equal(rec!.input, 3);
});

test("S3-3 compaction 载体解析成功", () => {
  const entry = {
    type: "compaction",
    id: "c1",
    timestamp: "2026-07-31T01:58:31.810Z",
    summary: "summary",
    usage: { input: 11, output: 12, cacheRead: 2, cacheWrite: 3, totalTokens: 28, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
  };
  const rec = parsePiUsageRecord(entry, "session-a", 1700000000, 1700000000);
  assert.ok(rec);
  assert.equal(rec!.kind, "compaction");
  assert.equal(rec!.input, 11);
});

test("S3-4 branch_summary 载体解析成功", () => {
  const entry = {
    type: "branch_summary",
    id: "b1",
    timestamp: "2026-07-31T01:58:32.810Z",
    summary: "branch summary",
    usage: { input: 13, output: 14, cacheRead: 4, cacheWrite: 5, totalTokens: 36, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
  };
  const rec = parsePiUsageRecord(entry, "session-a", 1700000000, 1700000000);
  assert.ok(rec);
  assert.equal(rec!.kind, "branch_summary");
});

test("S3-5 门控：全0无cost无失败 → 丢弃", () => {
  const entry = makeAssistantEntry({
    message: {
      role: "assistant",
      provider: "p",
      model: "m",
      usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
      stopReason: "stop",
    },
  });
  const rec = parsePiUsageRecord(entry, "session-a", 1700000000, 1700000000);
  assert.equal(rec, null);
});

test("S3-6 门控：全0但 failed (error) → 保留", () => {
  const entry = makeAssistantEntry({
    message: {
      role: "assistant",
      provider: "p",
      model: "m",
      usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
      stopReason: "error",
    },
  });
  const rec = parsePiUsageRecord(entry, "session-a", 1700000000, 1700000000);
  assert.ok(rec);
  assert.equal(rec!.statusCode, 500);
});

test("S3-7 非 assistant 的 model 归一：toolResult 固化 unknown", () => {
  const entry = {
    type: "message",
    id: "t1",
    timestamp: "2026-07-31T01:58:30.810Z",
    message: {
      role: "toolResult",
      usage: { input: 1, output: 1, cacheRead: 0, cacheWrite: 0, totalTokens: 2, cost: { total: 0 } },
    },
  };
  const rec = parsePiUsageRecord(entry, "session-a", 1700000000, 1700000000);
  assert.ok(rec);
  assert.equal(rec!.model, "unknown");
  assert.equal(rec!.provider, "_pi_session");
});

test("S3-8 responseModel 优先于 model", () => {
  const rec = parsePiUsageRecord(makeAssistantEntry(), "session-a", 1700000000, 1700000000);
  assert.equal(rec!.model, "actual-model");
  assert.equal(rec!.requestModel, "fixture-model");
});
