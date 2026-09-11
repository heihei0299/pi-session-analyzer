/**
 * Ticket 03 — 分组键转义（消除注入点、避免双重转义）。
 * Ticket 07 — 自定义时间预设预填与切换隐藏。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import vm from "node:vm";

test("T03 总览分组表对未格式化列（分组键）统一经 escapeHtml 转义", () => {
  const html = readFileSync(join("src", "webui.html"), "utf8");
  const fnStart = html.indexOf("function renderTable(");
  assert.ok(fnStart >= 0, "应能定位 renderTable");
  const fnEnd = html.indexOf("\n}", fnStart) + 2;
  const renderTableSrc = html.slice(fnStart, fnEnd);

  // 校验源码实现契约：未格式化列必须经 escapeHtml
  assert.match(
    renderTableSrc,
    /escapeHtml\(v\s*\?\?\s*""\)/,
    "renderTable 对无格式化函数的列应使用 escapeHtml(v??'')"
  );

  // 在 VM 中提取 escapeHtml 与 renderTable 执行行为测试
  const escStart = html.indexOf("function escapeHtml(");
  const escEnd = html.indexOf("\n}", escStart) + 2;
  const escapeHtmlSrc = html.slice(escStart, escEnd);

  const context = vm.createContext({
    headEl: { innerHTML: "" },
    bodyEl: { innerHTML: "" },
  });
  vm.runInContext(escapeHtmlSrc + "\n" + renderTableSrc, context);

  const cols = [
    { key: "key", label: "分组键" },
    { key: "requests", label: "请求数" },
    { key: "cost", label: "花费" },
  ];
  const fmts = {
    cost: (v: unknown) => `$${Number(v).toFixed(2)}`,
  };

  // 1. 分组键含 & / < / > / " / ' 时应转义为实体
  const evilRows = [
    { key: '<script>alert("xss")</script> & \'model\'', requests: 12, cost: 1.5 },
  ];
  vm.runInContext(
    `renderTable(headEl, bodyEl, ${JSON.stringify(cols)}, ${JSON.stringify(evilRows)}, { cost: ${fmts.cost.toString()} });`,
    context
  );

  const bodyHTML: string = context.bodyEl.innerHTML;
  assert.match(
    bodyHTML,
    /<td>&lt;script&gt;alert\(&quot;xss&quot;\)&lt;\/script&gt; &amp; &#39;model&#39;<\/td>/,
    "特殊字符应转义为 HTML 实体，不应含有原始 HTML 标签"
  );
  assert.doesNotMatch(bodyHTML, /<script>/, "不应直接输出未转义标签");

  // 2. 正常分组键不受影响
  const normalRows = [{ key: "gpt-4o", requests: 5, cost: 0.25 }];
  vm.runInContext(
    `renderTable(headEl, bodyEl, ${JSON.stringify(cols)}, ${JSON.stringify(normalRows)}, { cost: ${fmts.cost.toString()} });`,
    context
  );
  assert.match(context.bodyEl.innerHTML, /<td>gpt-4o<\/td>/, "正常键应正常渲染");
});

test("T03 明细表与会话管理已做单层转义，不应出现双重转义实体", () => {
  const html = readFileSync(join("src", "webui.html"), "utf8");
  // 确保明细表与会话管理不出现类似 escapeHtml(escapeHtml(...)) 的双重调用
  assert.doesNotMatch(html, /escapeHtml\s*\(\s*escapeHtml/, "不应存在嵌套 escapeHtml");
});

test("T07 自定义预设预填与切换隐藏契约", () => {
  const html = readFileSync(join("src", "webui.html"), "utf8");
  const fnStart = html.indexOf("function applyPreset(");
  assert.ok(fnStart >= 0, "应能定位 applyPreset");
  const fnEnd = html.indexOf("\nfunction customRangeValue()", fnStart);
  assert.ok(fnEnd > fnStart, "应能定位 applyPreset 结尾");
  const applyPresetSrc = html.slice(fnStart, fnEnd);

  // 1. 切回非 custom 预设时隐藏 #custom-range，custom 时显示
  assert.match(
    applyPresetSrc,
    /\$\("#custom-range"\)\.classList\.toggle\("hidden",\s*preset\s*!==\s*"custom"\)/,
    "切回其他预设时自定义控件应隐藏，custom 时显示"
  );

  // 2. 有筛选时取 state.since/until，无筛选时回退到 lastMeta?.dataRange
  assert.match(
    applyPresetSrc,
    /state\.since\s*\?\?\s*\(lastMeta\?\.dataRange\?\.since\s*\?\?\s*null\)/,
    "自定义预设预填 since 应优先取当前 state.since，回退取 lastMeta.dataRange.since"
  );
  assert.match(
    applyPresetSrc,
    /state\.until\s*\?\?\s*\(lastMeta\?\.dataRange\?\.until\s*\?\?\s*null\)/,
    "自定义预设预填 until 应优先取当前 state.until，回退取 lastMeta.dataRange.until"
  );

  // 3. 预填后应调用 applyCustomRange() 即时生效，且立即 return 避免重复触发 refreshAll
  assert.match(
    applyPresetSrc,
    /applyCustomRange\(\);\s*return;/,
    "自定义预设分支应调用 applyCustomRange() 并立即 return 避免二次触发 refreshAll"
  );

  // 4. 6 个控件的 input/change 监听绑定
  assert.match(
    html,
    /for\s*\(\s*const id of \["since-date", "since-hour", "since-minute", "until-date", "until-hour", "until-minute"\]\)/,
    "应包含 6 个时间控件的统一事件绑定"
  );
});
