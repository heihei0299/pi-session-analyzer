/**
 * HTTP API 层：/api/* 端点处理。
 * 每次请求全量 readSessionFiles(dir) → filterFiles（model/cwd/since/until）→ 按端点派生；
 * 响应字段复用 serialize.ts 的 *ToObject 转换（与 CLI 结构化输出一致）。
 * 错误：统一 JSON 错误体 { error, detail } + 400/404/409/500。
 */
import {
  readSessionFiles,
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
      const filtered = await loadFiltered(dir, params);
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
          displayName: displayNameOf(fileName),
          cwdNorm: normalizeCwd(f.cwd),
        };
      });
      return { status: 200, body: { window: "sessions", rows } };
    }
    if (method === "POST" && pathname === "/api/sessions/rename") {
      return await renameSession(dir, body);
    }
    if (method === "GET" && pathname === "/api/requests") {
      const filtered = await loadFiltered(dir, params);
      return { status: 200, body: { window: "requests", rows: requestRowsFromFiles(filtered).map(requestToObject) } };
    }
    if (method === "GET" && pathname === "/api/groups") {
      const by = parseGroupBy(params);
      const filtered = await loadFiltered(dir, params);
      return { status: 200, body: { window: "totals", by, rows: groupRowsFromFiles(filtered, by).map(groupToObject) } };
    }
    if (method === "GET" && pathname === "/api/period") {
      const period = parsePeriod(params);
      const filtered = await loadFiltered(dir, params);
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
    files = await readSessionFiles(dir);
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

/** 显示名派生：去尾 `_<UUID>.jsonl` 的前缀；无 `_` 尾缀 → 原始文件名 */
function displayNameOf(fileName: string): string {
  const base = fileName.endsWith(".jsonl") ? fileName.slice(0, -6) : fileName;
  const idx = base.lastIndexOf("_");
  if (idx > 0 && idx < base.length - 1) return base.slice(0, idx);
  return fileName;
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
