/**
 * 终端表格渲染：总消耗量窗口。
 * 列：请求数 / 输入 / 输出 / 缓存读 / 缓存写 / 推理 / 总 token / 花费 / 缓存率
 */
import type { Totals } from "./aggregate.ts";

const COLUMNS: { header: string; width: number }[] = [
  { header: "请求数", width: 8 },
  { header: "输入", width: 12 },
  { header: "输出", width: 12 },
  { header: "缓存读", width: 12 },
  { header: "缓存写", width: 12 },
  { header: "推理", width: 12 },
  { header: "总 token", width: 14 },
  { header: "花费", width: 12 },
  { header: "缓存率", width: 10 },
];

export function renderTotalsTable(totals: Totals): string {
  const rows: string[][] = [
    COLUMNS.map((c) => c.header),
    [
      String(totals.requests),
      formatTokens(totals.input),
      formatTokens(totals.output),
      formatTokens(totals.cacheRead),
      formatTokens(totals.cacheWrite),
      formatTokens(totals.reasoning),
      formatTokens(totals.totalTokens),
      formatCost(totals.cost),
      formatRate(totals.cacheRate),
    ],
  ];

  // 列宽按终端显示宽度（CJK 字符计 2）计算，保证中文表头与数字对齐
  const widths = COLUMNS.map((c, i) =>
    Math.max(c.width, ...rows.map((r) => displayWidth(r[i]))),
  );

  return (
    rows
      .map((row) =>
        row.map((cell, i) => padWidth(cell, widths[i])).join("  "),
      )
      .join("\n") + "\n"
  );
}

function padWidth(text: string, width: number): string {
  const diff = width - displayWidth(text);
  return diff > 0 ? " ".repeat(diff) + text : text;
}

/** 终端显示宽度：CJK / 全角字符计 2，其余计 1 */
function displayWidth(s: string): number {
  let w = 0;
  for (const ch of s) {
    w += /[\u1100-\u115f\u2e80-\ua4cf\uac00-\ud7a3\uf900-\ufaff\ufe30-\ufe4f\uff00-\uff60\uffe0-\uffe6]/.test(ch)
      ? 2
      : 1;
  }
  return w;
}

function formatTokens(n: number): string {
  return n.toLocaleString("en-US");
}

function formatCost(n: number): string {
  return n.toLocaleString("en-US", { maximumFractionDigits: 6 });
}

function formatRate(r: number): string {
  return `${(r * 100).toFixed(2)}%`;
}
