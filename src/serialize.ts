/**
 * 结构化输出序列化：JSON / CSV。
 * 字段与终端表格列一一对应（驼峰命名）；值为原始数值（cacheRate 为 0-1 小数）。
 */
import { emptyTotals, type Totals, type SessionRow, type RequestRow } from "./aggregate.ts";
import type { WindowName } from "./cli.ts";

/** 序列化为 JSON 文本（totals 为单对象；sessions/requests 为 { window, rows }） */
export function serializeJson(
  window: WindowName,
  totals: Totals,
  sessions: SessionRow[],
  requests: RequestRow[],
): string {
  let body: unknown;
  if (window === "totals") {
    body = { window, ...totalsToObject(totals) };
  } else if (window === "sessions") {
    body = { window, rows: sessions.map(sessionToObject) };
  } else {
    body = { window, rows: requests.map(requestToObject) };
  }
  return JSON.stringify(body, null, 2) + "\n";
}

/** 序列化为 CSV 文本（首行表头，字段与 JSON 一致；空数据也输出表头） */
export function serializeCsv(
  window: WindowName,
  totals: Totals,
  sessions: SessionRow[],
  requests: RequestRow[],
): string {
  const rows: Record<string, unknown>[] =
    window === "totals"
      ? [{ window, ...totalsToObject(totals) }]
      : window === "sessions"
        ? sessions.map(sessionToObject)
        : requests.map(requestToObject);
  // 空数据：用窗口的字段结构推导表头，保证脚本可消费
  const headers = Object.keys(rows[0] ?? emptyRowFor(window));
  const lines = [
    headers.map(csvEscape).join(","),
    ...rows.map((r) => headers.map((h) => csvEscape(String(r[h]))).join(",")),
  ];
  return lines.join("\n") + "\n";
}

/** 窗口为空时用于推导表头的占位行 */
function emptyRowFor(window: string): Record<string, unknown> {
  if (window === "sessions") {
    return { sessionId: "", timestamp: "", cwd: "", model: "", ...totalsToObject(emptyTotals()) };
  }
  if (window === "requests") {
    return { sessionId: "", timestamp: "", model: "", ...totalsToObject(emptyTotals()) };
  }
  return totalsToObject(emptyTotals());
}

/** CSV 字段转义：含逗号/引号/换行的字段加引号包裹，内部引号双写 */
function csvEscape(s: string): string {
  return /[",\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

export function totalsToObject(t: Totals): Record<string, unknown> {
  return {
    requests: t.requests,
    input: t.input,
    output: t.output,
    cacheRead: t.cacheRead,
    cacheWrite: t.cacheWrite,
    reasoning: t.reasoning,
    totalTokens: t.totalTokens,
    cost: t.cost,
    cacheRate: t.cacheRate,
  };
}

export function sessionToObject(r: SessionRow): Record<string, unknown> {
  return {
    sessionId: r.sessionId,
    timestamp: r.timestamp,
    cwd: r.cwd,
    model: r.model,
    ...totalsToObject(r),
  };
}

export function requestToObject(r: RequestRow): Record<string, unknown> {
  return {
    sessionId: r.sessionId,
    timestamp: r.timestamp,
    model: r.model,
    ...totalsToObject(r),
  };
}
