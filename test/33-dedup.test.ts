/**
 * 04 — 双账本去重（request_id/semantic_id）
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { piRequestIdentity, hashField } from "../src/pi-identity.ts";
import { createHash } from "node:crypto";

function makeEntry(id: string | null, usage: Record<string, unknown>): Record<string, unknown> {
  const base: Record<string, unknown> = {
    type: "message",
    timestamp: "2026-07-31T01:58:29.810Z",
    message: {
      role: "assistant",
      provider: "p",
      model: "m",
      usage,
      stopReason: "stop",
      content: [{ type: "text", text: "ok" }],
    },
  };
  if (id !== null) (base as Record<string, unknown>).id = id;
  return base;
}

test("S4-1 同 id 同 payload 产生相同 request_id/semantic_id", () => {
  const usage = { input: 10, output: 5, cacheRead: 3, cacheWrite: 2, totalTokens: 20, cost: { total: 0.1 } };
  const e1 = makeEntry("a1", usage);
  const e2 = makeEntry("a1", usage);
  const id1 = piRequestIdentity(e1, "assistant", usage, e1.message as Record<string, unknown>);
  const id2 = piRequestIdentity(e2, "assistant", usage, e2.message as Record<string, unknown>);
  assert.equal(id1.requestId, id2.requestId);
  assert.equal(id1.semanticId, id2.semanticId);
  assert.equal(id1.hasEntryId, true);
});

test("S4-2 无 id 时 requestId == semanticId 且 hasEntryId false", () => {
  const usage = { input: 10, output: 5, cacheRead: 3, cacheWrite: 2, totalTokens: 20, cost: { total: 0.1 } };
  const e = makeEntry(null, usage);
  const id = piRequestIdentity(e, "assistant", usage, e.message as Record<string, unknown>);
  assert.equal(id.requestId, id.semanticId);
  assert.equal(id.hasEntryId, false);
});

test("S4-3 同 payload 不同 id → request_id 不同但 semantic 相同", () => {
  const usage = { input: 10, output: 5, cacheRead: 3, cacheWrite: 2, totalTokens: 20, cost: { total: 0.1 } };
  const e1 = makeEntry("a1", usage);
  const e2 = makeEntry("a2", usage);
  const id1 = piRequestIdentity(e1, "assistant", usage, e1.message as Record<string, unknown>);
  const id2 = piRequestIdentity(e2, "assistant", usage, e2.message as Record<string, unknown>);
  assert.notEqual(id1.requestId, id2.requestId);
  assert.equal(id1.semanticId, id2.semanticId);
});

test("S4-4 hash_field 长度前缀一致", () => {
  const h1 = hashField(Buffer.from("hello"));
  const h2 = hashField(Buffer.from("hello"));
  assert.equal(h1.toString("hex"), h2.toString("hex"));
});

test("S4-5 不同 kind 产生不同 semantic", () => {
  const usage = { input: 10, output: 5, cacheRead: 3, cacheWrite: 2, totalTokens: 20, cost: { total: 0.1 } };
  const e = makeEntry("a1", usage);
  const idA = piRequestIdentity(e, "assistant", usage, e.message as Record<string, unknown>);
  const idB = piRequestIdentity(e, "tool_result", usage, e.message as Record<string, unknown>);
  assert.notEqual(idA.semanticId, idB.semanticId);
});
