/**
 * Bugfix — 明细表翻页键（问题汇总 #1）；ticket 21 起翻页走服务端分页。
 *
 * 背景：会话明细/请求明细只有「每页行数」下拉 + 页码文本，无上一页/下一页按钮，
 * 用户无法翻页（spec 只要求分页下拉，翻页键为用户反馈补充）。
 * 修复：两个 pager 各加「上一页」「下一页」按钮；bindDetailInteractions 绑定
 * prev/next 更新 detailState.page；renderDetailTable 首页禁 prev、末页禁 next。
 * 自 ticket 21（服务端分页）起：prev/next/改页大小/排序均重新 fetch（fetchRows 传
 * page/size/sortKey/sortDir），total 来自响应而非前端 rows.length。
 *
 * 断言（静态标记，与 08/09 骨架断言同风格）：按钮存在、文本正确、JS 绑定与禁用逻辑齐全。
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

test("明细 pager 含上一页/下一页按钮且 JS 绑定完整", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);

    // 两个 pager 各 4 个控件（size 下拉 + 上一页 + 页码 + 下一页）
    for (const kind of ["session", "request"]) {
      for (const id of [`${kind}-prev`, `${kind}-next`, `${kind}-page-size`, `${kind}-page-info`]) {
        assert.match(body, new RegExp(`id="${id}"`), `HTML 应含 ${id}`);
      }
    }
    assert.match(body, />上一页</, "应含「上一页」按钮文本");
    assert.match(body, />下一页</, "应含「下一页」按钮文本");

    // JS：prev/next 点击绑定 + page 增减 + 重新 fetch（服务端分页，见 ticket 21）
    assert.match(body, /\$\("#" \+ DETAIL_ID\[kind\] \+ "-prev"\)\.addEventListener\("click"/, "应绑定 prev 点击");
    assert.match(body, /\$\("#" \+ DETAIL_ID\[kind\] \+ "-next"\)\.addEventListener\("click"/, "应绑定 next 点击");
    assert.match(body, /if \(st\.page > 1\) \{ st\.page--; reload\(\(\) => \{\}\); \}/, "prev 应减页并重新拉取");
    assert.match(body, /if \(st\.page < pages\) \{ st\.page\+\+; reload\(\(\) => \{\}\); \}/, "next 应加页并重新拉取");
    assert.match(body, /const pages = Math\.max\(1, Math\.ceil\(st\.total \/ st\.size\)\);/, "next 按 total 计算总页数");

    // JS：服务端分页参数构造（page/size/sortKey/sortDir）
    assert.match(body, /params\.set\("page", String\(st\.page\)\);/, "fetchRows 应传 page 参数");
    assert.match(body, /params\.set\("size", String\(st\.size\)\);/, "fetchRows 应传 size 参数");
    assert.match(body, /if \(st\.sortKey\) \{ params\.set\("sortKey", st\.sortKey\); params\.set\("sortDir", st\.sortDir\); \}/, "fetchRows 应传排序参数");

    // JS：首页/末页禁用状态
    assert.match(body, /\$\("#" \+ DETAIL_ID\[kind\] \+ "-prev"\)\.disabled = st\.page <= 1;/, "首页应禁用 prev");
    assert.match(body, /\$\("#" \+ DETAIL_ID\[kind\] \+ "-next"\)\.disabled = st\.page >= pages;/, "末页应禁用 next");
  } finally {
    removeFixture(dir);
  }
});
