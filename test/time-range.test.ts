import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { makeSessionRange, makeMessageRange, applyTimeRange } from "../src/time-range.ts";
import type { SessionFileData } from "../src/session-data.ts";

// helper：构造最小 SessionFileData
function file(overrides: Partial<SessionFileData> & { items?: SessionFileData["items"] }): SessionFileData {
  return {
    sessionId: "s1",
    timestamp: "2026-08-10T10:00:00.000Z",
    cwd: "/proj",
    fileName: "2026-08-10T10-00-00_abc.jsonl",
    items: [],
    ...overrides,
  };
}
function item(timestamp: string, model = "m1"): SessionFileData["items"][number] {
  return { timestamp, model, usage: { input: 10, output: 5 } as unknown as import("../src/aggregate.ts").Usage };
}

describe("TimeRange 工厂校验", () => {
  test("非法 since 抛错（与 parseTimestamp 同文案）", () => {
    assert.throws(() => makeSessionRange({ since: "not-a-date" }), /无效时间/);
    assert.throws(() => makeMessageRange({ since: "2026-13-01" }), /无效时间/);
  });
  test("非法 until 抛错", () => {
    assert.throws(() => makeSessionRange({ until: "bad" }), /无效时间/);
    assert.throws(() => makeMessageRange({ until: "2026-02-30" }), /无效时间/);
  });
  test("空范围返回 null（无筛选语义）", () => {
    assert.equal(makeSessionRange({}), null);
    assert.equal(makeMessageRange({}), null);
    assert.equal(makeSessionRange({ since: undefined, until: undefined }), null);
  });
  test("纯日期 since 按本地 00:00，until 按本地 23:59:59.999（含端点）", () => {
    const noon = file({ timestamp: "2026-08-10T12:00:00.000Z", sessionId: "noon" });
    const outside = file({ timestamp: "2026-08-09T15:59:59.000Z", sessionId: "outside" });
    const range = makeSessionRange({ since: "2026-08-10", until: "2026-08-10" });
    assert.ok(range !== null);
    const out = applyTimeRange([noon, outside], range);
    assert.equal(out.length, 1);
    assert.equal(out[0].sessionId, "noon");
  });
});

describe("SessionTimeRange 批量过滤（header）", () => {
  test("按 header timestamp 闭区间，NaN header 保守保留", () => {
    const a = file({ timestamp: "2026-08-10T10:00:00.000Z", sessionId: "a" });
    const b = file({ timestamp: "2026-08-11T10:00:00.000Z", sessionId: "b" });
    const bad = file({ timestamp: "bad", sessionId: "bad" });
    const range = makeSessionRange({ since: "2026-08-10T09:00:00Z", until: "2026-08-10T11:00:00Z" });
    const out = applyTimeRange([a, b, bad], range);
    assert.equal(out.length, 2);
    assert.ok(out.some((f) => f.sessionId === "a"));
    assert.ok(out.some((f) => f.sessionId === "bad"));
  });
  test("空 range 不过滤", () => {
    const a = file({ sessionId: "a" });
    const b = file({ sessionId: "b" });
    assert.equal(applyTimeRange([a, b], null).length, 2);
  });
});

describe("MessageTimeRange 批量过滤（item）", () => {
  test("按 item timestamp 逐条闭区间，空 items 会话被剔除", () => {
    const s = file({
      sessionId: "s",
      items: [item("2026-08-10T09:00:00.000Z"), item("2026-08-10T11:00:00.000Z"), item("2026-08-11T10:00:00.000Z")],
    });
    const range = makeMessageRange({ since: "2026-08-10T10:00:00Z", until: "2026-08-10T12:00:00Z" });
    const out = applyTimeRange([s], range);
    assert.equal(out.length, 1);
    assert.equal(out[0].items.length, 1);
    assert.equal(out[0].items[0].timestamp, "2026-08-10T11:00:00.000Z");
  });
  test("NaN item timestamp 保守保留", () => {
    const s = file({ sessionId: "s", items: [item("bad"), item("2026-08-11T10:00:00.000Z")] });
    const range = makeMessageRange({ since: "2026-08-10T10:00:00Z", until: "2026-08-10T12:00:00Z" });
    const out = applyTimeRange([s], range);
    // bad 保留，11:00 剔除 → 仍剩 1 条 bad，会话不被剔除
    assert.equal(out.length, 1);
    assert.equal(out[0].items.length, 1);
    assert.equal(out[0].items[0].timestamp, "bad");
  });
  test("全部 item 被滤掉则会话整段剔除", () => {
    const s = file({ sessionId: "s", items: [item("2026-08-11T10:00:00.000Z")] });
    const range = makeMessageRange({ since: "2026-08-10T00:00:00Z", until: "2026-08-10T23:59:59Z" });
    const out = applyTimeRange([s], range);
    assert.equal(out.length, 0);
  });
});
