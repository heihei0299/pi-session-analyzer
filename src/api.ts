/**
 * HTTP API 层：/api/* 端点处理。
 * 每次请求全量 readSessionFiles(dir) → filterFiles（model/cwd/since/until）→ 按端点派生；
 * 响应字段复用 serialize.ts 的 *ToObject 转换（与 CLI 结构化输出一致）。
 * 错误：统一 JSON 错误体 { error, detail } + 400/404/409/500。
 */
import {
  readSessionFilesCached,
  filterFiles,
  totalsFromFiles,
  sessionRowsFromFiles,
  requestRowsFromFiles,
  groupRowsFromFiles,
  periodRowsFromFiles,
  collectJsonlFiles,
  normalizeCwd,
  parseTimestamp,
  parseUtcTimestamp,
  type SessionFileData,
} from "./analyze.ts";
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

export interface ApiResponse {
  status: number;
  body: unknown;
}

/** 参数非法 / 数据不可用等业务错误（映射统一错误体） */
class ApiError extends Error {
  status: number;
  detail: string;
  constructor(status: number, error: string, detail: string) {
    super(detail);
    this.status = status;
    this.detail = detail;
    this.name = error;
  }
}

/**
 * 处理一个 /api/* 请求。
 * 未知路径 / 方法不匹配 → 404 统一错误体；数据目录不可读或无合法会话 → 500。
 */
export async function handleApi(
  method: string,
  pathname: string,
  params: URLSearchParams,
  dir: string,
  body = "",
): Promise<ApiResponse> {
  try {
    if (method === "GET" && pathname === "/api/totals") {
      const filtered = await loadFilteredMessageTime(dir, params);
      return { status: 200, body: { window: "totals", ...totalsToObject(totalsFromFiles(filtered)) } };
    }
    if (method === "GET" && pathname === "/api/sessions") {
      const filtered = await loadFiltered(dir, params);
      const rows = sessionRowsFromFiles(filtered).map((r, i) => {
        const f = filtered[i];
        const fileName = f.fileName ?? "";
        return {
          ...sessionToObject(r),
          fileName,
          displayName: displayNameOf(fileName, f.firstUserText),
          cwdNorm: normalizeCwd(f.cwd),
        };
      });
      return { status: 200, body: { window: "sessions", ...paginate(rows, params) } };
    }
    if (method === "POST" && pathname === "/api/sessions/rename") {
      return await renameSession(dir, body);
    }
    if (method === "GET" && pathname === "/api/requests") {
      const timeFiltered = await loadFilteredMessageTime(dir, params);
      // 会话名称映射：sessionId → displayName（会话名称列数据源，与会话管理同规则）
      const nameBySession = new Map<string, string>();
      for (const f of timeFiltered) nameBySession.set(f.sessionId, displayNameOf(f.fileName ?? "", f.firstUserText));
      const rows = requestRowsFromFiles(timeFiltered).map((r) => ({ ...requestToObject(r), displayName: nameBySession.get(r.sessionId) ?? "" }));
      return {
        status: 200,
        body: {
          window: "requests",
          ...paginate(rows, params),
        },
      };
    }
    if (method === "GET" && pathname === "/api/groups") {
      const by = parseGroupBy(params);
      const filtered = await loadFilteredMessageTime(dir, params);
      return { status: 200, body: { window: "totals", by, rows: groupRowsFromFiles(filtered, by).map(groupToObject) } };
    }
    if (method === "GET" && pathname === "/api/period") {
      const period = parsePeriod(params);
      const filtered = await loadFilteredMessageTime(dir, params);
      return { status: 200, body: { window: "totals", period, rows: periodRowsFromFiles(filtered, period).map(periodToObject) } };
    }
    if (method === "GET" && pathname === "/api/meta") {
      const files = await loadFiles(dir);
      return { status: 200, body: buildMeta(dir, files) };
    }
    return { status: 404, body: { error: "Not Found", detail: `未知 API 路径: ${pathname}` } };
  } catch (e) {
    if (e instanceof ApiError) {
      return { status: e.status, body: { error: e.name, detail: e.detail } };
    }
    return { status: 500, body: { error: "Internal Server Error", detail: String(e instanceof Error ? e.message : e) } };
  }
}

/** 读取会话数据；目录不可读或无合法会话 → 500（统一错误体） */
async function loadFiles(dir: string): Promise<SessionFileData[]> {
  let files: SessionFileData[];
  try {
    files = await readSessionFilesCached(dir);
  } catch (e) {
    throw new ApiError(500, "Internal Server Error", `数据目录不可读: ${dir}（${e instanceof Error ? e.message : String(e)}）`);
  }
  if (files.length === 0) {
    throw new ApiError(500, "Internal Server Error", `数据目录无合法会话: ${dir}`);
  }
  return files;
}

/** 读取并应用筛选（model/cwd/since/until 映射 CLI 语义）；非法时间参数 → 400 */
async function loadFiltered(dir: string, params: URLSearchParams): Promise<SessionFileData[]> {
  const files = await loadFiles(dir);
  const filters = filtersFromParams(params);
  return filterFiles(files, filters);
}

/**
 * webui 消息级时间筛选路径（ticket 22/23 用户决策）：model/cwd 沿用 filterFiles（model 消息级、cwd 会话级），
 * since/until 按**消息 timestamp** 过滤——totals/groups/period/requests 均按消息归属（跨天会话的凌晨请求计入当天）；
 * sessions 明细与会话管理仍走 loadFiltered（会话 header 归属，CLI 口径）。
 */
async function loadFilteredMessageTime(dir: string, params: URLSearchParams): Promise<SessionFileData[]> {
  const files = await loadFiles(dir);
  const filters = filtersFromParams(params);
  const filtered = filterFiles(files, { model: filters.model, cwd: filters.cwd });
  return applyMessageTimeFilter(filtered, filters.since, filters.until);
}

function filtersFromParams(params: URLSearchParams): { model?: string; cwd?: string; since?: string; until?: string } {
  const since = params.get("since") ?? undefined;
  const until = params.get("until") ?? undefined;
  // 提前校验时间格式（filterFiles 内部 parseTimestamp 抛错会变成 500，须在此转 400）
  if (since !== undefined) {
    try {
      parseTimestamp(since, false);
    } catch {
      throw new ApiError(400, "Bad Request", `无效 since: ${since}（支持 ISO 日期或时间戳）`);
    }
  }
  if (until !== undefined) {
    try {
      parseTimestamp(until, true);
    } catch {
      throw new ApiError(400, "Bad Request", `无效 until: ${until}（支持 ISO 日期或时间戳）`);
    }
  }
  return {
    model: params.get("model") ?? undefined,
    cwd: params.get("cwd") ?? undefined,
    since,
    until,
  };
}

/**
 * 明细（单请求级）时间过滤：按**消息 timestamp**（UTC 基准）闭区间过滤，与会话级（header）不同——
 * 跨天会话中落在 [since, until] 内的请求保留（ticket 22 用户决策：requests 明细消息级；
 * totals/sessions/groups/period 保持会话级口径不变）。无有效时间戳的条目不参与过滤（与会话级一致）。
 */
function applyMessageTimeFilter(
  files: SessionFileData[],
  since?: string,
  until?: string,
): SessionFileData[] {
  if (since === undefined && until === undefined) return files;
  const sinceMs = since !== undefined ? parseTimestamp(since, false) : undefined;
  const untilMs = until !== undefined ? parseTimestamp(until, true) : undefined;
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

// ---------- 明细端点排序 + 分页（page/size/sortKey/sortDir） ----------

/** 明细端点可排序字段（前端列集；cache 为 cacheRead+cacheWrite 别名） */
const SORT_KEYS = new Set([
  "displayName", "sessionId", "timestamp", "cwd", "model",
  "requests", "input", "output", "cache", "cacheRead", "cacheWrite",
  "reasoning", "cacheRate", "totalTokens", "cost",
]);
/** 数值列（数字排序）；其余列字符串 localeCompare */
const NUMERIC_SORT_KEYS = new Set([
  "requests", "input", "output", "cache", "cacheRead", "cacheWrite",
  "reasoning", "cacheRate", "totalTokens", "cost",
]);

/**
 * 明细端点统一分页/排序出口：解析可选参数 → 排序 → 分页。
 * 响应恒含 total（= 筛选后全量行数，前端「N 行 · 第 X/Y 页」用）；
 * 传了 page/size 时附加 page/size 字段并只返回当前页。
 * 参数规则：page 与 size 须成对（1-200），sortKey 与 sortDir 须成对——单独出现 → 400。
 */
function paginate<T extends Record<string, unknown>>(
  rows: T[],
  params: URLSearchParams,
): { rows: T[]; total: number; page?: number; size?: number } {
  const sort = parseSortParams(params);
  const out = sort !== null ? applySort(rows, sort.sortKey, sort.sortDir) : rows;
  const paging = parsePagingParams(params);
  const total = out.length;
  if (paging === null) return { rows: out, total };
  const start = (paging.page - 1) * paging.size;
  return { rows: out.slice(start, start + paging.size), total, page: paging.page, size: paging.size };
}

/** 解析 sortKey/sortDir（须成对出现）；非法字段或方向 → 400 */
function parseSortParams(params: URLSearchParams): { sortKey: string; sortDir: "asc" | "desc" } | null {
  const key = params.get("sortKey");
  const dir = params.get("sortDir");
  if (key === null && dir === null) return null;
  if (key === null || dir === null) {
    throw new ApiError(400, "Bad Request", "sortKey 与 sortDir 必须同时提供");
  }
  if (!SORT_KEYS.has(key)) throw new ApiError(400, "Bad Request", `未知排序字段: ${key}`);
  if (dir !== "asc" && dir !== "desc") throw new ApiError(400, "Bad Request", `未知排序方向: ${dir}（支持 asc/desc）`);
  return { sortKey: key, sortDir: dir };
}

/** 解析 page/size（须成对出现）；非法值 → 400；都未提供 → null（不分页） */
function parsePagingParams(params: URLSearchParams): { page: number; size: number } | null {
  const pageRaw = params.get("page");
  const sizeRaw = params.get("size");
  if (pageRaw === null && sizeRaw === null) return null;
  if (pageRaw === null || sizeRaw === null) {
    throw new ApiError(400, "Bad Request", "page 与 size 必须同时提供");
  }
  const page = Number(pageRaw);
  const size = Number(sizeRaw);
  if (!Number.isInteger(page) || page < 1) {
    throw new ApiError(400, "Bad Request", `无效 page: ${pageRaw}（需为正整数）`);
  }
  if (!Number.isInteger(size) || size < 1 || size > 200) {
    throw new ApiError(400, "Bad Request", `无效 size: ${sizeRaw}（需为 1-200 的整数）`);
  }
  return { page, size };
}

/** 应用排序（copy + stable sort）：数值列数字比较，其余 localeCompare；cache 别名 = cacheRead+cacheWrite */
function applySort<T extends Record<string, unknown>>(
  rows: T[],
  sortKey: string,
  sortDir: "asc" | "desc",
): T[] {
  const dir = sortDir === "asc" ? 1 : -1;
  const valueOf = (r: T): unknown =>
    sortKey === "cache" ? Number(r.cacheRead ?? 0) + Number(r.cacheWrite ?? 0) : r[sortKey];
  return [...rows].sort((a, b) => {
    const va = valueOf(a);
    const vb = valueOf(b);
    if (NUMERIC_SORT_KEYS.has(sortKey)) return (Number(va) - Number(vb)) * dir;
    return String(va).localeCompare(String(vb)) * dir;
  });
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

/** /api/meta：dir=传入值；sessionCount=合法会话数；dataRange=可解析时间戳的 min/max（ISO 字典序=时间序） */
function buildMeta(dir: string, files: SessionFileData[]): Record<string, unknown> {
  const timestamps = files
    .map((f) => f.timestamp)
    .filter((t) => !Number.isNaN(parseUtcTimestamp(t)));
  const dataRange =
    timestamps.length === 0
      ? { since: null, until: null }
      : {
          since: timestamps.reduce((a, b) => (a < b ? a : b)),
          until: timestamps.reduce((a, b) => (a > b ? a : b)),
        };
  return { dir, sessionCount: files.length, dataRange };
}

// ---------- 会话管理：显示名派生 + 重命名 ----------

/** 显示名派生：重命名过的（文件名前缀非 pi 时间戳格式）用前缀；否则用首条 user 消息文本 */
function displayNameOf(fileName: string, firstUserText?: string): string {
  const base = fileName.endsWith(".jsonl") ? fileName.slice(0, -6) : fileName;
  const idx = base.lastIndexOf("_");
  const prefix = idx > 0 && idx < base.length - 1 ? base.slice(0, idx) : fileName;
  // pi 默认文件名 `<时间戳>_<UUID>.jsonl`（前缀 ISO 时间戳）→ 未重命名 → 用首条 user 消息
  if (/^\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}/.test(prefix)) {
    return firstUserText && firstUserText.length > 0 ? firstUserText : prefix;
  }
  return prefix;
}

/** 显示名规范化：去除非法文件名字符（/ \ : * ? " < > |）与首尾空白；去除后为空 → 非法 */
function sanitizeName(name: string): string {
  return name.replace(/[\/\\:*?"<>|]/g, "").trim();
}

/** 定位 header id == sessionId 的会话文件；未找到返回 null */
async function findSessionFile(dir: string, sessionId: string): Promise<string | null> {
  for (const file of collectJsonlFiles(dir)) {
    const header = await readHeader(file);
    if (header !== null && header.type === "session" && header.id === sessionId) return file;
  }
  return null;
}

/** 读取 JSONL 首行 JSON entry（type/id）；无首行或解析失败返回 null */
async function readHeader(file: string): Promise<{ type?: unknown; id?: unknown } | null> {
  const rl = createInterface({ input: createReadStream(file, "utf8"), crlfDelay: Infinity });
  try {
    for await (const line of rl) {
      if (!line.trim()) continue;
      try {
        const entry = JSON.parse(line) as Record<string, unknown>;
        return { type: entry.type, id: entry.id };
      } catch {
        return null;
      }
    }
    return null;
  } finally {
    rl.close();
  }
}

/**
 * POST /api/sessions/rename：改文件名 `<显示名>_<UUID>.jsonl` 保留尾 UUID（= header id）。
 * 校验链：body 合法 → 会话存在（404）→ 显示名合法（400）→ 文件带 `_<UUID>` 尾缀且尾缀等于 header id（400）
 * → 非活跃（mtime > 5min，409）→ 目标无重名（409）→ rename。改名到自身 = 幂等成功。
 */
async function renameSession(dir: string, bodyRaw: string): Promise<ApiResponse> {
  let body: unknown;
  try {
    body = bodyRaw.trim() === "" ? null : JSON.parse(bodyRaw);
  } catch {
    throw new ApiError(400, "Bad Request", "请求体不是合法 JSON");
  }
  const sessionId = (body as Record<string, unknown> | null)?.sessionId;
  const name = (body as Record<string, unknown> | null)?.name;
  if (typeof sessionId !== "string" || sessionId === "") {
    throw new ApiError(400, "Bad Request", "缺少 sessionId");
  }
  if (typeof name !== "string") {
    throw new ApiError(400, "Bad Request", "缺少 name");
  }
  const sanitized = sanitizeName(name);
  if (sanitized === "") {
    throw new ApiError(400, "Bad Request", "显示名非法（去除非法字符后为空）");
  }

  const file = await findSessionFile(dir, sessionId);
  if (file === null) {
    throw new ApiError(404, "Not Found", `会话不存在: ${sessionId}`);
  }
  const base = basename(file).replace(/\.jsonl$/, "");
  const idx = base.lastIndexOf("_");
  if (idx <= 0 || idx === base.length - 1) {
    throw new ApiError(400, "Bad Request", "无法识别会话 UUID（文件名缺少 _<UUID> 尾缀）");
  }
  const tail = base.slice(idx + 1);
  if (tail !== sessionId) {
    throw new ApiError(400, "Bad Request", "会话 UUID 与 header id 不一致");
  }

  const st = statSync(file);
  if (Date.now() - st.mtimeMs <= ACTIVE_MS) {
    throw new ApiError(409, "Conflict", "会话活跃中，稍后再试");
  }

  const target = join(dirname(file), `${sanitized}_${tail}.jsonl`);
  if (target !== file) {
    if (existsSync(target)) {
      throw new ApiError(409, "Conflict", "同名文件已存在");
    }
    renameSync(file, target);
  }
  return { status: 200, body: { ok: true, fileName: basename(target) } };
}
