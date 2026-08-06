/**
 * Ticket 03 — 总览视图骨架（HTML 结构标记断言）。
 * 前端 JS 行为（卡片渲染/k-M 缩写/分组切换/预设/导出/错误提示）按 spec 手工验收，
 * 自动化只断言 HTML 骨架标记（spec 明示）。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { startWebServer } from "../src/server.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

async function fetchHtml(dir: string): Promise<string> {
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const res = await fetch(server.url);
    assert.equal(res.status, 200);
    return await res.text();
  } finally {
    await server.close();
  }
}

test("S13 总览骨架标记：8 卡片 / 分组切换 / 时间预设 / 状态行 / 导出按钮 / 错误横幅 / 深色主题", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);

    // 8 张汇总卡片占位
    for (const id of [
      "card-requests",
      "card-input",
      "card-output",
      "card-cache",
      "card-reasoning",
      "card-cache-rate",
      "card-total-tokens",
      "card-cost",
    ]) {
      assert.match(body, new RegExp(`id="${id}"`), `HTML 应含卡片占位 ${id}`);
    }

    // 分组切换控件（按模型 / 按 cwd）
    assert.match(body, /data-group="model"/);
    assert.match(body, /data-group="cwd"/);

    // 时间预设按钮（今天/7天/30天/自8/1/全部/自定义），「自 8/1」默认激活（网关可比窗口）
    for (const preset of ["today", "7d", "30d", "gateway", "all", "custom"]) {
      assert.match(body, new RegExp(`data-preset="${preset}"`), `HTML 应含时间预设 ${preset}`);
    }
    assert.match(body, /data-preset="gateway"[^>]*class="active"/, "「自 8/1」应默认激活");
    assert.doesNotMatch(body, /data-preset="all"[^>]*class="active"/, "「全部」不应默认激活");

    // 状态行 / 导出按钮 / 错误横幅
    assert.match(body, /id="status-line"/);
    assert.match(body, /id="export-json"/);
    assert.match(body, /id="export-csv"/);
    assert.match(body, /id="error-banner"/);

    // 深色主题（zinc 暗色系，data-theme 标记或 CSS 变量）
    assert.match(body, /data-theme="dark"/);
  } finally {
    removeFixture(dir);
  }
});

test("S14 tab 骨架：4 个 tab（总览/会话明细/请求明细/会话管理）", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);
    for (const tab of ["overview", "sessions", "requests", "manage"]) {
      assert.match(body, new RegExp(`data-tab="${tab}"`), `HTML 应含 tab ${tab}`);
    }
    assert.match(body, /总览/);
    assert.match(body, /会话明细/);
    assert.match(body, /请求明细/);
    assert.match(body, /会话管理/);
  } finally {
    removeFixture(dir);
  }
});
