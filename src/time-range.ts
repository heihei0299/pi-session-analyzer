import type { SessionFileData } from "./session-data.ts";
export function parseUtcTimestamp(s: string): number {
  const hasTz = /(?:Z|[+-]\d{2}:?\d{2})$/.test(s);
  return Date.parse(hasTz ? s : s + "Z");
}
export function parseTimestamp(s: string, endOfDay: boolean): number {
  const dateOnly = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s);
  if (dateOnly) {
    const [, y, mo, d] = dateOnly;
    const yi = Number(y), mi = Number(mo), di = Number(d);
    // 严格校验：非法月/日（如 2026-13-01、2026-02-30）抛错，避免 new Date 自动归一化
    const baseDate = new Date(yi, mi - 1, di);
    if (baseDate.getFullYear() !== yi || baseDate.getMonth() !== mi - 1 || baseDate.getDate() !== di) {
      throw new Error(`无效时间: ${s}（支持 ISO 日期或时间戳）`);
    }
    const base = baseDate.getTime();
    return endOfDay ? base + 86_400_000 - 1 : base;
  }
  const ms = Date.parse(s);
  if (Number.isNaN(ms)) throw new Error(`无效时间: ${s}（支持 ISO 日期或时间戳）`);
  return ms;
}

export type SessionTimeRange = { kind: "session"; since?: string; until?: string; _sinceMs?: number; _untilMs?: number };
export type MessageTimeRange = { kind: "message"; since?: string; until?: string; _sinceMs?: number; _untilMs?: number };
export type TimeRange = SessionTimeRange | MessageTimeRange;

function makeRange(kind: "session" | "message", input: { since?: string; until?: string }): TimeRange | null {
  const { since, until } = input;
  if (since === undefined && until === undefined) return null;
  // 校验：无效即抛，文案与 parseTimestamp 一致（上层映射 400）
  let sinceMs: number | undefined;
  let untilMs: number | undefined;
  if (since !== undefined) sinceMs = parseTimestamp(since, false);
  if (until !== undefined) untilMs = parseTimestamp(until, true);
  // 将已解析 ms 缓存于对象内部，避免每次 apply 重解析
  return kind === "session"
    ? { kind: "session", since, until, _sinceMs: sinceMs, _untilMs: untilMs }
    : { kind: "message", since, until, _sinceMs: sinceMs, _untilMs: untilMs };
}

export function makeSessionRange(input: { since?: string; until?: string }): SessionTimeRange | null {
  return makeRange("session", input) as SessionTimeRange | null;
}
export function makeMessageRange(input: { since?: string; until?: string }): MessageTimeRange | null {
  return makeRange("message", input) as MessageTimeRange | null;
}

export function applyTimeRange(files: SessionFileData[], range: TimeRange | null): SessionFileData[] {
  if (range === null || range === undefined) return files;
  // 兼容：若来自 SessionData 的原始 {kind,since,until} 未经 make*Range 缓存，现场解析
  let sinceMs = (range as { _sinceMs?: number })._sinceMs;
  let untilMs = (range as { _untilMs?: number })._untilMs;
  if (sinceMs === undefined && (range as { since?: string }).since !== undefined) {
    sinceMs = parseTimestamp((range as { since: string }).since, false);
  }
  if (untilMs === undefined && (range as { until?: string }).until !== undefined) {
    untilMs = parseTimestamp((range as { until: string }).until, true);
  }
  if (range.kind === "session") {
    if (sinceMs === undefined && untilMs === undefined) return files;
    return files.filter((f) => {
      const ts = parseUtcTimestamp(f.timestamp);
      if (Number.isNaN(ts)) return true;
      if (sinceMs !== undefined && ts < sinceMs) return false;
      if (untilMs !== undefined && ts > untilMs) return false;
      return true;
    });
  }
  if (sinceMs === undefined && untilMs === undefined) return files;
  return files
    .map((f) => ({
      ...f,
      items: f.items.filter((it) => {
        const ts = parseUtcTimestamp(it.timestamp);
        if (Number.isNaN(ts)) return true;
        if (sinceMs !== undefined && ts < sinceMs) return false;
        if (untilMs !== undefined && ts > untilMs) return false;
        return true;
      }),
    }))
    .filter((f) => f.items.length > 0);
}
