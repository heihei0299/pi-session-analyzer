/**
 * Ticket 05 — 自动刷新骨架（HTML 结构标记断言）。
 * 下拉 Off/5s/30s/5min 默认 Off、手动刷新按钮、「已更新 HH:MM:SS」状态元素。
 * 轮询/静默替换/变化提示行为按 spec 手工验收。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { startWebServer } from "../src/server.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

test("S16 自动刷新骨架标记：下拉 Off/5s/30s/5min 默认 Off / 刷新按钮 / 已更新状态元素", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const res = await fetch(server.url);
    assert.equal(res.status, 200);
    const body = await res.text();

    // 自动刷新下拉：Off/5s/30s/5min，默认 Off
    assert.match(body, /id="auto-refresh"/);
    assert.match(body, /<option value="off" selected>/);
    assert.match(body, /<option value="5000">/);
    assert.match(body, /<option value="30000">/);
    assert.match(body, /<option value="300000">/);
    assert.match(body, /Off/);
    assert.match(body, /5s/);
    assert.match(body, /30s/);
    assert.match(body, /5min/);

    // 手动「刷新」按钮
    assert.match(body, /id="refresh-btn"/);
    assert.match(body, />刷新</);

    // 「已更新 HH:MM:SS」状态元素（updated-at span 在状态行内）
    assert.match(body, /id="updated-at"/);
    assert.match(body, /已更新/);
    assert.match(body, /class="updated hidden"/);
  } finally {
    await server.close();
    removeFixture(dir);
  }
});
