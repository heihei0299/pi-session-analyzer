/**
 * Ticket 04 — 数字显示打磨（十亿 B 单位、表头死属性清理）。
 * Ticket 05 — 导出进行中反馈（按钮禁用与文案切换、异常恢复）。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import vm from "node:vm";

test("T04 fmtCompact 紧凑数字十亿档显示 B 单位且去尾 0，其它档位保持不变", () => {
  const html = readFileSync(join("internal", "server", "webui.html"), "utf8");
  const fnStart = html.indexOf("function fmtCompact(");
  assert.ok(fnStart >= 0, "应能定位 fmtCompact");
  const fnEnd = html.indexOf("\n", fnStart);
  const fnSrc = html.slice(fnStart, fnEnd);

  const context = vm.createContext({});
  vm.runInContext(fnSrc, context);
  const fmtCompact = context.fmtCompact as (n: number) => string;

  // 1. ≥ 10 亿显示 B 单位
  assert.equal(fmtCompact(1_000_000_000), "1B", "10 亿整去尾 0 为 1B");
  assert.equal(fmtCompact(1_400_000_000), "1.4B", "14 亿保留一位小数 1.4B");
  assert.equal(fmtCompact(1_415_600_000), "1.4B", "14.156 亿四舍五入保留一位小数 1.4B");
  assert.equal(fmtCompact(13_900_000_000), "13.9B", "139 亿保留一位小数 13.9B");

  // 2. 百万 M 档保持不变
  assert.equal(fmtCompact(1_000_000), "1M", "100 万去尾 0 为 1M");
  assert.equal(fmtCompact(999_999_999), "1000M", "10 亿以下仍走 M 档");
  assert.equal(fmtCompact(123_456_789), "123.5M", "百万档保留一位小数");

  // 3. 千 k 档保持不变
  assert.equal(fmtCompact(10_000), "10k", "1 万为 10k");
  assert.equal(fmtCompact(999_000), "999k", "万到百万走 k 档");

  // 4. 1 万以下千分位
  assert.equal(fmtCompact(9_999), "9,999", "1 万以下使用千分位");
  assert.equal(fmtCompact(0), "0", "0 使用千分位");
});

test("T04 请求明细表头移除无效的 data-sort/data-dir 静态属性", () => {
  const html = readFileSync(join("internal", "server", "webui.html"), "utf8");

  // request-head 不应再包含 data-sort 或 data-dir 静态属性
  assert.doesNotMatch(
    html,
    /<tr id="request-head"[^>]*data-sort/,
    "request-head 表头不应包含 data-sort 属性"
  );
  assert.doesNotMatch(
    html,
    /<tr id="request-head"[^>]*data-dir/,
    "request-head 表头不应包含 data-dir 属性"
  );

  // 默认排序状态应由 detailState 承载
  assert.match(
    html,
    /requests:\s*\{[^}]*sortKey:\s*"timestamp",\s*sortDir:\s*"desc"/,
    "detailState.requests 仍承载默认时间倒序"
  );
});

test("T05 exportData 导出触发瞬间按钮禁用并显示导出中，完成后恢复", () => {
  const html = readFileSync(join("internal", "server", "webui.html"), "utf8");
  const fnStart = html.indexOf("async function exportData(");
  assert.ok(fnStart >= 0, "应能定位 exportData");
  const fnEnd = html.indexOf("\nfunction downloadBlob(", fnStart);
  assert.ok(fnEnd > fnStart, "应能定位 exportData 结尾");
  const fnSrc = html.slice(fnStart, fnEnd);

  // 1. 必须获取两个导出按钮并切换状态
  assert.match(fnSrc, /#export-json/, "exportData 应引用 #export-json 按钮");
  assert.match(fnSrc, /#export-csv/, "exportData 应引用 #export-csv 按钮");
  assert.match(fnSrc, /导出中…/, "导出时按钮文案应切换为 '导出中…'");

  // 2. 导出结束时在 finally 中恢复按钮状态
  assert.match(fnSrc, /finally\s*\{/, "exportData 必须包含 finally 块以恢复按钮状态");

  // 3. 错误时调用 showError
  assert.match(fnSrc, /catch\s*\(\s*\w+\s*\)\s*\{\s*showError\(/, "导出异常时应走 showError 提示");
});
