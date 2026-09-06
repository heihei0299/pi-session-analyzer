/**
 * 06 — 费用回算
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { costForRecord } from "../src/cost/calculator.ts";

test("S6-1 reported 有值则用 reported", () => {
  const usage = { input: 10, output: 5, cacheRead: 3, cacheWrite: 2, cost: { total: 0.1 } };
  const pricing = { inputCostPerMillion: "1", outputCostPerMillion: "2", cacheReadCostPerMillion: "0.5", cacheCreationCostPerMillion: "1" };
  const cost = costForRecord(usage, pricing);
  assert.equal(cost, 0.1);
});

test("S6-2 reported 为 0 但 pricing 有 → 回算", () => {
  const usage = { input: 1000000, output: 0, cacheRead: 0, cacheWrite: 0, cost: { total: 0 } };
  const pricing = { inputCostPerMillion: "1", outputCostPerMillion: "2", cacheReadCostPerMillion: "0.5", cacheCreationCostPerMillion: "1" };
  const cost = costForRecord(usage, pricing);
  // 1M * $1 /1M = $1
  assert.equal(cost, 1);
});

test("S6-3 无 reported 无 pricing → 0", () => {
  const usage = { input: 10, output: 5, cacheRead: 3, cacheWrite: 2, cost: { total: 0 } };
  const cost = costForRecord(usage, null);
  assert.equal(cost, 0);
});

test("S6-4 cache 计费回算", () => {
  const usage = { input: 0, output: 0, cacheRead: 1000000, cacheWrite: 1000000, cost: { total: 0 } };
  const pricing = { inputCostPerMillion: "0", outputCostPerMillion: "0", cacheReadCostPerMillion: "0.5", cacheCreationCostPerMillion: "1" };
  const cost = costForRecord(usage, pricing);
  assert.equal(cost, 1.5);
});
