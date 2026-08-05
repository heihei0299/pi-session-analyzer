/**
 * 终端表格渲染。
 * 列：请求数 / 输入 / 输出 / 缓存读 / 缓存写 / 推理 / 总 token / 花费 / 缓存率
 */
import type { Totals, SessionRow, RequestRow } from "./aggregate.ts";



export function renderTotalsTable(totals: Totals): string {
  return renderRows(
    [
      ["请求数", "输入", "输出", "缓存读", "缓存写", "推理", "总 token", "花费", "缓存率"],
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
    ],
    [8, 12, 12, 12, 12, 12, 14, 14, 10],
  );
}

export function renderSessionTable(rows: SessionRow[]): string {
  return renderRows(
    [
      ["会话ID", "时间戳", "cwd", "模型", "请求数", "输入", "输出", "缓存读", "缓存写", "推理", "总 token", "花费", "缓存率"],
      ...rows.map((r) => [
        r.sessionId,
        r.timestamp,
        r.cwd,
        r.model,
        String(r.requests),
        formatTokens(r.input),
        formatTokens(r.output),
        formatTokens(r.cacheRead),
        formatTokens(r.cacheWrite),
        formatTokens(r.reasoning),
        formatTokens(r.totalTokens),
        formatCost(r.cost),
        formatRate(r.cacheRate),
      ]),
    ],
    [24, 26, 32, 12, 8, 12, 12, 12, 12, 12, 14, 14, 10],
  );
}

export function renderRequestTable(rows: RequestRow[]): string {
  return renderRows(
    [
      ["会话ID", "时间戳", "模型", "请求数", "输入", "输出", "缓存读", "缓存写", "推理", "总 token", "花费", "缓存率"],
      ...rows.map((r) => [
        r.sessionId,
        r.timestamp,
        r.model,
        String(r.requests),
        formatTokens(r.input),
        formatTokens(r.output),
        formatTokens(r.cacheRead),
        formatTokens(r.cacheWrite),
        formatTokens(r.reasoning),
        formatTokens(r.totalTokens),
        formatCost(r.cost),
        formatRate(r.cacheRate),
      ]),
    ],
    [24, 26, 12, 8, 12, 12, 12, 12, 12, 14, 14, 10],
  );
}

/** 通用表格渲染：首行为表头，后续为数据行，按终端显示宽度（CJK 计 2）对齐 */
function renderRows(allRows: string[][], minWidths: number[]): string {
  const headers = allRows[0];
  const widths = headers.map((h, i) =>
    Math.max(minWidths[i] ?? 0, displayWidth(h), ...allRows.slice(1).map((r) => displayWidth(r[i] ?? ""))),
  );
  return allRows
    .map((row) => row.map((cell, i) => padWidth(cell, widths[i])).join("  "))
    .join("\n") + "\n";
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

/** 零花费（费率未配置/免费未定价）的展示标注 */
const UNPRICED = "费率未配置（免费/未定价）";
function formatCost(n: number): string {
  if (n === 0) return UNPRICED;
  return n.toLocaleString("en-US", { maximumFractionDigits: 6 });
}

function formatRate(r: number): string {
  return `${(r * 100).toFixed(2)}%`;
}
