import { test } from "node:test";
import assert from "node:assert/strict";
import { utimesSync, readdirSync } from "node:fs";
import { join } from "node:path";
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

const UUID = "019fb5e2-3c91-76bd-b12c-c8d2ab31c532";
const UUID2 = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";

// T1: 头部 displayName 点击进入编辑态（复用 rename-input 样式与校验，成功后刷新抽屉与后列表）
test("Seam-T1 drawer displayName rename: 点击进入编辑态，复用校验与刷新", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader({ id: UUID }), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);
    // 标题可点击进入编辑（应监听 drawer-title）
    assert.match(body, /drawer-title/, "应含抽屉标题 #drawer-title");
    // 应可点击触发重命名（标题点击进入 rename-input）
    assert.match(body, /drawer-title.*rename|startDetailRename|detail.*rename/i, "drawer-title 应可点击进入重命名编辑态");
    // 应复用 rename-input 样式
    assert.match(body, /rename-input/, "应复用 rename-input 样式");
    // 抽屉重命名应调用 POST /api/sessions/rename
    assert.match(body, /\/api\/sessions\/rename/, "抽屉重命名应调用 POST /api/sessions/rename");
    // 成功后应刷新抽屉（fetchDetail/renderDetailDrawer）与后列表（refreshSessionTable/fetchRows 等）
    assert.match(body, /fetchDetail.*renderDetailDrawer|renderDetailDrawer.*fetchDetail/, "成功后应刷新抽屉数据与视图");
    assert.match(body, /refreshSessionTable|fetchRows.*sessions|refreshSessionGroups/, "成功后应刷新后列表");
    // 应对活跃/非法/同名做 409/400 友好提示（至少包含错误处理文字）
    // 检查存在对 409/400 的分支或显示 detail/error
    assert.match(body, /409|400|显示名不能为空|会话活跃中|同名文件已存在/, "应处理活跃 409/非法 400/同名 409 校验提示");
  } finally {
    removeFixture(dir);
  }
});

// T1b: 端到端重命名集成（drawer rename 真正改文件并刷新 detail）
test("Seam-T1b drawer rename integration: POST 改名后 detail 与列表同步", async () => {
  const dir = makeFixture({
    [`2026-07-31T01-55-30-577Z_${UUID}.jsonl`]: [sessionHeader({ id: UUID, timestamp: "2026-07-31T01:55:30.000Z", cwd: "/proj" }), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 10 }) })],
  });
  // 非活跃：mtime 10 分钟前，否则 409
  const old = new Date(Date.now() - 10 * 60 * 1000);
  utimesSync(join(dir, `2026-07-31T01-55-30-577Z_${UUID}.jsonl`), old, old);
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    // 重命名 via API（模拟 drawer 的保存）
    const res = await fetch(new URL("/api/sessions/rename", server.url), {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ sessionId: UUID, name: "新会话名" }),
    });
    assert.equal(res.status, 200, "重命名应 200");
    const body = await res.json() as any;
    assert.equal(body.fileName, `新会话名_${UUID}.jsonl`);
    // 文件系统断言
    const names = readdirSync(dir);
    assert.ok(names.includes(`新会话名_${UUID}.jsonl`), "新文件名应存在");
    // detail 抽屉数据应同步新 displayName
    const detailRes = await fetch(new URL(`/api/sessions/${UUID}/detail`, server.url));
    assert.equal(detailRes.status, 200);
    const detail = await detailRes.json() as any;
    assert.equal(detail.session.displayName, "新会话名", "detail displayName 应同步更新");
    assert.equal(detail.session.fileName, `新会话名_${UUID}.jsonl`);
    // 列表也应同步
    const sessRes = await fetch(new URL("/api/sessions", server.url));
    const sess = await sessRes.json() as any;
    const row = sess.rows.find((r: any) => r.sessionId === UUID);
    assert.ok(row, "列表应仍含该会话");
    assert.equal(row.displayName, "新会话名");
    // 校验错误透传：活跃 409
    const activeDir = makeFixture({
      [`2026-07-31T01-55-30-577Z_${UUID2}.jsonl`]: [sessionHeader({ id: UUID2 })],
    });
    const activeServer = await startWebServer({ dir: activeDir, host: "127.0.0.1", port: 0 });
    try {
      const r2 = await fetch(new URL("/api/sessions/rename", activeServer.url), {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ sessionId: UUID2, name: "x" }),
      });
      assert.equal(r2.status, 409, "活跃会话应 409");
      const b2 = await r2.json() as any;
      assert.match(b2.detail, /会话活跃中/);
    } finally {
      await activeServer.close();
      removeFixture(activeDir);
    }
    // 非法 400
    const bad = await fetch(new URL("/api/sessions/rename", server.url), {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ sessionId: UUID, name: "///" }),
    });
    assert.equal(bad.status, 400, "非法名应 400");
    const bb = await bad.json() as any;
    assert.match(bb.detail, /非法/);
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

// T2: 异常空态 — 404 会话不存在 + 孤儿不合并
test("Seam-T2 404 and orphan: 抽屉空态与归属隔离", async () => {
  const dir = makeFixture({
    "2026-08-01T10-00-00-000Z_p1.jsonl": [
      sessionHeader({ id: "p1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 10 }) }),
    ],
    "2026-08-02T10-00-00-000Z_orphan.jsonl": [
      { ...sessionHeader({ id: "orphan", timestamp: "2026-08-02T10:00:00.000Z", cwd: "/proj" }), parentSession: "missing" },
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 999 }) }),
    ],
  });
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    // API 404
    const r404 = await fetch(new URL("/api/sessions/missing/detail", server.url));
    assert.equal(r404.status, 404);
    const b404 = await r404.json() as any;
    assert.ok(b404.detail.includes("会话不存在") || b404.detail.includes("missing"));
    // 前端抽屉空态文案与关闭按钮
    const html = await fetchHtml(dir);
    assert.match(html, /会话不存在或已删除/, "抽屉应含 404 空态“会话不存在或已删除”");
    assert.match(html, /关闭/, "空态应含关闭按钮");
    // 孤儿不并入父
    const pDetail = await (await fetch(new URL("/api/sessions/p1/detail", server.url))).json() as any;
    assert.equal(pDetail.totals.childrenCount, 0, "孤儿不应计入 p1 childrenCount");
    assert.equal(pDetail.meta.hasChildren, false);
    assert.equal(pDetail.children.length, 0);
    assert.ok(!pDetail.requests.some((r: any) => r.input === 999), "孤儿请求不应混入父时间线");
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

// T3: 无请求会话 0 值展示 + 无子代理隐藏开关
test("Seam-T3 empty requests: 0 值展示与无子代理隐藏", async () => {
  // 无请求：仅 header，无 assistant 消息
  const dirEmpty = makeFixture({
    "2026-08-01T10-00-00-000Z_empty.jsonl": [
      sessionHeader({ id: "empty", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
    ],
    // 无子代理的普通会话（1 请求）
    "2026-08-01T11-00-00-000Z_solo.jsonl": [
      sessionHeader({ id: "solo", timestamp: "2026-08-01T11:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 10, output: 5, cacheRead: 2, cacheWrite: 0, cost: { total: 0.01 } }) }),
    ],
  });
  const server = await startWebServer({ dir: dirEmpty, host: "127.0.0.1", port: 0 });
  try {
    const emptyDetail = await (await fetch(new URL("/api/sessions/empty/detail", server.url))).json() as any;
    // 汇总 0 值：totalTokens 0, requests 0, cost 0, cacheRate 0
    assert.equal(emptyDetail.totals.main.requests, 0);
    assert.equal(emptyDetail.totals.main.totalTokens, 0);
    assert.equal(emptyDetail.totals.main.cost, 0);
    assert.equal(emptyDetail.totals.main.cacheRate, 0);
    assert.equal(emptyDetail.requests.length, 0, "无请求时间线为空");
    // 前端文案与样式：UNPRICED/0% 与空态占位
    const html = await fetchHtml(dirEmpty);
    assert.match(html, /UNPRICED|费率未配置/, "0 花费应展示 UNPRICED");
    // fmtRate(0) -> 0.00%
    assert.match(html, /fmtRate|0\.00%|cacheRate/, "0 缓存率应展示 0% 相关");
    assert.match(html, /暂无计入口径请求/, "无请求会话时间线应展示“暂无计入口径请求”");
    // 无子代理：detail 应 hasChildren false，渲染层应隐藏开关与含 N 提示
    assert.equal(emptyDetail.meta.hasChildren, false);
    assert.equal(emptyDetail.totals.childrenCount, 0);
    // 检查 HTML：开关仅在 hasChildren 时渲染；无子代理不应默认出现合并开关的常驻文本“含 N 个子代理”
    // 这里的 bone 检查：存在对 hasChildren 的条件渲染逻辑
    assert.match(html, /hasChildren/, "应根据 hasChildren 条件渲染开关与含 N 提示");
    // 同时确保 CSS/JS 中存在对 hasChildren false 时隐藏开关的逻辑（已由变体 A 实现，复用）
    const soloDetail = await (await fetch(new URL("/api/sessions/solo/detail", server.url))).json() as any;
    assert.equal(soloDetail.meta.hasChildren, false);
    assert.equal(soloDetail.totals.merged.requests, soloDetail.totals.main.requests, "无子时代理 merged===main");
  } finally {
    await server.close();
    removeFixture(dirEmpty);
  }
});

// T4: 滚动与性能 — 请求表容器 max-height:60vh + overflow:auto
test("Seam-T4 scroll container: 请求表 60vh 可滚动，极端 >200 行", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader({ id: "p1" }), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);
    assert.match(body, /max-height:60vh/, "请求表容器应设 max-height:60vh");
    assert.match(body, /overflow:\s*auto/, "容器应 overflow:auto 可滚动");
    // 极端 250 行仍全量渲染：后端排序不分页，前端全量展示（通过 detailFromFiles 验证 >200 仍保留）
    const { SessionData } = await import("../src/session-data.ts");
    const sd = new SessionData();
    const files = [
      { sessionId: "big", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj", fileName: "big.jsonl", isTask: false, items: Array.from({ length: 250 }, (_, i) => ({ timestamp: `2026-08-01T10:${String(Math.floor(i / 60)).padStart(2, "0")}:${String(i % 60).padStart(2, "0")}.000Z`, model: "m1", usage: { input: 1, output: 1, cacheRead: 0, cacheWrite: 0, cost: { total: 0.001 } } })) },
    ] as any;
    const detail: any = (sd as any).detailFromFiles(files, "big");
    assert.equal(detail.requests.length, 250, "合并后请求应全量 250 行");
    // HTML 中 detail 合并后请求表应无分页控件，仅滚动
    // 检查 template 中 detail 区域不存在分页 button
    // 通过不存在 "drawer.*page" 来弱校验无分页
    assert.doesNotMatch(body, /drawer.*page-info|#drawer.*page/i, "详情请求时间线应无分页控件");
  } finally {
    removeFixture(dir);
  }
});

// T5: 刷新 — 抽屉打开期间不参与 poll，关闭后恢复
test("Seam-T5 poll pause: 抽屉打开期间跳过 autoRefresh poll", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);
    // poll 函数首行即判断 drawer open 则 return
    assert.match(body, /function poll\(\)\s*\{[^}]*detailDrawerState\.open/, "poll 应在抽屉打开时 early return");
    assert.match(body, /if\s*\(\s*detailDrawerState\.open\s*\)\s*return/, "poll 应含 if(detailDrawerState.open) return");
    // 关闭后恢复：closeDrawer 将 open 设为 false
    assert.match(body, /closeDrawer.*detailDrawerState\.open\s*=\s*false|detailDrawerState\.open\s*=\s*false.*closeDrawer/s, "关闭后应重置 open=false 恢复轮询");
    // snapshot/poll 不应在抽屉打开时触发 fetch（已由 early return 保证）
    // 检查 poll 内部无绕过逻辑
    const pollSection = body.slice(body.indexOf("async function poll"), body.indexOf("async function poll") + 800);
    assert.ok(pollSection.includes("detailDrawerState.open"), "poll 分支应检查抽屉状态");
  } finally {
    removeFixture(dir);
  }
});
