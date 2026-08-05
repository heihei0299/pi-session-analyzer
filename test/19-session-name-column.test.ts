/**
 * Bugfix — 会话明细/请求明细新增「会话名称」列（第一列，可排序，截断+悬浮）。
 *
 * 背景：明细表无会话名称列，只有 sessionId（UUID，不直观）。需求（已共识）：
 * 会话名称 = displayName（重命名过的用文件名前缀，否则首条 user 消息，与会话管理一致）；
 * 置于第一列；点击列头可排序；超长截断 + hover 悬浮全文。
 * 数据源：/api/sessions 与 /api/requests 行均带 displayName（requests 为本次后端新增）。
 *
 * 断言（静态）：列定义第一项为 displayName；两个表头含 data-key="displayName"；
 * METRIC_FMTS.displayName 输出 name-cell（截断 + title 全量）。
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

test("明细表新增会话名称列：列定义第一项 + 表头 + 截断格式化", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);

    // 列定义：SESSION_COLS / REQUEST_COLS 第一项为 displayName
    const sessCols = body.match(/const SESSION_COLS = \[([^\]]+)\]/)?.[1] ?? "";
    assert.ok(sessCols.startsWith("\"displayName\""), "会话明细列定义第一项应为 displayName");
    const reqCols = body.match(/const REQUEST_COLS = \[([^\]]+)\]/)?.[1] ?? "";
    assert.ok(reqCols.startsWith("\"displayName\""), "请求明细列定义第一项应为 displayName");

    // 两个明细表头第一列：data-key="displayName" + 「会话名称」文本 + 排序标记
    const sessHead = body.match(/<table id="session-table">[\s\S]*?<thead>([\s\S]*?)<\/thead>/)?.[1] ?? "";
    const reqHead = body.match(/<table id="request-table">[\s\S]*?<thead>([\s\S]*?)<\/thead>/)?.[1] ?? "";
    for (const h of [sessHead, reqHead]) {
      assert.match(h, /<th class="sortable" data-key="displayName">会话名称 <span class="arrow"><\/span><\/th>/, "表头第一列应为可排序的会话名称列");
    }

    // METRIC_FMTS.displayName：截断 + title 悬浮（escapeHtml 转义）
    const fmt = body.match(/const METRIC_FMTS = \{([\s\S]*?)\n\}/)?.[1] ?? "";
    assert.match(fmt, /displayName: \(v\) =>/, "应有 displayName 格式化器");
    assert.match(fmt, /name-cell/, "格式化输出应带 name-cell 类（截断样式）");
    assert.match(fmt, /title="\$\{escapeHtml\(s\)\}"/, "应输出 title 悬浮全文（经 escapeHtml）");

    // CSS：name-cell 截断
    const css = body.match(/\.name-cell \{([\s\S]*?)\n\}/)?.[1] ?? "";
    assert.match(css, /text-overflow: ellipsis/, "name-cell 应省略号截断");
    assert.match(css, /white-space: nowrap/, "name-cell 应不换行");
  } finally {
    removeFixture(dir);
  }
});
