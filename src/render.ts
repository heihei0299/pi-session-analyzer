/**
 * 终端表格渲染。
 * 指标列：请求数 / 输入 / 输出 / 缓存读 / 缓存写 / 推理 / 总 token / 花费 / 缓存率
 */
import type { Totals, SessionRow, RequestRow, GroupRow, GroupBy } from "./aggregate.ts";

export function renderTotalsTable(totals: Totals): string {
  return renderRows([metricHeaders(), metricValues(totals)], metricWidths());
}

export function renderSessionTable(rows: SessionRow[]): string {
  return renderRows(
    [
      ["会话ID", "时间戳", "cwd", "模型", ...metricHeaders()],
      ...rows.map((r) => [r.sessionId, r.timestamp, r.cwd, r.model, ...metricValues(r)]),
    ],
    [24, 26, 32, 12, ...metricWidths()],
  );
}

export function renderRequestTable(rows: RequestRow[]): string {
  return renderRows(
    [
      ["会话ID", "时间戳", "模型", ...metricHeaders()],
      ...rows.map((r) => [r.sessionId, r.timestamp, r.model, ...metricValues(r)]),
    ],
    [24, 26, 12, ...metricWidths()],
  );
}

/** 维度分组表格（--by model|cwd|model,cwd，作用于 totals 窗口） */
export function renderGroupTable(rows: GroupRow[], by: GroupBy): string {
  const byModel = by === "model" || by === "model,cwd";
  const byCwd = by === "cwd" || by === "model,cwd";
  const keyCols: string[] = [];
  const keyWidths: number[] = [];
  if (byModel) {
    keyCols.push("模型");
    keyWidths.push(12);
  }
  if (byCwd) {
    keyCols.push("cwd");
    keyWidths.push(32);
  }
  return renderRows(
    [
      [...keyCols, ...metricHeaders()],
      ...rows.map((r) => [
        ...(byModel ? [r.model ?? ""] : []),
        ...(byCwd ? [r.cwd ?? ""] : []),
        ...metricValues(r),
      ]),
    ],
    [...keyWidths, ...metricWidths()],
  );
}

/** 指标列定义（9 列，totals/sessions/requests/group 表格共用） */
const METRIC_COLUMNS = [
  { header: "请求数", width: 8, fmt: (t: Totals) => String(t.requests) },
  { header: "输入", width: 12, fmt: (t: Totals) => formatTokens(t.input) },
  { header: "输出", width: 12, fmt: (t: Totals) => formatTokens(t.output) },
  { header: "缓存读", width: 12, fmt: (t: Totals) => formatTokens(t.cacheRead) },
  { header: "缓存写", width: 12, fmt: (t: Totals) => formatTokens(t.cacheWrite) },
  { header: "推理", width: 12, fmt: (t: Totals) => formatTokens(t.reasoning) },
  { header: "总 token", width: 14, fmt: (t: Totals) => formatTokens(t.totalTokens) },
  { header: "花费", width: 14, fmt: (t: Totals) => formatCost(t.cost) },
  { header: "缓存率", width: 10, fmt: (t: Totals) => formatRate(t.cacheRate) },
] as const;

function metricHeaders(): string[] {
  return METRIC_COLUMNS.map((c) => c.header);
}

function metricValues(t: Totals): string[] {
  return METRIC_COLUMNS.map((c) => c.fmt(t));
}

function metricWidths(): number[] {
  return METRIC_COLUMNS.map((c) => c.width);
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
