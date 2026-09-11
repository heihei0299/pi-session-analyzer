/**
 * 会话数据仓（SessionData）— 深模块。
 * 收敛会话目录 → 派生窗口（totals/sessions/requests/groups/period/meta）的唯一 seams。
 * - 单一 query() interface（filter + view）服务 CLI / API / watch 5 处调用
 * - 内聚双时间语义 via branded TimeRange（SessionTimeRange vs MessageTimeRange）
 * - 内部承载：fork 去重（ADR-0001）、cwd 归一缓存、文件级快照缓存、派生、分页排序
 */
import { readdirSync, createReadStream, realpathSync, statSync } from "node:fs";
import { join, resolve, basename } from "node:path";
import { createInterface } from "node:readline";
import {
  addUsage,
  emptyTotals,
  finalizeTotals,
  type Totals,
  type Usage,
  type SessionRow,
  type RequestRow,
  type GroupRow,
  type GroupBy,
  type PeriodRow,
  type Period,
} from "./aggregate.ts";
import {
  parseTimestamp as parseTimestampImpl,
  parseUtcTimestamp as parseUtcTimestampImpl,
  applyTimeRange,
  type TimeRange,
  type SessionTimeRange,
  type MessageTimeRange,
} from "./time-range.ts";

// ---------- 领域类型：会话文件原始数据 ----------
export interface SessionFileData {
  sessionId: string;
  timestamp: string;
  cwd: string;
  fileName?: string;
  /** 是否为子代理会话（路径含 /tasks/，历史称 isTask） */
  isTask?: boolean;
  /** 归属主会话 ID（header.parentSession 非空字符串，子代理会话） */
  parentSessionId?: string;
  firstUserText?: string;
  items: { timestamp: string; model: string; usage: Usage }[];
}

// 时间范围类型自 time-range 深模块 re-export，保持旧 import 路径兼容
export type { SessionTimeRange, MessageTimeRange, TimeRange } from "./time-range.ts";

export interface Filter {
  model?: string;
  cwd?: string;
  timeRange?: TimeRange | null;
}

export type View =
  | { kind: "totals" }
  | { kind: "sessions"; page?: number; size?: number; sortKey?: string; sortDir?: "asc" | "desc" }
  | { kind: "requests"; page?: number; size?: number; sortKey?: string; sortDir?: "asc" | "desc" }
  | { kind: "groups"; by: GroupBy }
  | { kind: "period"; period: Period }
  | { kind: "meta" };

export type SessionRowEnriched = SessionRow & { fileName: string; displayName: string; cwdNorm: string; isTask: boolean; parentSessionId?: string };
export type RequestRowEnriched = RequestRow & { displayName: string };

export interface QueryResultTotals { window: "totals"; totals: Totals }
export interface QueryResultSessions { window: "sessions"; rows: SessionRowEnriched[]; total: number; page?: number; size?: number; totals: Totals }
export interface QueryResultRequests { window: "requests"; rows: RequestRowEnriched[]; total: number; page?: number; size?: number }
export interface QueryResultGroups { window: "totals"; by: GroupBy; rows: GroupRow[] }
export interface QueryResultPeriod { window: "totals"; period: Period; rows: PeriodRow[] }
export interface QueryResultMeta { dir: string; sessionCount: number; dataRange: { since: string | null; until: string | null } }

// ---------- 缓存结构 ----------
interface FileCacheEntry { mtimeMs: number; size: number; data: SessionFileData | null; }
interface DirCache { byFile: Map<string, FileCacheEntry>; data: SessionFileData[]; }

// ---------- 排序/分页常量（自 api.ts 回迁，归属 SessionData 派生层） ----------
const SORT_KEYS = new Set([
  "displayName", "sessionId", "timestamp", "cwd", "model",
  "requests", "input", "output", "cache", "cacheRead", "cacheWrite",
  "reasoning", "cacheRate", "totalTokens", "cost",
]);
const NUMERIC_SORT_KEYS = new Set([
  "requests", "input", "output", "cache", "cacheRead", "cacheWrite",
  "reasoning", "cacheRate", "totalTokens", "cost",
]);

export class SessionData {
  private dirCache = new Map<string, DirCache>();
  private inflight = new Map<string, Promise<SessionFileData[]>>();
  private fileLoaderOverride: ((file: string) => Promise<SessionFileData | null>) | null = null;

  // ---------- 测试注入 ----------
  __setFileLoaderForTest(fn: (file: string) => Promise<SessionFileData | null>): void {
    this.fileLoaderOverride = fn;
  }
  private get fileLoader(): (file: string) => Promise<SessionFileData | null> {
    return this.fileLoaderOverride ?? this.analyzeFile.bind(this);
  }

  // ---------- 文件收集 ----------
  collectJsonlFiles(dir: string): string[] {
    const out: string[] = [];
    const walk = (d: string): void => {
      let entries;
      try {
        entries = readdirSync(d, { withFileTypes: true });
      } catch {
        // 子目录不可读（EACCES/EPERM）或已被删除（ENOENT）时跳过；顶层目录错误由外层感知（走空结果或调用方错误处理）
        return;
      }
      for (const entry of entries) {
        const p = join(d, entry.name);
        if (entry.isDirectory()) walk(p);
        else if (entry.isFile() && entry.name.endsWith(".jsonl")) out.push(p);
      }
    };
    walk(dir);
    return out.sort();
  }

  // 时间解析：委托至 time-range 单引擎（严格校验 + 本地时区）
  parseUtcTimestamp(s: string): number { return parseUtcTimestampImpl(s); }
  parseTimestamp(s: string, endOfDay: boolean): number { return parseTimestampImpl(s, endOfDay); }
  // ---------- cwd 归一 ----------
  normalizeCwd(cwd: string): string {
    const abs = resolve(cwd).replace(/\/+$/, "") || "/";
    try { return realpathSync(abs); } catch { return abs; }
  }
  displayNameOf(fileName: string, firstUserText?: string): string {
    const base = fileName.endsWith(".jsonl") ? fileName.slice(0, -6) : fileName;
    const idx = base.lastIndexOf("_");
    const prefix = idx > 0 && idx < base.length - 1 ? base.slice(0, idx) : fileName;
    if (/^\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}/.test(prefix)) {
      return firstUserText && firstUserText.length > 0 ? firstUserText : prefix;
    }
    return prefix;
  }

  // ---------- 周期键 ----------
  periodKey(timestamp: string, period: Period): string | null {
    const d = new Date(this.parseUtcTimestamp(timestamp));
    if (Number.isNaN(d.getTime())) return null;
    const y = d.getUTCFullYear(); const m = d.getUTCMonth(); const day = d.getUTCDate();
    const pad = (n: number, w: number) => String(n).padStart(w, "0");
    if (period === "day") return `${pad(y, 4)}-${pad(m + 1, 2)}-${pad(day, 2)}`;
    if (period === "month") return `${pad(y, 4)}-${pad(m + 1, 2)}-01`;
    const dow = (d.getUTCDay() + 6) % 7;
    const monday = new Date(Date.UTC(y, m, day - dow));
    return `${pad(monday.getUTCFullYear(), 4)}-${pad(monday.getUTCMonth() + 1, 2)}-${pad(monday.getUTCDate(), 2)}`;
  }

  // ---------- 单文件解析（含 fork 去重） ----------
  async analyzeFile(file: string): Promise<SessionFileData | null> {
    const rl = createInterface({ input: createReadStream(file, "utf8"), crlfDelay: Infinity });
    let header: { id?: unknown; timestamp?: unknown; cwd?: unknown } | null = null;
    const items: SessionFileData["items"] = [];
    let firstUserText: string | undefined;
    let forkTs: number | undefined;
    let parentSessionId: string | undefined;
    try {
      let firstLine = true;
      for await (const line of rl) {
        if (firstLine) {
          firstLine = false;
          const entry = parseJson(line);
          if (entry === null || entry.type !== "session") return null;
          header = { id: entry.id, timestamp: entry.timestamp, cwd: entry.cwd };
          if (typeof entry.parentSession === "string" && entry.parentSession.length > 0) {
            parentSessionId = entry.parentSession;
            // 仅当 parentSession 为文件路径形态（fork 会话，含 / 或 \）时才启用 fork 去重；子代理会话的 parentSession 为纯 sessionId（如 "p1"），不触发去重
            if (entry.parentSession.includes("/") || entry.parentSession.includes("\\")) {
              const t = this.parseUtcTimestamp(typeof entry.timestamp === "string" ? entry.timestamp : "");
              if (!Number.isNaN(t)) forkTs = t;
            }
          }
          continue;
        }
        if (!line.trim()) continue;
        const entry = parseJson(line);
        if (entry === null) continue;
        if (entry.type === "message") {
          const msg = entry.message;
          if (forkTs !== undefined) {
            const itTs = this.parseUtcTimestamp(typeof entry.timestamp === "string" ? entry.timestamp : "");
            if (!Number.isNaN(itTs) && itTs < forkTs) continue;
          }
          if (firstUserText === undefined && msg !== null && typeof msg === "object" && !Array.isArray(msg) && (msg as Record<string, unknown>).role === "user") {
            const content = (msg as Record<string, unknown>).content;
            if (Array.isArray(content)) {
              for (const part of content) {
                if (part !== null && typeof part === "object" && !Array.isArray(part) && (part as Record<string, unknown>).type === "text") {
                  const text = String((part as Record<string, unknown>).text ?? "");
                  if (text.trim()) { firstUserText = text.trim(); break; }
                }
              }
            }
          }
          if (msg !== null && typeof msg === "object" && !Array.isArray(msg) && (msg as Record<string, unknown>).role === "assistant" && (msg as Record<string, unknown>).usage != null) {
            const m = msg as Record<string, unknown>;
            items.push({ timestamp: typeof entry.timestamp === "string" ? entry.timestamp : "", model: typeof m.model === "string" ? m.model : "", usage: m.usage as Usage });
          }
        }
      }
    } finally { rl.close(); }
    if (header === null) return null;
    return { sessionId: typeof header.id === "string" ? header.id : "", timestamp: typeof header.timestamp === "string" ? header.timestamp : "", cwd: typeof header.cwd === "string" ? header.cwd : "", fileName: basename(file), isTask: file.includes("/tasks/") || file.includes("\\tasks\\"), parentSessionId, firstUserText, items };
  }

  async readSessionFiles(dir: string): Promise<SessionFileData[]> {
    const out: SessionFileData[] = [];
    for (const file of this.collectJsonlFiles(dir)) {
      const data = await this.analyzeFile(file);
      if (data !== null) out.push(data);
    }
    return out;
  }

  async readSessionFilesCached(dir: string): Promise<SessionFileData[]> {
    const files = this.collectJsonlFiles(dir);
    const prev = this.dirCache.get(dir);
    const byFile = new Map<string, FileCacheEntry>();
    const changed: string[] = [];
    if (prev !== undefined) {
      for (const f of files) {
        const st = statSync(f);
        const old = prev.byFile.get(f);
        if (old !== undefined && old.mtimeMs === st.mtimeMs && old.size === st.size) byFile.set(f, old);
        else changed.push(f);
      }
    } else { changed.push(...files); }
    const fileSet = new Set(files);
    const deleted = prev !== undefined ? [...prev.byFile.keys()].filter((f) => !fileSet.has(f)) : [];
    if (changed.length === 0 && deleted.length === 0) {
      this.dirCache.set(dir, { byFile, data: prev!.data });
      return prev!.data;
    }
    const key = dir + "|" + changed.map((f) => `${f}:${statSync(f).mtimeMs}:${statSync(f).size}`).join(",");
    const pending = this.inflight.get(key);
    if (pending !== undefined) return pending;
    const p = (async () => {
      const next = new Map<string, FileCacheEntry>();
      const changedSet = new Set(changed);
      for (const f of files) {
        if (changedSet.has(f)) {
          const st = statSync(f);
          next.set(f, { mtimeMs: st.mtimeMs, size: st.size, data: await this.fileLoader(f) });
        } else next.set(f, byFile.get(f)!);
      }
      const data = [...next.values()].filter((e) => e.data !== null).map((e) => e.data!);
      this.dirCache.set(dir, { byFile: next, data });
      return data;
    })().finally(() => this.inflight.delete(key));
    this.inflight.set(key, p);
    return p;
  }

  // 统一过滤：Filter（含 TimeRange 双语义）— 时间部分委托 time-range 单引擎
  applyFilter(files: SessionFileData[], filter: Filter): SessionFileData[] {
    const { model, cwd, timeRange } = filter;
    const normCwd = cwd !== undefined ? this.normalizeCwd(cwd) : undefined;
    const normCwdCache = new Map<string, string>();
    const normOf = (f: SessionFileData): string => {
      let n = normCwdCache.get(f.cwd);
      if (n === undefined) { n = this.normalizeCwd(f.cwd); normCwdCache.set(f.cwd, n); }
      return n;
    };
    let out = files;
    if (normCwd !== undefined) out = out.filter((f) => normOf(f) === normCwd);

    // 时间过滤：单引擎委托
    if (timeRange !== undefined && timeRange !== null) {
      out = applyTimeRange(out, timeRange);
    }

    // 模型过滤（消息级）
    if (model !== undefined) {
      out = out.map((f) => ({ ...f, items: f.items.filter((i) => i.model === model) })).filter((f) => f.items.length > 0);
    }
    return out;
  }

  // 兼容旧 filterFiles 签名：{ model,cwd,since,until } 会话级语义
  filterFiles(files: SessionFileData[], filters: { model?: string; cwd?: string; since?: string; until?: string }): SessionFileData[] {
    const tr: TimeRange | null = (filters.since !== undefined || filters.until !== undefined) ? { kind: "session", since: filters.since, until: filters.until } : null;
    return this.applyFilter(files, { model: filters.model, cwd: filters.cwd, timeRange: tr });
  }

  // ---------- 派生 ----------
  totalsFromFiles(files: SessionFileData[]): Totals {
    const totals = emptyTotals();
    for (const file of files) for (const item of file.items) addUsage(totals, item.usage);
    finalizeTotals(totals);
    return totals;
  }
  sessionRowsFromFiles(files: SessionFileData[]): SessionRow[] {
    return files.map((file) => {
      const totals = emptyTotals(); const models = new Set<string>();
      for (const item of file.items) { addUsage(totals, item.usage); models.add(item.model); }
      finalizeTotals(totals);
      return { sessionId: file.sessionId, timestamp: file.timestamp, cwd: file.cwd, model: models.size === 0 ? "-" : models.size === 1 ? [...models][0] : "mixed", ...totals };
    });
  }
  requestRowsFromFiles(files: SessionFileData[]): RequestRow[] {
    const rows: RequestRow[] = [];
    for (const file of files) for (const item of file.items) {
      const totals = emptyTotals(); addUsage(totals, item.usage); finalizeTotals(totals);
      rows.push({ sessionId: file.sessionId, timestamp: item.timestamp, model: item.model, ...totals });
    }
    return rows;
  }

  // ---------- 详情聚合（#01） ----------
  detailFromFiles(files: SessionFileData[], sessionId: string): {
    session: SessionRowEnriched;
    children: SessionRowEnriched[];
    totals: { main: Totals; merged: Totals; childrenCount: number };
    requests: (RequestRowEnriched & { source: "main" | "child"; sourceSessionId: string })[];
    meta: { hasChildren: boolean };
  } {
    const parent = files.find((f) => f.sessionId === sessionId);
    if (!parent) {
      const err = new Error(`会话不存在: ${sessionId}`) as Error & { status?: number };
      err.status = 404;
      throw err;
    }
    const children = files.filter((f) => f.parentSessionId === sessionId);
    const mainTotals = this.totalsFromFiles([parent]);
    const mergedTotals = this.totalsFromFiles([parent, ...children]);
    const parentRow = this.sessionRowsFromFiles([parent])[0];
    const session: SessionRowEnriched = {
      ...parentRow,
      fileName: parent.fileName ?? "",
      displayName: this.displayNameOf(parent.fileName ?? "", parent.firstUserText),
      cwdNorm: this.normalizeCwd(parent.cwd),
      isTask: parent.isTask ?? false,
    };
    const childrenEnriched: SessionRowEnriched[] = children.map((f) => {
      const r = this.sessionRowsFromFiles([f])[0];
      return {
        ...r,
        fileName: f.fileName ?? "",
        displayName: this.displayNameOf(f.fileName ?? "", f.firstUserText),
        cwdNorm: this.normalizeCwd(f.cwd),
        isTask: f.isTask ?? false,
      };
    });
    const nameBySession = new Map<string, string>();
    nameBySession.set(parent.sessionId, session.displayName);
    for (const c of children) {
      nameBySession.set(c.sessionId, this.displayNameOf(c.fileName ?? "", c.firstUserText));
    }
    const mainReqs = this.requestRowsFromFiles([parent]).map((r) => ({
      ...r,
      displayName: nameBySession.get(r.sessionId) ?? "",
      source: "main" as const,
      sourceSessionId: r.sessionId,
    }));
    const childReqs = this.requestRowsFromFiles(children).map((r) => ({
      ...r,
      displayName: nameBySession.get(r.sessionId) ?? "",
      source: "child" as const,
      sourceSessionId: r.sessionId,
    }));
    const requests = [...mainReqs, ...childReqs].sort((a, b) => a.timestamp.localeCompare(b.timestamp));
    return {
      session,
      children: childrenEnriched,
      totals: { main: mainTotals, merged: mergedTotals, childrenCount: children.length },
      requests,
      meta: { hasChildren: children.length > 0 },
    };
  }

  async queryDetail(dir: string, sessionId: string): Promise<{
    session: SessionRowEnriched;
    children: SessionRowEnriched[];
    totals: { main: Totals; merged: Totals; childrenCount: number };
    requests: (RequestRowEnriched & { source: "main" | "child"; sourceSessionId: string })[];
    meta: { hasChildren: boolean };
  }> {
    const allFiles = await this.readSessionFilesCached(dir);
    return this.detailFromFiles(allFiles, sessionId);
  }

  groupRowsFromFiles(files: SessionFileData[], by: GroupBy): GroupRow[] {
    const byModel = by === "model" || by === "model,cwd"; const byCwd = by === "cwd" || by === "model,cwd";
    const normCwdCache = new Map<string, string>();
    const normOf = (f: SessionFileData): string => {
      let n = normCwdCache.get(f.cwd);
      if (n === undefined) { n = this.normalizeCwd(f.cwd); normCwdCache.set(f.cwd, n); }
      return n;
    };
    const map = new Map<string, GroupRow>();
    for (const file of files) for (const item of file.items) {
      const keyParts: string[] = []; const row: GroupRow = { ...emptyTotals() };
      if (byModel) { row.model = item.model; keyParts.push(`m:${item.model}`); }
      if (byCwd) { row.cwd = normOf(file); keyParts.push(`c:${row.cwd}`); }
      const key = keyParts.join("|"); let g = map.get(key); if (!g) { g = row; map.set(key, g); } addUsage(g, item.usage);
    }
    for (const g of map.values()) finalizeTotals(g);
    return [...map.values()];
  }
  periodRowsFromFiles(files: SessionFileData[], period: Period): PeriodRow[] {
    const map = new Map<string, PeriodRow>();
    // period 按每条 usage event 的消息 timestamp 归属；header timestamp 只用于会话级。
    for (const file of files) for (const item of file.items) {
      const key = this.periodKey(item.timestamp, period); if (key === null) continue;
      let g = map.get(key); if (!g) { g = { period: key, ...emptyTotals() }; map.set(key, g); }
      addUsage(g, item.usage);
    }
    for (const g of map.values()) finalizeTotals(g);
    return [...map.values()].sort((a, b) => a.period.localeCompare(b.period));
  }

  buildMeta(dir: string, files: SessionFileData[]): { dir: string; sessionCount: number; dataRange: { since: string | null; until: string | null } } {
    const timestamps = files.map((f) => f.timestamp).filter((t) => !Number.isNaN(this.parseUtcTimestamp(t)));
    const dataRange = timestamps.length === 0 ? { since: null, until: null } : { since: timestamps.reduce((a, b) => (a < b ? a : b)), until: timestamps.reduce((a, b) => (a > b ? a : b)) };
    return { dir, sessionCount: files.length, dataRange };
  }

  // ---------- 排序/分页（原 api.ts） ----------
  applySort<T extends Record<string, unknown>>(rows: T[], sortKey: string, sortDir: "asc" | "desc"): T[] {
    const dir = sortDir === "asc" ? 1 : -1;
    const valueOf = (r: T): unknown => sortKey === "cache" ? Number(r.cacheRead ?? 0) + Number(r.cacheWrite ?? 0) : r[sortKey];
    return [...rows].sort((a, b) => {
      const va = valueOf(a); const vb = valueOf(b);
      if (NUMERIC_SORT_KEYS.has(sortKey)) return (Number(va) - Number(vb)) * dir;
      return String(va).localeCompare(String(vb)) * dir;
    });
  }
  paginate<T extends Record<string, unknown>>(rows: T[], page?: number, size?: number, sortKey?: string, sortDir?: "asc" | "desc"): { rows: T[]; total: number; page?: number; size?: number } {
    let out = rows;
    if (sortKey && sortDir) {
      if (!SORT_KEYS.has(sortKey)) throw new Error(`未知排序字段: ${sortKey}`);
      if (sortDir !== "asc" && sortDir !== "desc") throw new Error(`未知排序方向: ${sortDir}`);
      out = this.applySort(out, sortKey, sortDir);
    }
    const total = out.length;
    if (page === undefined && size === undefined) return { rows: out, total };
    if (page === undefined || size === undefined) throw new Error("page 与 size 必须同时提供");
    if (!Number.isInteger(page) || page < 1) throw new Error(`无效 page: ${page}`);
    if (!Number.isInteger(size) || size < 1 || size > 200) throw new Error(`无效 size: ${size}`);
    const start = (page - 1) * size;
    return { rows: out.slice(start, start + size), total, page, size };
  }

  // ---------- 统一 query interface ----------
  async query(dir: string, filter: Filter, view: View): Promise<QueryResultTotals | QueryResultSessions | QueryResultRequests | QueryResultGroups | QueryResultPeriod | QueryResultMeta> {
    if (view.kind === "meta") {
      const files = await this.readSessionFilesCached(dir);
      return this.buildMeta(dir, files);
    }
    // 数据读取：统一走缓存
    const allFiles = await this.readSessionFilesCached(dir);
    const filtered = this.applyFilter(allFiles, filter);

    switch (view.kind) {
      case "totals": {
        return { window: "totals", totals: this.totalsFromFiles(filtered) };
      }
      case "sessions": {
        const rowsRaw = this.sessionRowsFromFiles(filtered).map((r, i) => {
          const f = filtered[i];
          const fileName = f.fileName ?? "";
          return { ...r, fileName, displayName: this.displayNameOf(fileName, f.firstUserText), cwdNorm: this.normalizeCwd(f.cwd), isTask: f.isTask ?? false, parentSessionId: f.parentSessionId } as SessionRowEnriched;
        });
        // 转为 Record 以复用 paginate 的排序（需将 enriched 视为 Record）
        const paged = this.paginate(rowsRaw as unknown as Record<string, unknown>[], view.page, view.size, view.sortKey, view.sortDir);
        return { window: "sessions", rows: paged.rows as unknown as SessionRowEnriched[], total: paged.total, page: paged.page, size: paged.size, totals: this.totalsFromFiles(filtered) };
      }
      case "requests": {
        const nameBySession = new Map<string, string>();
        for (const f of filtered) nameBySession.set(f.sessionId, this.displayNameOf(f.fileName ?? "", f.firstUserText));
        const rowsRaw = this.requestRowsFromFiles(filtered).map((r) => ({ ...r, displayName: nameBySession.get(r.sessionId) ?? "" } as RequestRowEnriched));
        const paged = this.paginate(rowsRaw as unknown as Record<string, unknown>[], view.page, view.size, view.sortKey, view.sortDir);
        return { window: "requests", rows: paged.rows as unknown as RequestRowEnriched[], total: paged.total, page: paged.page, size: paged.size };
      }
      case "groups": {
        return { window: "totals", by: view.by, rows: this.groupRowsFromFiles(filtered, view.by) };
      }
      case "period": {
        return { window: "totals", period: view.period, rows: this.periodRowsFromFiles(filtered, view.period) };
      }
    }
  }
}

function parseJson(line: string): Record<string, unknown> | null {
  try {
    const v = JSON.parse(line) as unknown;
    if (v !== null && typeof v === "object" && !Array.isArray(v)) return v as Record<string, unknown>;
    return null;
  } catch { return null; }
}

// 默认单例
export const defaultSessionData = new SessionData();

// ---------- 兼容旧 analyze.ts 具名导出：委托至默认单例（保留 1 版本） ----------
export function collectJsonlFiles(dir: string): string[] { return defaultSessionData.collectJsonlFiles(dir); }
export function parseUtcTimestamp(s: string): number { return defaultSessionData.parseUtcTimestamp(s); }
export function parseTimestamp(s: string, endOfDay: boolean): number { return defaultSessionData.parseTimestamp(s, endOfDay); }
export function normalizeCwd(cwd: string): string { return defaultSessionData.normalizeCwd(cwd); }
export function periodKey(timestamp: string, period: Period): string | null { return defaultSessionData.periodKey(timestamp, period); }
export function groupRowsFromFiles(files: SessionFileData[], by: GroupBy): GroupRow[] { return defaultSessionData.groupRowsFromFiles(files, by); }
export function periodRowsFromFiles(files: SessionFileData[], period: Period): PeriodRow[] { return defaultSessionData.periodRowsFromFiles(files, period); }
export function filterFiles(files: SessionFileData[], filters: { model?: string; cwd?: string; since?: string; until?: string }): SessionFileData[] { return defaultSessionData.filterFiles(files, filters); }
export function totalsFromFiles(files: SessionFileData[]): Totals { return defaultSessionData.totalsFromFiles(files); }
export function sessionRowsFromFiles(files: SessionFileData[]): SessionRow[] { return defaultSessionData.sessionRowsFromFiles(files); }
export function requestRowsFromFiles(files: SessionFileData[]): RequestRow[] { return defaultSessionData.requestRowsFromFiles(files); }
export async function analyzeFile(file: string): Promise<SessionFileData | null> { return defaultSessionData.analyzeFile(file); }
export async function readSessionFiles(dir: string): Promise<SessionFileData[]> { return defaultSessionData.readSessionFiles(dir); }
export async function readSessionFilesCached(dir: string): Promise<SessionFileData[]> { return defaultSessionData.readSessionFilesCached(dir); }
export function __setFileLoaderForTest(fn: (file: string) => Promise<SessionFileData | null>): void { defaultSessionData.__setFileLoaderForTest(fn); }
