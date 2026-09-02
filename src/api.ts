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
import { existsSync, renameSync, statSync, createReadStream, readFileSync } from "node:fs";
import { basename, dirname, join, resolve } from "node:path";
import { createInterface } from "node:readline";
import { OpenCodeStorage } from "./opencode/storage.ts";
import { OpenCodeClient } from "./opencode/client.ts";
import { buildAudit } from "./opencode/audit.ts";
import { loadCredentials } from "./opencode/credentials.ts";

/** 会话活跃阈值：文件 mtime 距今 ≤ 5min 视为活跃（pi 正在写入） */
const ACTIVE_MS = 5 * 60 * 1000;

// ponytail: 串行同步，避免并发写 history.json 冲突；单进程锁足够，后续并发需求再上文件锁
let opencodeSyncLock = false;

export interface ApiResponse { status: number; body: unknown; }

class ApiError extends Error {
  status: number; detail: string;
  constructor(status: number, error: string, detail: string) {
    super(detail); this.status = status; this.detail = detail; this.name = error;
  }
}

function serializeDetail(detail: {
  session: import("./session-data.ts").SessionRowEnriched;
  children: import("./session-data.ts").SessionRowEnriched[];
  totals: { main: import("./aggregate.ts").Totals; merged: import("./aggregate.ts").Totals; childrenCount: number };
  requests: (import("./aggregate.ts").RequestRow & { displayName: string; source: string; sourceSessionId: string })[];
  meta: { hasChildren: boolean };
}): Record<string, unknown> {
  const sessionObj = {
    ...sessionToObject(detail.session as unknown as import("./aggregate.ts").SessionRow),
    fileName: detail.session.fileName,
    displayName: detail.session.displayName,
    cwdNorm: detail.session.cwdNorm,
    isTask: detail.session.isTask,
  };
  const childrenObjs = detail.children.map((c) => ({
    ...sessionToObject(c as unknown as import("./aggregate.ts").SessionRow),
    fileName: c.fileName,
    displayName: c.displayName,
    cwdNorm: c.cwdNorm,
    isTask: c.isTask,
  }));
  const totals = {
    main: totalsToObject(detail.totals.main),
    merged: totalsToObject(detail.totals.merged),
    childrenCount: detail.totals.childrenCount,
  };
  const reqRows = detail.requests.map((r) => ({
    ...requestToObject(r as unknown as import("./aggregate.ts").RequestRow),
    displayName: (r as unknown as Record<string, unknown>).displayName,
    source: (r as unknown as Record<string, unknown>).source,
    sourceSessionId: (r as unknown as Record<string, unknown>).sourceSessionId,
  }));
  return { session: sessionObj, children: childrenObjs, totals, requests: reqRows, meta: detail.meta };
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
        parentSessionId: r.parentSessionId,
      }));
      const out: Record<string, unknown> = { window: "sessions", rows, total: result.total, totals: totalsToObject(result.totals) };
      if (result.page !== undefined) { out.page = result.page; out.size = result.size; }
      return { status: 200, body: out };
    }
    if (method === "GET" && pathname === "/api/sessions/detail") {
      const sessionId = params.get("sessionId") ?? params.get("sessionID") ?? params.get("id") ?? "";
      if (!sessionId || sessionId.trim() === "") throw new ApiError(400, "Bad Request", "缺少 sessionId");
      try {
        const detail = await defaultSessionData.queryDetail(dir, sessionId);
        return { status: 200, body: serializeDetail(detail as unknown as Parameters<typeof serializeDetail>[0]) };
      } catch (e) {
        if ((e as Error & { status?: number })?.status === 404) throw new ApiError(404, "Not Found", (e as Error).message);
        throw e;
      }
    }
    if (method === "GET" && pathname.startsWith("/api/sessions/") && pathname.endsWith("/detail")) {
      const m = pathname.match(/^\/api\/sessions\/([^/]+)\/detail$/);
      if (!m) throw new ApiError(400, "Bad Request", "缺少 sessionId");
      let sessionId: string;
      try { sessionId = decodeURIComponent(m[1]); } catch { sessionId = m[1]; }
      if (!sessionId || sessionId.trim() === "") throw new ApiError(400, "Bad Request", "缺少 sessionId");
      try {
        const detail = await defaultSessionData.queryDetail(dir, sessionId);
        return { status: 200, body: serializeDetail(detail as unknown as Parameters<typeof serializeDetail>[0]) };
      } catch (e) {
        if ((e as Error & { status?: number })?.status === 404) throw new ApiError(404, "Not Found", (e as Error).message);
        throw e;
      }
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
    // ---------- OpenCode 扩展端点 ----------
    if (method === "GET" && pathname === "/api/opencode/costs") {
      const { year, month } = parseYearMonth(params);
      const storage = getOpencodeStorage();
      await storage.ensureDataDir();
      const costs = await storage.getCosts(year, month);
      if (costs === null) {
        return { status: 200, body: { year, month, costs: { usage: [], keys: [] } } };
      }
      return { status: 200, body: { year, month, costs } };
    }
    if (method === "GET" && pathname === "/api/opencode/history") {
      const paging = parseOpencodePaging(params);
      const model = params.get("model") ?? undefined;
      const session = params.get("session") ?? params.get("sessionID") ?? params.get("sessionId") ?? undefined;
      const storage = getOpencodeStorage();
      await storage.ensureDataDir();
      const filter: import("./opencode/storage.ts").HistoryFilter = {};
      if (model) filter.model = model;
      if (session) filter.sessionId = session;
      const hasFilter = model !== undefined || session !== undefined;
      const records = await storage.getHistory(hasFilter ? filter : undefined);
      const total = records.length;
      if (paging) {
        const start = (paging.page - 1) * paging.size;
        const rows = records.slice(start, start + paging.size);
        return { status: 200, body: { rows, total, page: paging.page, size: paging.size } };
      }
      return { status: 200, body: { rows: records, total } };
    }
    if (method === "GET" && pathname === "/api/opencode/audit") {
      const { year, month } = parseYearMonth(params);
      const pad2 = (n: number) => String(n).padStart(2, "0");
      const lastDay = new Date(year, month, 0).getDate();
      const sinceStr = `${year}-${pad2(month)}-01`;
      const untilStr = `${year}-${pad2(month)}-${pad2(lastDay)}`;
      const filter: Filter = { timeRange: { kind: "message", since: sinceStr, until: untilStr } as TimeRange };
      const localRes = await defaultSessionData.query(dir, filter, { kind: "totals" }) as { totals: import("./aggregate.ts").Totals };
      const localTotals = localRes.totals;
      const storage = getOpencodeStorage();
      await storage.ensureDataDir();
      const loaded = await storage.loadHistory();
      const sinceMs = defaultSessionData.parseTimestamp(sinceStr, false);
      const untilMs = defaultSessionData.parseTimestamp(untilStr, true);
      const filtered = loaded.records.filter((r) => {
        const t = Date.parse(r.timeCreated);
        if (Number.isNaN(t)) return false;
        return t >= sinceMs && t <= untilMs;
      });
      const audit = buildAudit(localTotals, filtered);
      return {
        status: 200,
        body: {
          year,
          month,
          localTotals: totalsToObject(localTotals),
          opencodeTotals: audit.opencodeTotals,
          diff: audit.diff,
          diffRate: audit.diffRate,
          comparison: audit.comparison,
        },
      };
    }
    if (method === "POST" && pathname === "/api/opencode/sync") {
      if (opencodeSyncLock) {
        throw new ApiError(409, "Conflict", "同步进行中，请稍后再试");
      }
      opencodeSyncLock = true;
      try {
        const storage = getOpencodeStorage();
        await storage.ensureDataDir();
        // 解析 body 临时凭证（若提供则优先于 env/.env，优先级 body > env）
        let bodyAuth: string | undefined;
        let bodyWorkspace: string | undefined;
        if (body && body.trim()) {
          try {
            const parsed = JSON.parse(body) as Record<string, unknown>;
            if (typeof parsed.auth === "string" && parsed.auth.trim()) bodyAuth = parsed.auth.trim();
            if (typeof parsed.workspaceId === "string" && parsed.workspaceId.trim()) bodyWorkspace = parsed.workspaceId.trim();
            if (typeof parsed.workspace === "string" && parsed.workspace.trim() && !bodyWorkspace) bodyWorkspace = parsed.workspace.trim();
          } catch {
            // body 非 JSON 时忽略，按 env 凭证继续
          }
        }
        const creds = loadOpencodeCredentials(bodyAuth, bodyWorkspace);
        const auth = creds.auth;
        let workspaceId = creds.workspaceId;
        if (!auth) {
          throw new ApiError(500, "Internal Server Error", "认证失效: 缺少 OpenCode auth，请设置 OPENCODE_AUTH 环境变量或 .env（凭证过期/缺失）");
        }
        const client = new OpenCodeClient(auth);
        if (!workspaceId) {
          let workspaces: import("./opencode/types.ts").WorkspaceInfo[] = [];
          try {
            workspaces = await client.getWorkspaces();
          } catch (e) {
            const msg = e instanceof Error ? e.message : String(e);
            throw new ApiError(500, "Internal Server Error", msg);
          }
          if (workspaces.length === 0) {
            throw new ApiError(500, "Internal Server Error", "无法发现工作区：请显式设置 OPENCODE_WORKSPACE_ID");
          }
          const first = workspaces[0] as unknown as Record<string, unknown>;
          workspaceId = (first.id as string) ?? (first.workspaceID as string) ?? "";
          if (!workspaceId) {
            throw new ApiError(500, "Internal Server Error", "工作区 ID 为空，无法同步");
          }
        }
        let result: import("./opencode/storage.ts").SyncResult;
        try {
          result = await storage.sync(client, { workspaceId });
        } catch (e) {
          const msg = e instanceof Error ? e.message : String(e);
          throw new ApiError(500, "Internal Server Error", msg);
        }
        return { status: 200, body: result };
      } catch (e) {
        if (e instanceof ApiError) throw e;
        const msg = e instanceof Error ? e.message : String(e);
        throw new ApiError(500, "Internal Server Error", msg);
      } finally {
        opencodeSyncLock = false;
      }
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

// ---------- OpenCode 扩展：year/month 校验与分页 ----------
function parseYearMonth(params: URLSearchParams): { year: number; month: number } {
  const yearRaw = params.get("year");
  const monthRaw = params.get("month");
  if (yearRaw === null) throw new ApiError(400, "Bad Request", "缺少 year 参数（需为 4 位年份，如 2026）");
  if (monthRaw === null) throw new ApiError(400, "Bad Request", "缺少 month 参数（需为 1-12 的整数）");
  if (!/^\d{4}$/.test(yearRaw)) throw new ApiError(400, "Bad Request", `无效 year: ${yearRaw}（需为 4 位年份）`);
  const year = Number(yearRaw);
  if (!Number.isInteger(year) || year < 1000 || year > 9999) throw new ApiError(400, "Bad Request", `无效 year: ${yearRaw}（需为 1000-9999）`);
  const month = Number(monthRaw);
  if (!Number.isInteger(month) || month < 1 || month > 12) throw new ApiError(400, "Bad Request", `无效 month: ${monthRaw}（需为 1-12 的整数）`);
  return { year, month };
}

function parseOpencodePaging(params: URLSearchParams): { page: number; size: number } | null {
  const pageRaw = params.get("page"); const sizeRaw = params.get("size");
  if (pageRaw === null && sizeRaw === null) return null;
  if (pageRaw === null || sizeRaw === null) throw new ApiError(400, "Bad Request", "page 与 size 必须同时提供");
  const page = Number(pageRaw); const size = Number(sizeRaw);
  if (!Number.isInteger(page) || page < 1 || page > 200) throw new ApiError(400, "Bad Request", `无效 page: ${pageRaw}（需为 1-200 的整数）`);
  if (!Number.isInteger(size) || size < 1 || size > 200) throw new ApiError(400, "Bad Request", `无效 size: ${sizeRaw}（需为 1-200 的整数）`);
  return { page, size };
}

function getOpencodeStorage(): OpenCodeStorage {
  const envDir = process.env.OPENCODE_DATA_DIR;
  const dir = envDir && envDir.trim() ? envDir.trim() : "data/opencode";
  return new OpenCodeStorage(resolve(dir));
}

function loadOpencodeCredentials(bodyAuth?: string, bodyWorkspace?: string): { auth?: string; workspaceId?: string } {
  // 复用 #03 已实现的凭证加载器（含 .env 解析），零重复
  const dataDir = process.env.OPENCODE_DATA_DIR?.trim() || "data/opencode";
  try {
    const creds = loadCredentials({ auth: bodyAuth, workspace: bodyWorkspace, dataDir });
    return { auth: creds.auth, workspaceId: creds.workspace };
  } catch {
    // loadCredentials 在缺失 auth 时抛错，此处返回 undefined 供上层映射为 500 友好错误
    const auth = bodyAuth?.trim() || process.env.OPENCODE_AUTH?.trim() || undefined;
    const wid = bodyWorkspace?.trim() || process.env.OPENCODE_WORKSPACE_ID?.trim() || undefined;
    return { auth: auth || undefined, workspaceId: wid || undefined };
  }
}

// 用于测试：重置同步锁，避免跨用例污染
export function __resetOpencodeSyncLockForTest(): void {
  opencodeSyncLock = false;
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
