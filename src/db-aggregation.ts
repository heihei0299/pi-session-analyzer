/**
 * DB 聚合与剪枝（直切）。
 * - 读路径一律走 proxy_request_logs（+ usage_daily_rollups）的 SQL 聚合，消息级语义
 * - withDirDb：按目录隔离的内存库（全量同步后聚合），CLI/API 读路径统一入口
 */
import type { Database } from "./db.ts";
import { defaultSessionData } from "./session-data.ts";
import { collectJsonlFiles } from "./session-data.ts";
import { collectPiJsonlFiles, resolvePiSessionRoot, getPiNativeSessionDir } from "./pi-discovery.ts";
import { syncPiUsage } from "./pi-sync.ts";
import { Database as DbClass } from "./db.ts";
import { emptyTotals, finalizeTotals, type Totals, type GroupRow, type GroupBy, type Period, type PeriodRow } from "./aggregate.ts";
import type { SessionRowEnriched, RequestRowEnriched } from "./session-data.ts";

export interface DbFilter {
  since?: string;
  until?: string;
  model?: string;
  cwd?: string;
  sessionIds?: string[];
}

function parseSinceUntil(since?: string, until?: string): { sinceTs?: number; untilTs?: number } {
  let sinceTs: number | undefined;
  let untilTs: number | undefined;
  if (since) {
    const t = Date.parse(since.includes("T") ? since : `${since}T00:00:00`);
    if (!Number.isNaN(t)) sinceTs = Math.floor(t / 1000);
  }
  if (until) {
    const t = Date.parse(until.includes("T") ? until : `${until}T23:59:59`);
    if (!Number.isNaN(t)) untilTs = Math.floor(t / 1000);
  }
  return { sinceTs, untilTs };
}

/** cwd 过滤：规范化后匹配，返回命中的原始 cwd 列表（null = 无过滤） */
function matchingRawCwds(db: Database, cwdFilter: string): string[] {
  const norm = defaultSessionData.normalizeCwd(cwdFilter);
  const rows = db.prepare(`SELECT DISTINCT cwd FROM pi_sessions`).all() as { cwd: string }[];
  const out: string[] = [];
  for (const r of rows) {
    try {
      if (defaultSessionData.normalizeCwd(r.cwd) === norm) out.push(r.cwd);
    } catch {
      if (r.cwd === cwdFilter) out.push(r.cwd);
    }
  }
  return out;
}

/** 读路径统一入口：持久库（全量同步后聚合）+ 内存隔离（测试 fixture） */
export async function withDirDb<T>(dir: string, fn: (db: Database) => Promise<T> | T): Promise<T> {
  const isTest = dir.includes("token-analyzer") || dir.includes("ta-") || dir.startsWith("/tmp/") || dir.startsWith("/private/tmp/");
  const db = isTest ? await DbClass.memory() : await DbClass.getInstance();
  try {
    const piNative = getPiNativeSessionDir();
    const { root, layout } = resolvePiSessionRoot({ envDb: process.env.PI_CODING_AGENT_SESSION_DIR, defaultRoot: dir, piConfig: piNative });
    let files = collectPiJsonlFiles(root, layout);
    // 测试 fixture 扁平文件：projectDirectories 下无文件时回退到递归收集（仅 isTest）
    if (isTest && files.length === 0) {
      files = collectJsonlFiles(dir);
    }
    await syncPiUsage(db, files);
    if (!isTest) rollupAndPrune(db, 30);
    return await fn(db);
  } finally {
    await db.close();
  }
}

interface SumRow {
  requests: number;
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  reasoning: number;
  cost: number;
}

function sumsToTotals(row: SumRow, roll: SumRow): Totals {
  const totals = emptyTotals();
  totals.requests = row.requests + roll.requests;
  totals.input = row.input + roll.input;
  totals.output = row.output + roll.output;
  totals.cacheRead = row.cacheRead + roll.cacheRead;
  totals.cacheWrite = row.cacheWrite + roll.cacheWrite;
  totals.reasoning = row.reasoning + roll.reasoning;
  totals.cost = row.cost + roll.cost;
  finalizeTotals(totals);
  return totals;
}

export function queryTotals(db: Database, filter: DbFilter): Totals {
  const { sinceTs, untilTs } = parseSinceUntil(filter.since, filter.until);
  const rawCwds = filter.cwd !== undefined ? matchingRawCwds(db, filter.cwd) : null;
  let sql = `SELECT COUNT(*) as requests, COALESCE(SUM(input_tokens),0) as input, COALESCE(SUM(output_tokens),0) as output, COALESCE(SUM(cache_read_tokens),0) as cacheRead, COALESCE(SUM(cache_creation_tokens),0) as cacheWrite, COALESCE(SUM(reasoning_tokens),0) as reasoning, COALESCE(SUM(CAST(total_cost_usd AS REAL)),0) as cost FROM proxy_request_logs WHERE app_type='pi' AND data_source='pi_session'`;
  const params: unknown[] = [];
  if (filter.model) {
    sql += ` AND model = ?`;
    params.push(filter.model);
  }
  if (rawCwds !== null) {
    if (rawCwds.length === 0) {
      const t = emptyTotals();
      finalizeTotals(t);
      return t;
    }
    sql += ` AND cwd IN (${rawCwds.map(() => "?").join(",")})`;
    params.push(...rawCwds);
  }
  if (filter.sessionIds !== undefined) {
    if (filter.sessionIds.length === 0) {
      const t = emptyTotals();
      finalizeTotals(t);
      return t;
    }
    sql += ` AND session_id IN (${filter.sessionIds.map(() => "?").join(",")})`;
    params.push(...filter.sessionIds);
  }
  if (sinceTs !== undefined) {
    sql += ` AND created_at >= ?`;
    params.push(sinceTs);
  }
  if (untilTs !== undefined) {
    sql += ` AND created_at <= ?`;
    params.push(untilTs);
  }
  const row = db.prepare(sql).get(...(params as string[])) as unknown as SumRow;
  // UNION rollups：按 date 范围聚合后叠加（reasoning/cwd 不可恢复，记 0）
  let rollupSql = `SELECT COALESCE(SUM(request_count),0) as requests, COALESCE(SUM(input_tokens),0) as input, COALESCE(SUM(output_tokens),0) as output, COALESCE(SUM(cache_read_tokens),0) as cacheRead, COALESCE(SUM(cache_creation_tokens),0) as cacheWrite, 0 as reasoning, COALESCE(SUM(CAST(total_cost_usd AS REAL)),0) as cost FROM usage_daily_rollups WHERE app_type='pi'`;
  const rollupParams: unknown[] = [];
  if (filter.model) {
    rollupSql += ` AND model = ?`;
    rollupParams.push(filter.model);
  }
  if (filter.since) {
    rollupSql += ` AND date >= ?`;
    rollupParams.push(filter.since.slice(0, 10));
  }
  if (filter.until) {
    rollupSql += ` AND date <= ?`;
    rollupParams.push(filter.until.slice(0, 10));
  }
  const roll = db.prepare(rollupSql).get(...(rollupParams as string[])) as unknown as SumRow;
  return sumsToTotals(row, roll);
}

export function queryGroups(db: Database, by: GroupBy, filter: DbFilter): GroupRow[] {
  const { sinceTs, untilTs } = parseSinceUntil(filter.since, filter.until);
  const rawCwds = filter.cwd !== undefined ? matchingRawCwds(db, filter.cwd) : null;
  if (rawCwds !== null && rawCwds.length === 0) return [];
  const byModel = by === "model" || by === "model,cwd";
  const byCwd = by === "cwd" || by === "model,cwd";
  const selectKeys = [...(byModel ? ["model"] : []), ...(byCwd ? ["cwd"] : [])].join(", ");
  let sql = `SELECT ${selectKeys}, COUNT(*) as requests, COALESCE(SUM(input_tokens),0) as input, COALESCE(SUM(output_tokens),0) as output, COALESCE(SUM(cache_read_tokens),0) as cacheRead, COALESCE(SUM(cache_creation_tokens),0) as cacheWrite, COALESCE(SUM(reasoning_tokens),0) as reasoning, COALESCE(SUM(CAST(total_cost_usd AS REAL)),0) as cost FROM proxy_request_logs WHERE app_type='pi' AND data_source='pi_session'`;
  const params: unknown[] = [];
  if (filter.model) {
    sql += ` AND model = ?`;
    params.push(filter.model);
  }
  if (rawCwds !== null) {
    sql += ` AND cwd IN (${rawCwds.map(() => "?").join(",")})`;
    params.push(...rawCwds);
  }
  if (sinceTs !== undefined) {
    sql += ` AND created_at >= ?`;
    params.push(sinceTs);
  }
  if (untilTs !== undefined) {
    sql += ` AND created_at <= ?`;
    params.push(untilTs);
  }
  sql += ` GROUP BY ${selectKeys}`;
  type Row = SumRow & { model?: string; cwd?: string };
  const rows = db.prepare(sql).all(...(params as string[])) as unknown as Row[];
  // rollups 仅 model 维度可合并；cwd 维度不参与（无 cwd 列）
  if (byModel && !byCwd) {
    let rollupSql = `SELECT model, COALESCE(SUM(request_count),0) as requests, COALESCE(SUM(input_tokens),0) as input, COALESCE(SUM(output_tokens),0) as output, COALESCE(SUM(cache_read_tokens),0) as cacheRead, COALESCE(SUM(cache_creation_tokens),0) as cacheWrite, 0 as reasoning, COALESCE(SUM(CAST(total_cost_usd AS REAL)),0) as cost FROM usage_daily_rollups WHERE app_type='pi'`;
    const rollupParams: unknown[] = [];
    if (filter.model) {
      rollupSql += ` AND model = ?`;
      rollupParams.push(filter.model);
    }
    if (filter.since) {
      rollupSql += ` AND date >= ?`;
      rollupParams.push(filter.since.slice(0, 10));
    }
    if (filter.until) {
      rollupSql += ` AND date <= ?`;
      rollupParams.push(filter.until.slice(0, 10));
    }
    rollupSql += ` GROUP BY model`;
    const rollups = db.prepare(rollupSql).all(...(rollupParams as string[])) as unknown as Row[];
    const map = new Map<string, SumRow & { model?: string; cwd?: string }>();
    for (const r of rows) map.set(r.model ?? "", { ...r });
    for (const r of rollups) {
      const cur = map.get(r.model ?? "") ?? { requests: 0, input: 0, output: 0, cacheRead: 0, cacheWrite: 0, reasoning: 0, cost: 0, model: r.model };
      cur.requests += r.requests;
      cur.input += r.input;
      cur.output += r.output;
      cur.cacheRead += r.cacheRead;
      cur.cacheWrite += r.cacheWrite;
      cur.cost += r.cost;
      map.set(r.model ?? "", cur);
    }
    const out: GroupRow[] = [];
    for (const r of map.values()) {
      const t = emptyTotals();
      t.requests = r.requests;
      t.input = r.input;
      t.output = r.output;
      t.cacheRead = r.cacheRead;
      t.cacheWrite = r.cacheWrite;
      t.reasoning = r.reasoning;
      t.cost = r.cost;
      finalizeTotals(t);
      out.push({ model: r.model, ...t });
    }
    return out;
  }
  // cwd / model,cwd：规范化合并
  const map = new Map<string, GroupRow>();
  for (const r of rows) {
    const normCwd = r.cwd !== undefined ? defaultSessionData.normalizeCwd(r.cwd) : undefined;
    const key = `${byModel ? `m:${r.model}` : ""}|${byCwd ? `c:${normCwd}` : ""}`;
    let g = map.get(key);
    if (!g) {
      g = { ...emptyTotals() };
      if (byModel) g.model = r.model;
      if (byCwd) g.cwd = normCwd;
      map.set(key, g);
    }
    g.requests += r.requests;
    g.input += r.input;
    g.output += r.output;
    g.cacheRead += r.cacheRead;
    g.cacheWrite += r.cacheWrite;
    g.reasoning += r.reasoning;
    g.cost += r.cost;
  }
  for (const g of map.values()) finalizeTotals(g);
  return [...map.values()];
}

interface SessionMeta {
  sessionId: string;
  headerTs: string;
  cwd: string;
  fileName: string;
  displayName: string;
  isTask: boolean;
  parentSessionId?: string;
}

function loadSessionMetas(db: Database): SessionMeta[] {
  const rows = db.prepare(`SELECT session_id, header_ts, cwd, file_name, display_name, is_task, parent_session_id FROM pi_sessions`).all() as {
    session_id: string;
    header_ts: string;
    cwd: string;
    file_name: string;
    display_name: string;
    is_task: number;
    parent_session_id: string | null;
  }[];
  return rows.map((r) => ({
    sessionId: r.session_id,
    headerTs: r.header_ts,
    cwd: r.cwd,
    fileName: r.file_name,
    displayName: r.display_name,
    isTask: r.is_task === 1,
    parentSessionId: r.parent_session_id ?? undefined,
  }));
}

interface RequestLogRow extends SumRow {
  session_id: string;
  model: string;
  created_at: number;
  timestamp_text: string;
}

/** 会话作用域：cwd 会话级 + model/time/sessionIds 请求级 */
function scopedSessionIds(
  db: Database,
  metas: SessionMeta[],
  filter: DbFilter,
): { ids: Set<string>; metaById: Map<string, SessionMeta> } {
  const metaById = new Map<string, SessionMeta>();
  let normCwd: string | undefined;
  if (filter.cwd !== undefined) normCwd = defaultSessionData.normalizeCwd(filter.cwd);
  for (const m of metas) {
    if (filter.sessionIds !== undefined && !filter.sessionIds.includes(m.sessionId)) continue;
    if (normCwd !== undefined) {
      try {
        if (defaultSessionData.normalizeCwd(m.cwd) !== normCwd) continue;
      } catch {
        if (m.cwd !== filter.cwd) continue;
      }
    }
    metaById.set(m.sessionId, m);
  }
  const hasRequestFilter = filter.model !== undefined || filter.since !== undefined || filter.until !== undefined;
  if (!hasRequestFilter) return { ids: new Set(metaById.keys()), metaById };
  const { sinceTs, untilTs } = parseSinceUntil(filter.since, filter.until);
  let sql = `SELECT DISTINCT session_id FROM proxy_request_logs WHERE app_type='pi' AND data_source='pi_session'`;
  const params: unknown[] = [];
  if (filter.model) {
    sql += ` AND model = ?`;
    params.push(filter.model);
  }
  if (sinceTs !== undefined) {
    sql += ` AND created_at >= ?`;
    params.push(sinceTs);
  }
  if (untilTs !== undefined) {
    sql += ` AND created_at <= ?`;
    params.push(untilTs);
  }
  const rows = db.prepare(sql).all(...(params as string[])) as { session_id: string }[];
  const ids = new Set<string>();
  for (const r of rows) if (metaById.has(r.session_id)) ids.add(r.session_id);
  return { ids, metaById };
}

function sumRequests(rows: RequestLogRow[]): Totals {
  const t = emptyTotals();
  for (const r of rows) {
    t.requests += 1;
    t.input += r.input;
    t.output += r.output;
    t.cacheRead += r.cacheRead;
    t.cacheWrite += r.cacheWrite;
    t.reasoning += r.reasoning;
    t.cost += r.cost;
  }
  finalizeTotals(t);
  return t;
}

function loadMatchingRequests(db: Database, sessionIds: Set<string>, filter: DbFilter): Map<string, RequestLogRow[]> {
  const bySession = new Map<string, RequestLogRow[]>();
  if (sessionIds.size === 0) return bySession;
  const { sinceTs, untilTs } = parseSinceUntil(filter.since, filter.until);
  const ids = [...sessionIds];
  let sql = `SELECT session_id, model, created_at, timestamp_text, input_tokens as input, output_tokens as output, cache_read_tokens as cacheRead, cache_creation_tokens as cacheWrite, reasoning_tokens as reasoning, CAST(total_cost_usd AS REAL) as cost FROM proxy_request_logs WHERE app_type='pi' AND data_source='pi_session' AND session_id IN (${ids.map(() => "?").join(",")})`;
  const params: unknown[] = [...ids];
  if (filter.model) {
    sql += ` AND model = ?`;
    params.push(filter.model);
  }
  if (sinceTs !== undefined) {
    sql += ` AND created_at >= ?`;
    params.push(sinceTs);
  }
  if (untilTs !== undefined) {
    sql += ` AND created_at <= ?`;
    params.push(untilTs);
  }
  sql += ` ORDER BY created_at ASC`;
  const rows = db.prepare(sql).all(...(params as string[])) as unknown as RequestLogRow[];
  for (const r of rows) {
    const arr = bySession.get(r.session_id) ?? [];
    arr.push(r);
    bySession.set(r.session_id, arr);
  }
  return bySession;
}

function isoOf(r: RequestLogRow): string {
  if (r.timestamp_text) return r.timestamp_text;
  return new Date(r.created_at * 1000).toISOString();
}

export function querySessions(
  db: Database,
  filter: DbFilter,
  paging?: { page?: number; size?: number; sortKey?: string; sortDir?: "asc" | "desc" },
): { rows: SessionRowEnriched[]; total: number; page?: number; size?: number; totals: Totals } {
  const metas = loadSessionMetas(db);
  const { ids, metaById } = scopedSessionIds(db, metas, filter);
  const bySession = loadMatchingRequests(db, ids, filter);
  const rows: SessionRowEnriched[] = [];
  for (const sid of ids) {
    const m = metaById.get(sid)!;
    const reqs = bySession.get(sid) ?? [];
    // 有 model/time 过滤时，无匹配请求的会话不列出（与 applyFilter 一致）
    const hasRequestFilter = filter.model !== undefined || filter.since !== undefined || filter.until !== undefined;
    if (hasRequestFilter && reqs.length === 0) continue;
    const t = sumRequests(reqs);
    const models = new Set(reqs.map((r) => r.model));
    rows.push({
      sessionId: sid,
      timestamp: m.headerTs,
      cwd: m.cwd,
      model: models.size === 0 ? "-" : models.size === 1 ? [...models][0] : "mixed",
      ...t,
      fileName: m.fileName,
      displayName: m.displayName,
      cwdNorm: defaultSessionData.normalizeCwd(m.cwd),
      isTask: m.isTask,
      parentSessionId: m.parentSessionId,
    });
  }
  const totals = (() => {
    const t = emptyTotals();
    for (const r of rows) {
      t.requests += r.requests;
      t.input += r.input;
      t.output += r.output;
      t.cacheRead += r.cacheRead;
      t.cacheWrite += r.cacheWrite;
      t.reasoning += r.reasoning;
      t.cost += r.cost;
    }
    finalizeTotals(t);
    return t;
  })();
  const paged = defaultSessionData.paginate(
    rows as unknown as Record<string, unknown>[],
    paging?.page,
    paging?.size,
    paging?.sortKey,
    paging?.sortDir as "asc" | "desc" | undefined,
  );
  return { rows: paged.rows as unknown as SessionRowEnriched[], total: paged.total, page: paged.page, size: paged.size, totals };
}

export function queryRequests(
  db: Database,
  filter: DbFilter,
  paging?: { page?: number; size?: number; sortKey?: string; sortDir?: "asc" | "desc" },
): { rows: RequestRowEnriched[]; total: number; page?: number; size?: number } {
  const metas = loadSessionMetas(db);
  const { ids, metaById } = scopedSessionIds(db, metas, filter);
  const bySession = loadMatchingRequests(db, ids, filter);
  const rows: RequestRowEnriched[] = [];
  for (const sid of ids) {
    const m = metaById.get(sid)!;
    for (const r of bySession.get(sid) ?? []) {
      const t = emptyTotals();
      t.requests = 1;
      t.input = r.input;
      t.output = r.output;
      t.cacheRead = r.cacheRead;
      t.cacheWrite = r.cacheWrite;
      t.reasoning = r.reasoning;
      t.cost = r.cost;
      finalizeTotals(t);
      rows.push({
        sessionId: sid,
        timestamp: isoOf(r),
        model: r.model,
        ...t,
        displayName: m.displayName,
      });
    }
  }
  const paged = defaultSessionData.paginate(
    rows as unknown as Record<string, unknown>[],
    paging?.page,
    paging?.size,
    paging?.sortKey,
    paging?.sortDir as "asc" | "desc" | undefined,
  );
  return { rows: paged.rows as unknown as RequestRowEnriched[], total: paged.total, page: paged.page, size: paged.size };
}

function periodKeyUtc(createdAt: number, period: Period): string {
  const d = new Date(createdAt * 1000);
  const pad = (n: number, w: number): string => String(n).padStart(w, "0");
  const y = d.getUTCFullYear();
  const m = d.getUTCMonth();
  const day = d.getUTCDate();
  if (period === "day") return `${pad(y, 4)}-${pad(m + 1, 2)}-${pad(day, 2)}`;
  if (period === "month") return `${pad(y, 4)}-${pad(m + 1, 2)}-01`;
  const dow = (d.getUTCDay() + 6) % 7;
  const monday = new Date(Date.UTC(y, m, day - dow));
  return `${pad(monday.getUTCFullYear(), 4)}-${pad(monday.getUTCMonth() + 1, 2)}-${pad(monday.getUTCDate(), 2)}`;
}

export function queryPeriod(db: Database, period: Period, filter: DbFilter): { period: Period; rows: PeriodRow[] } {
  const metas = loadSessionMetas(db);
  const { ids } = scopedSessionIds(db, metas, filter);
  const bySession = loadMatchingRequests(db, ids, filter);
  const map = new Map<string, Totals>();
  for (const reqs of bySession.values()) {
    for (const r of reqs) {
      const key = periodKeyUtc(r.created_at, period);
      let g = map.get(key);
      if (!g) {
        g = emptyTotals();
        map.set(key, g);
      }
      g.requests += 1;
      g.input += r.input;
      g.output += r.output;
      g.cacheRead += r.cacheRead;
      g.cacheWrite += r.cacheWrite;
      g.reasoning += r.reasoning;
      g.cost += r.cost;
    }
  }
  const rows: PeriodRow[] = [...map.entries()]
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([key, g]) => {
      finalizeTotals(g);
      return { period: key, ...g };
    });
  return { period, rows };
}

export function queryMeta(db: Database, dir: string): { dir: string; sessionCount: number; dataRange: { since: string | null; until: string | null } } {
  const metas = loadSessionMetas(db);
  const timestamps = metas
    .map((m) => m.headerTs)
    .filter((t) => !Number.isNaN(defaultSessionData.parseUtcTimestamp(t)));
  const dataRange =
    timestamps.length === 0
      ? { since: null as string | null, until: null as string | null }
      : { since: timestamps.reduce((a, b) => (a < b ? a : b)), until: timestamps.reduce((a, b) => (a > b ? a : b)) };
  return { dir, sessionCount: metas.length, dataRange };
}

export function queryDetail(
  db: Database,
  sessionId: string,
): {
  session: SessionRowEnriched;
  children: SessionRowEnriched[];
  totals: { main: Totals; merged: Totals; childrenCount: number };
  requests: (RequestRowEnriched & { source: "main" | "child"; sourceSessionId: string })[];
  meta: { hasChildren: boolean };
} {
  const metas = loadSessionMetas(db);
  const parent = metas.find((m) => m.sessionId === sessionId);
  if (!parent) {
    const err = new Error(`会话不存在: ${sessionId}`) as Error & { status?: number };
    err.status = 404;
    throw err;
  }
  const children = metas.filter((m) => m.parentSessionId === sessionId);
  const allIds = [parent.sessionId, ...children.map((c) => c.sessionId)];
  const mainTotals = queryTotals(db, { sessionIds: [parent.sessionId] });
  const mergedTotals = queryTotals(db, { sessionIds: allIds });
  const sessRes = querySessions(db, { sessionIds: allIds }, {});
  const rowById = new Map(sessRes.rows.map((r) => [r.sessionId, r]));
  const session = rowById.get(parent.sessionId)!;
  const childrenRows = children.map((c) => rowById.get(c.sessionId)!).filter(Boolean);
  const reqRes = queryRequests(db, { sessionIds: allIds }, {});
  const childIds = new Set(children.map((c) => c.sessionId));
  const requests = reqRes.rows
    .map((r) => ({
      ...r,
      source: (childIds.has(r.sessionId) ? "child" : "main") as "main" | "child",
      sourceSessionId: r.sessionId,
    }))
    .sort((a, b) => a.timestamp.localeCompare(b.timestamp));
  return {
    session,
    children: childrenRows,
    totals: { main: mainTotals, merged: mergedTotals, childrenCount: children.length },
    requests,
    meta: { hasChildren: children.length > 0 },
  };
}

export function rollupAndPrune(db: Database, days: number): { rolled: number } {
  const cutoff = Math.floor(Date.now() / 1000) - days * 86400;
  const rows = db.prepare(`SELECT date(created_at, 'unixepoch') as date, app_type, provider_id, model, request_model, pricing_model, COUNT(*) as request_count, SUM(CASE WHEN status_code BETWEEN 200 AND 299 THEN 1 ELSE 0 END) as success_count, COALESCE(SUM(input_tokens),0) as input_tokens, COALESCE(SUM(output_tokens),0) as output_tokens, COALESCE(SUM(cache_read_tokens),0) as cache_read_tokens, COALESCE(SUM(cache_creation_tokens),0) as cache_creation_tokens, COALESCE(SUM(CAST(total_cost_usd AS REAL)),0) as total_cost FROM proxy_request_logs WHERE created_at < ? GROUP BY date, app_type, provider_id, model, request_model, pricing_model`).all(cutoff) as { date: string; app_type: string; provider_id: string; model: string; request_model: string; pricing_model: string; request_count: number; success_count: number; input_tokens: number; output_tokens: number; cache_read_tokens: number; cache_creation_tokens: number; total_cost: number }[];
  for (const r of rows) {
    db.prepare(`INSERT OR REPLACE INTO usage_daily_rollups (date, app_type, provider_id, model, request_model, pricing_model, request_count, success_count, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, total_cost_usd, avg_latency_ms) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`).run(
      r.date, r.app_type, r.provider_id, r.model, r.request_model, r.pricing_model, r.request_count, r.success_count, r.input_tokens, r.output_tokens, r.cache_read_tokens, r.cache_creation_tokens, String(r.total_cost), 0,
    );
  }
  const del = db.prepare(`DELETE FROM proxy_request_logs WHERE created_at < ?`).run(cutoff) as unknown as { changes: number };
  const changes = (del as { changes?: number })?.changes ?? 0;
  if (rows.length > 0 && changes === 0) {
    return { rolled: rows.length };
  }
  return { rolled: rows.length };
}
