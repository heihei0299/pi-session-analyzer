/**
 * Bugfix — 明细视图 id 引用一致性回归测试。
 *
 * 背景：renderDetailTable / bindDetailInteractions 用 kind（sessions/requests，复数）拼接
 * `"#" + kind + "-head"` 等选择器，但 HTML 静态 id 是单数（session-head/session-body/...），
 * 导致 bodyEl 为 null → 「明细加载失败: Cannot set properties of null (setting 'innerHTML')」，
 * 且 init() 在 bindDetailInteractions 绑定阶段即中断（初始 refreshAll() 未执行，总览也空）。
 *
 * 断言：JS 动态拼接引用的每个 id 都必须命中 HTML 静态 id 定义。
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

test("明细视图动态 id 引用全部命中 HTML 静态 id（单复数不匹配回归）", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);

    // 静态 id 全集
    const staticIds = new Set([...body.matchAll(/id="([^"]+)"/g)].map((m) => m[1]));

    // JS 的 DETAIL_ID 映射：复数 kind → 单数 HTML id 前缀
    const mapMatch = body.match(/const DETAIL_ID = \{ ([^}]+) \};/);
    assert.ok(mapMatch, "JS 应定义 DETAIL_ID 映射（复数 kind → 单数 id 前缀）");
    const prefixById = Object.fromEntries([...mapMatch![1].matchAll(/(\w+): "(\w+)"/g)].map((m) => [m[1], m[2]]));
    assert.deepEqual(Object.keys(prefixById).sort(), ["requests", "sessions"]);

    // 动态拼接后缀：$("#" + DETAIL_ID[kind] + "-<suffix>")
    const suffixes = [...body.matchAll(/\$\("#" \+ DETAIL_ID\[kind\] \+ "-([a-z-]+)"\)/g)].map((m) => m[1]);
    assert.ok(suffixes.length > 0, "JS 应通过 DETAIL_ID[kind] 动态拼接明细 id");
    assert.deepEqual([...new Set(suffixes)].sort(), ["body", "head", "page-info", "page-size"]);

    // 每个 kind 的前缀 + 后缀组合必须命中静态 id
    for (const kind of Object.keys(prefixById)) {
      for (const suffix of suffixes) {
        const id = `${prefixById[kind]}-${suffix}`;
        assert.ok(staticIds.has(id), `HTML 应静态定义 id="${id}"（${kind} 视图的 ${suffix}）`);
      }
    }
  } finally {
    removeFixture(dir);
  }
});
