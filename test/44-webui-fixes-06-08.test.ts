/**
 * Ticket 06 — 轮询复用快照（总览页收敛至 totals+groups+meta，明细页仅当前页+total，消除重复请求）。
 * Ticket 08 — 状态行双值会话数（有筛选时「会话数: N（全量 M）」，无筛选时单值 M）。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import vm from "node:vm";

test("T08 状态行会话数骨架与标签标记", () => {
  const html = readFileSync(join("internal", "server", "webui.html"), "utf8");

  // 状态行应包含会话数专用 pill 与 id="meta-count"
  assert.match(
    html,
    /<span class="meta-pill" id="meta-sessions-pill">\s*会话数:\s*<strong id="meta-count">—<\/strong>\s*<\/span>/,
    "HTML 应包含带会话数标签的 meta-sessions-pill"
  );
});

test("T08 renderSessionCount 逻辑：有筛选双值 N（全量 M），无筛选单值 M", () => {
  const html = readFileSync(join("internal", "server", "webui.html"), "utf8");

  // 直接提取 webui.html 中的 hasTimeFilter 与 renderSessionCount 真实源码
  const startIdx = html.indexOf("function hasTimeFilter()");
  assert.ok(startIdx >= 0, "应能定位 hasTimeFilter");
  const endIdx = html.indexOf("async function updateFilteredSessionCount()", startIdx);
  assert.ok(endIdx > startIdx, "应能定位 renderSessionCount 结尾");
  const fnSrc = html.slice(startIdx, endIdx);

  const contextObj = {
    state: { since: null as string | null, until: null as string | null },
    lastMeta: { sessionCount: 231 },
    filteredSessionCount: null as number | null,
    cntEl: { textContent: "—" },
    $: (sel: string) => sel === "#meta-count" ? contextObj.cntEl : null,
  };

  const context = vm.createContext(contextObj);
  vm.runInContext(fnSrc, context);

  // 1. 无时间筛选时：保持单值 M
  context.renderSessionCount();
  assert.equal(context.cntEl.textContent, "231", "无筛选时显示单值 231");

  // 2. 有时间筛选但 filteredSessionCount 为 15：双值 15（全量 231）
  context.state.since = "2026-09-12";
  context.filteredSessionCount = 15;
  context.renderSessionCount();
  assert.equal(context.cntEl.textContent, "15（全量 231）", "有筛选时显示 15（全量 231）");

  // 3. 切换回全部预设（无筛选）：恢复单值 231
  context.state.since = null;
  context.state.until = null;
  context.filteredSessionCount = null;
  context.renderSessionCount();
  assert.equal(context.cntEl.textContent, "231", "切换全部后恢复单值 231");
});

test("T06 总览页轮询复用快照数据并收敛为 totals+groups+meta 最小请求集", () => {
  const html = readFileSync(join("internal", "server", "webui.html"), "utf8");

  // 1. poll 函数必须包含 snapshot 复用逻辑
  assert.match(
    html,
    /if\(state\.tab==="overview"\)\s*\{[\s\S]*?JSON\.parse\(snap\)[\s\S]*?renderCards\(totals\)[\s\S]*?renderGroupsData\(groups\)[\s\S]*?api\("meta"\)[\s\S]*?renderStatus\(meta\)/,
    "总览页在 snap 变化时必须复用快照 totals 与 groups 渲染，仅拉取 meta"
  );

  // 2. 无变化时不触发渲染（严格依赖 snap !== lastSnapshot）
  assert.match(
    html,
    /if\(snap!==lastSnapshot\)\s*\{/,
    "poll 必须在快照有变化时才执行更新"
  );

  // 3. 成功更新后刷新时间戳
  assert.match(
    html,
    /\$\("#updated-at"\)\.textContent="已更新 "\+pad2\(now\.getHours\(\)\)/,
    "更新时显示「已更新 HH:MM:SS」"
  );

  // 4. 明细页轮询保持仅当前页 + total 的语义
  assert.match(
    html,
    /else if\(state\.tab==="sessions" \|\| state\.tab==="requests"\)\s*\{[\s\S]*?fetchRows\(state\.tab\)[\s\S]*?renderDetailTable\(state\.tab\)/,
    "明细页仅拉取并渲染当前明细表"
  );

  // 5. snapshot 在总览页拉取 totals 与 groups，明细页仅拉取当前页分页数据
  assert.match(
    html,
    /if \(state\.tab === "overview"\) \{\s*const t = await api\("totals", params\);\s*const g = await fetchGroupsData\(\);\s*return JSON\.stringify\(\[t, g\]\);\s*\}/,
    "snapshot 在总览页拉取 totals 与 groups"
  );
});
