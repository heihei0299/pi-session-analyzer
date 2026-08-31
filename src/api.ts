/**
 * HTTP API 层：/api/* 端点处理 — 薄路由 adapter，委托 SessionData 深模块。
 * 响应字段复用 serialize.ts 的 *ToObject 转换（与 CLI 结构化输出一致）。
 * 错误：统一 JSON 错误体 { error, detail } + 400/404/409/500。
 */
import {
  defaultSessionData,
  type Filter,
  type TimeRange,
} from "./session-data.ts";
import { collectJsonlFiles } from "./session-data.ts";
import {
  totalsToObject,
  sessionToObject,
  requestToObject,
  groupToObject,
  periodToObject,
} from "./serialize.ts";
import type { GroupBy, Period } from "./aggregate.ts";
import { existsSync, renameSync, statSync, createReadStream } from "node:fs";
import { basename, dirname, join } from "node:path";
import { createInterface } from "node:readline";

/** 会话活跃阈值：文件 mtime 距今 ≤ 5min 视为活跃（pi 正在写入） */
const ACTIVE_MS = 5 * 60 * 1000;

export interface ApiResponse { status: number; body: unknown; }

class ApiError extends Error {
  status: number; detail: string;
  constructor(status: number, error: string, detail: string) {
    super(detail); this.status = status; this.detail = detail; this.name = error;
  }
}

export async function handleApi(
  method: string,
  pathname: string,
  params: URLSearchParams,
  dir: string,
  body = "",
): Promise<ApiResponse> {
  try {
    if (method === "GET" && pathname === "/api/totals") {
      const filter = filterFromParams(params, "message");
      const result = await defaultSessionData.query(dir, filter, { kind: "totals" }) as { totals: import("./aggregate.ts").Totals };
      return { status: 200, body: { window: "totals", ...totalsToObject(result.totals) } };
    }
    if (method === "GET" && pathname === "/api/sessions") {
      const filter = filterFromParams(params, "message");
      const paging = parsePagingAndSort(params);
      const result = await defaultSessionData.query(dir, filter, {
        kind: "sessions",
        page: paging.paging?.page,
        size: paging.paging?.size,
        sortKey: paging.sort?.sortKey,
        sortDir: paging.sort?.sortDir,
      }) as { rows: unknown[]; total: number; page?: number; size?: number; totals: import("./aggregate.ts").Totals };
      // 将 SessionData 的 enriched 行转为 API 响应（已含 fileName/displayName/cwdNorm，补充 serialize）
      const rows = (result.rows as Array<Record<string, unknown>>).map((r) => ({
        ...sessionToObject(r as unknown as import("./aggregate.ts").SessionRow),
        fileName: r.fileName,
        displayName: r.displayName,
        cwdNorm: r.cwdNorm,
        isTask: r.isTask,
      }));
      const out: Record<string, unknown> = { window: "sessions", rows, total: result.total, totals: totalsToObject(result.totals) };
      if (result.page !== undefined) { out.page = result.page; out.size = result.size; }
      return { status: 200, body: out };
    }
    if (method === "POST" && pathname === "/api/sessions/rename") {
      return await renameSession(dir, body);
    }
    if (method === "GET" && pathname === "/api/requests") {
      const filter = filterFromParams(params, "message");
      const paging = parsePagingAndSort(params);
      const result = await defaultSessionData.query(dir, filter, {
        kind: "requests",
        page: paging.paging?.page,
        size: paging.paging?.size,
        sortKey: paging.sort?.sortKey,
        sortDir: paging.sort?.sortDir,
      }) as { rows: unknown[]; total: number; page?: number; size?: number };
      const rows = (result.rows as Array<Record<string, unknown>>).map((r) => ({
        ...requestToObject(r as unknown as import("./aggregate.ts").RequestRow),
        displayName: r.displayName,
      }));
      const out: Record<string, unknown> = { window: "requests", rows, total: result.total };
      if (result.page !== undefined) { out.page = result.page; out.size = result.size; }
      return { status: 200, body: out };
    }
    if (method === "GET" && pathname === "/api/groups") {
      const by = parseGroupBy(params);
      const filter = filterFromParams(params, "message");
      const result = await defaultSessionData.query(dir, filter, { kind: "groups", by }) as { rows: import("./aggregate.ts").GroupRow[]; by: GroupBy };
      return { status: 200, body: { window: "totals", by, rows: result.rows.map(groupToObject) } };
    }
    if (method === "GET" && pathname === "/api/period") {
      const period = parsePeriod(params);
      const filter = filterFromParams(params, "message");
      const result = await defaultSessionData.query(dir, filter, { kind: "period", period }) as { rows: import("./aggregate.ts").PeriodRow[]; period: Period };
      return { status: 200, body: { window: "totals", period, rows: result.rows.map(periodToObject) } };
    }
    if (method === "GET" && pathname === "/api/meta") {
      const result = await defaultSessionData.query(dir, { } as Filter, { kind: "meta" }) as { dir: string; sessionCount: number; dataRange: { since: string | null; until: string | null } };
      return { status: 200, body: result };
    }
    return { status: 404, body: { error: "Not Found", detail: `未知 API 路径: ${pathname}` } };
  } catch (e) {
    if (e instanceof ApiError) return { status: e.status, body: { error: e.name, detail: e.detail } };
    return { status: 500, body: { error: "Internal Server Error", detail: String(e instanceof Error ? e.message : e) } };
  }
}

// ---------- 过滤构造（含 TimeRange 双语义） ----------
function filterFromParams(params: URLSearchParams, kind: "session" | "message"): Filter {
  const since = params.get("since") ?? undefined;
  const until = params.get("until") ?? undefined;
  if (since !== undefined) {
    try { defaultSessionData.parseTimestamp(since, false); } catch { throw new ApiError(400, "Bad Request", `无效 since: ${since}（支持 ISO 日期或时间戳）`); }
  }
  if (until !== undefined) {
    try { defaultSessionData.parseTimestamp(until, true); } catch { throw new ApiError(400, "Bad Request", `无效 until: ${until}（支持 ISO 日期或时间戳）`); }
  }
  const timeRange: TimeRange | null = (since !== undefined || until !== undefined) ? { kind, since, until } as TimeRange : null;
  return {
    model: params.get("model") ?? undefined,
    cwd: params.get("cwd") ?? undefined,
    timeRange,
  };
}

// ---------- 分页/排序解析（复用 SessionData 常量校验） ----------
function parsePagingAndSort(params: URLSearchParams): { paging: { page: number; size: number } | null; sort: { sortKey: string; sortDir: "asc" | "desc" } | null } {
  return { paging: parsePagingParams(params), sort: parseSortParams(params) };
}
function parseSortParams(params: URLSearchParams): { sortKey: string; sortDir: "asc" | "desc" } | null {
  const key = params.get("sortKey"); const dir = params.get("sortDir");
  if (key === null && dir === null) return null;
  if (key === null || dir === null) throw new ApiError(400, "Bad Request", "sortKey 与 sortDir 必须同时提供");
  // 复用 SessionData 的 SORT_KEYS 语义校验（此处硬编码保持与 SessionData 一致，避免循环 import 常量未导出）
  const SORT_KEYS = new Set(["displayName","sessionId","timestamp","cwd","model","requests","input","output","cache","cacheRead","cacheWrite","reasoning","cacheRate","totalTokens","cost"]);
  if (!SORT_KEYS.has(key)) throw new ApiError(400, "Bad Request", `未知排序字段: ${key}`);
  if (dir !== "asc" && dir !== "desc") throw new ApiError(400, "Bad Request", `未知排序方向: ${dir}（支持 asc/desc）`);
  return { sortKey: key, sortDir: dir };
}
function parsePagingParams(params: URLSearchParams): { page: number; size: number } | null {
  const pageRaw = params.get("page"); const sizeRaw = params.get("size");
  if (pageRaw === null && sizeRaw === null) return null;
  if (pageRaw === null || sizeRaw === null) throw new ApiError(400, "Bad Request", "page 与 size 必须同时提供");
  const page = Number(pageRaw); const size = Number(sizeRaw);
  if (!Number.isInteger(page) || page < 1) throw new ApiError(400, "Bad Request", `无效 page: ${pageRaw}（需为正整数）`);
  if (!Number.isInteger(size) || size < 1 || size > 200) throw new ApiError(400, "Bad Request", `无效 size: ${sizeRaw}（需为 1-200 的整数）`);
  return { page, size };
}

function parseGroupBy(params: URLSearchParams): GroupBy {
  const by = params.get("by");
  if (by === "model" || by === "cwd" || by === "model,cwd") return by;
  throw new ApiError(400, "Bad Request", `未知分组: ${by ?? "(缺失)"}（支持 model/cwd/model,cwd）`);
}
function parsePeriod(params: URLSearchParams): Period {
  const period = params.get("period");
  if (period === "day" || period === "week" || period === "month") return period;
  throw new ApiError(400, "Bad Request", `未知周期: ${period ?? "(缺失)"}（支持 day/week/month）`);
}

// ---------- 会话管理：重命名（保持独立，候选 3 再深潜） ----------
function sanitizeName(name: string): string { return name.replace(/[\/\\:*?"<>|]/g, "").trim(); }

async function findSessionFile(dir: string, sessionId: string): Promise<string | null> {
  for (const file of collectJsonlFiles(dir)) {
    const header = await readHeader(file);
    if (header !== null && header.type === "session" && header.id === sessionId) return file;
  }
  return null;
}
async function readHeader(file: string): Promise<{ type?: unknown; id?: unknown } | null> {
  const rl = createInterface({ input: createReadStream(file, "utf8"), crlfDelay: Infinity });
  try {
    for await (const line of rl) {
      if (!line.trim()) continue;
      try { const entry = JSON.parse(line) as Record<string, unknown>; return { type: entry.type, id: entry.id }; } catch { return null; }
    }
    return null;
  } finally { rl.close(); }
}
async function renameSession(dir: string, bodyRaw: string): Promise<ApiResponse> {
  let body: unknown;
  try { body = bodyRaw.trim() === "" ? null : JSON.parse(bodyRaw); } catch { throw new ApiError(400, "Bad Request", "请求体不是合法 JSON"); }
  const sessionId = (body as Record<string, unknown> | null)?.sessionId;
  const name = (body as Record<string, unknown> | null)?.name;
  if (typeof sessionId !== "string" || sessionId === "") throw new ApiError(400, "Bad Request", "缺少 sessionId");
  if (typeof name !== "string") throw new ApiError(400, "Bad Request", "缺少 name");
  const sanitized = sanitizeName(name);
  if (sanitized === "") throw new ApiError(400, "Bad Request", "显示名非法（去除非法字符后为空）");
  const file = await findSessionFile(dir, sessionId);
  if (file === null) throw new ApiError(404, "Not Found", `会话不存在: ${sessionId}`);
  const base = basename(file).replace(/\.jsonl$/, ""); const idx = base.lastIndexOf("_");
  if (idx <= 0 || idx === base.length - 1) throw new ApiError(400, "Bad Request", "无法识别会话 UUID（文件名缺少 _<UUID> 尾缀）");
  const tail = base.slice(idx + 1);
  if (tail !== sessionId) throw new ApiError(400, "Bad Request", "会话 UUID 与 header id 不一致");
  const st = statSync(file);
  if (Date.now() - st.mtimeMs <= ACTIVE_MS) throw new ApiError(409, "Conflict", "会话活跃中，稍后再试");
  const target = join(dirname(file), `${sanitized}_${tail}.jsonl`);
  if (target !== file) {
    if (existsSync(target)) throw new ApiError(409, "Conflict", "同名文件已存在");
    renameSync(file, target);
  }
  return { status: 200, body: { ok: true, fileName: basename(target) } };
}
