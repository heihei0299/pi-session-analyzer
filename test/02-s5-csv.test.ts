import { test } from "node:test";
import assert from "node:assert/strict";
import { runCli } from "../src/cli.ts";
import {
  makeFixture,
  removeFixture,
  sessionHeader,
  messageEntry,
  assistantUsage,
} from "./helpers.ts";

const SESSIONS = {
  "2026-07-31T01-00-00-000Z_a.jsonl": [
    sessionHeader({ id: "sess-a", timestamp: "2026-07-31T01:00:00.000Z", cwd: "/proj/a" }),
    messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 300 }) }),
  ],
  "2026-07-31T02-00-00-000Z_b.jsonl": [
    sessionHeader({ id: "sess-b", timestamp: "2026-07-31T02:00:00.000Z", cwd: "/proj/b" }),
    messageEntry({ role: "assistant", model: "m2", usage: assistantUsage({ input: 500 }) }),
  ],
};

function parseCsv(text: string): Record<string, string>[] {
  const lines = text.trim().split("\n");
  const headers = lines[0].split(",");
  return lines.slice(1).map((line) => {
    const values = line.split(",");
    const row: Record<string, string> = {};
    headers.forEach((h, i) => {
      row[h] = values[i];
    });
    return row;
  });
}

test("S5 CSV 输出：--format csv 表头+数据行可解析、字段与表格一一对应", async () => {
  const dir = makeFixture(SESSIONS);
  try {
    // totals：单行
    const totals = parseCsv(await runCli(["totals", "--dir", dir, "--format", "csv"]));
    assert.equal(totals.length, 1);
    assert.equal(totals[0].requests, "3");
    assert.equal(totals[0].input, "900");
    assert.equal(totals[0].cacheRate, (600 / (900 + 600 + 30)).toString());

    // sessions：两行，字段含 sessionId/timestamp/cwd/model
    const sessions = parseCsv(await runCli(["sessions", "--dir", dir, "--format", "csv"]));
    assert.equal(sessions.length, 2);
    const a = sessions.find((r) => r.sessionId === "sess-a");
    assert.ok(a, "会话 A 行存在");
    assert.equal(a!.timestamp, "2026-07-31T01:00:00.000Z");
    assert.equal(a!.cwd, "/proj/a");
    assert.equal(a!.model, "m1");
    assert.equal(a!.requests, "2");
    assert.equal(a!.input, "400");

    // requests：三行逐消息
    const requests = parseCsv(await runCli(["requests", "--dir", dir, "--format", "csv"]));
    assert.equal(requests.length, 3);
    const r300 = requests.find((r) => r.input === "300");
    assert.ok(r300);
    assert.equal(r300!.sessionId, "sess-a");
    assert.equal(r300!.requests, "1");

    // 表头字段与 JSON 字段一致（驼峰）——sessions 窗口
    const json = JSON.parse(await runCli(["sessions", "--dir", dir, "--format", "json"]));
    const jsonKeys = Object.keys(json.rows[0]).sort();
    const csvKeys = Object.keys(sessions[0]).sort();
    assert.deepEqual(csvKeys, jsonKeys, "CSV 表头应与 JSON 字段一一对应");

    // totals 窗口同样一致（JSON 含 window 键，CSV 也应有 window 列）
    const jsonTotals = JSON.parse(await runCli(["totals", "--dir", dir, "--format", "json"]));
    const jsonTotalsKeys = Object.keys(jsonTotals).sort();
    const csvTotalsKeys = Object.keys(totals[0]).sort();
    assert.deepEqual(csvTotalsKeys, jsonTotalsKeys, "totals 窗口 CSV 表头应与 JSON 字段一致（含 window）");
  } finally {
    removeFixture(dir);
  }
});
