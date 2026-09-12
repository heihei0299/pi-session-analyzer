import { test } from "node:test";
import assert from "node:assert/strict";
import { cpSync, mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { startWebServer } from "../src/server.ts";

const EXPECTED = resolve("testdata/canonical/expected/pi-api.json");
const COST_EPSILON = 1e-9;

function sortRows(rows: Record<string, unknown>[], keys: string[]): Record<string, unknown>[] {
  return [...rows].sort((a, b) => keys.map((key) => String(a[key] ?? "").localeCompare(String(b[key] ?? ""))).find((n) => n !== 0) ?? 0);
}

// normalize 只锁业务字段：分页/服务端元字段不在契约内，避免无关抖动。
function normalize(path: string, body: Record<string, unknown>): unknown {
  if (path === "totals") {
    const { window, requests, input, output, cacheRead, cacheWrite, reasoning, totalTokens, cost, cacheRate } = body;
    return { window, requests, input, output, cacheRead, cacheWrite, reasoning, totalTokens, cost, cacheRate };
  }
  if (path === "meta") {
    const { sessionCount, dataRange, sources } = body;
    return { sessionCount, dataRange, sources };
  }
  const rows = body.rows as Record<string, unknown>[];
  const keys = path === "sessions" ? ["sessionId"] : path === "requests" ? ["timestamp", "sessionId", "model"] : path === "groups" ? ["model", "cwd"] : ["period"];
  return { ...body, rows: sortRows(rows, keys) };
}

function assertContract(actual: unknown, expected: unknown, path = "root"): void {
  if (typeof expected === "number" && typeof actual === "number" && (path.endsWith(".cost") || path.endsWith(".cacheRate"))) {
    assert.ok(Math.abs(actual - expected) <= COST_EPSILON, `${path}: ${actual} != ${expected} ± ${COST_EPSILON}`);
    return;
  }
  if (Array.isArray(expected)) {
    assert.ok(Array.isArray(actual), `${path} must be an array`);
    assert.equal(actual.length, expected.length, `${path}.length`);
    expected.forEach((value, index) => assertContract(actual[index], value, `${path}[${index}]`));
    return;
  }
  if (expected !== null && typeof expected === "object") {
    assert.ok(actual !== null && typeof actual === "object" && !Array.isArray(actual), `${path} must be an object`);
    assert.deepEqual(Object.keys(actual as object).sort(), Object.keys(expected).sort(), `${path} keys`);
    for (const [key, value] of Object.entries(expected)) assertContract((actual as Record<string, unknown>)[key], value, `${path}.${key}`);
    return;
  }
  assert.deepEqual(actual, expected, path);
}

test("canonical Pi fixture matches complete API golden", async () => {
  const dir = mkdtempSync(join(tmpdir(), "ta-canonical-pi-"));
  cpSync(resolve("testdata/canonical/pi"), dir, { recursive: true });
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const endpoints = {
      totals: "/api/totals",
      sessions: "/api/sessions?sortKey=sessionId&sortDir=asc",
      requests: "/api/requests?sortKey=timestamp&sortDir=asc",
      groups: "/api/groups?by=model",
      period: "/api/period?period=day",
      meta: "/api/meta",
    };
    const actual: Record<string, unknown> = {};
    for (const [name, endpoint] of Object.entries(endpoints)) {
      const response = await fetch(new URL(endpoint, server.url));
      assert.equal(response.status, 200, `${name} status`);
      actual[name] = normalize(name, await response.json() as Record<string, unknown>);
    }
    const expected = JSON.parse(readFileSync(EXPECTED, "utf8")) as unknown;
    assertContract(actual, expected);
  } finally {
    await server.close();
    rmSync(dir, { recursive: true, force: true });
  }
});
