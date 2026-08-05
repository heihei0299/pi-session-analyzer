/**
 * Ticket 04 — 明细视图骨架（HTML 结构标记断言）。
 * 会话明细 10 列、请求明细 11 列、分页 20/50/100、列头排序标记、请求明细默认时间倒序、tab 切换拉取端点。
 * 分页/排序/tab 切换交互按 spec 手工验收。
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

test("S15 明细 tab 骨架标记：表头列数 / 排序标记 / 默认倒序 / 分页 / tab 切换端点", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);

    // 会话明细表头 10 列：会话ID/时间/cwd/模型/请求数/输入/输出/缓存率/总token/花费
    const sessHead = body.match(/<tr id="session-head"[^>]*>([\s\S]*?)<\/tr>/)?.[1] ?? "";
    assert.equal((sessHead.match(/<th/g) ?? []).length, 10, "会话明细表头应为 10 列");
    for (const label of ["会话ID", "时间", "cwd", "模型", "请求数", "输入", "输出", "缓存率", "总token", "花费"]) {
      assert.match(sessHead, new RegExp(`>${label} <span`), `会话明细表头应含列 ${label}`);
    }
    // 请求明细表头 10 列：会话ID/时间/模型/输入/输出/缓存/推理/缓存率/总token/花费
    const reqHead = body.match(/<tr id="request-head"[^>]*>([\s\S]*?)<\/tr>/)?.[1] ?? "";
    assert.equal((reqHead.match(/<th/g) ?? []).length, 10, "请求明细表头应为 10 列（spec 列清单）");
    for (const label of ["会话ID", "时间", "模型", "输入", "输出", "缓存", "推理", "缓存率", "总token", "花费"]) {
      assert.match(reqHead, new RegExp(`>${label} <span`), `请求明细表头应含列 ${label}`);
    }

    // 列头排序标记（sortable + 箭头占位）
    assert.match(body, /class="sortable"/);
    assert.match(body, /class="arrow"/);

    // 请求明细默认时间倒序声明
    assert.match(body, /data-sort="timestamp"/);
    assert.match(body, /data-dir="desc"/);

    // 分页下拉含 20/50/100
    assert.match(body, /<option value="20" selected>20<\/option>/);
    assert.match(body, /<option value="50">50<\/option>/);
    assert.match(body, /<option value="100">100<\/option>/);

    // tab 切换拉取对应端点（fetch 端点串出现在 HTML）
    assert.match(body, /fetchRows\("sessions"\)/);
    assert.match(body, /fetchRows\("requests"\)/);
  } finally {
    removeFixture(dir);
  }
});
