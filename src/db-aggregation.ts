/**
 * DB 聚合与剪枝（直切）— 修正版：since/until 下推、UNION rollups、by=cwd 提示
 */
import type { Database } from "./db.ts";
import { emptyTotals, finalizeTotals, type Totals, type GroupRow } from "./aggregate.ts";

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

export function queryTotals(db: Database, filter: { since?: string; until?: string; model?: string; cwd?: string }): Totals {
  const { sinceTs, untilTs } = parseSinceUntil(filter.since, filter.until);
  let sql = `SELECT COUNT(*) as requests, COALESCE(SUM(input_tokens),0) as input, COALESCE(SUM(output_tokens),0) as output, COALESCE(SUM(cache_read_tokens),0) as cacheRead, COALESCE(SUM(cache_creation_tokens),0) as cacheWrite, COALESCE(SUM(CAST(total_cost_usd AS REAL)),0) as cost FROM proxy_request_logs WHERE app_type='pi' AND data_source='pi_session'`;
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
  const row = db.prepare(sql).get(...(params as string[])) as { requests: number; input: number; output: number; cacheRead: number; cacheWrite: number; cost: number };
  // UNION rollups：按 date 范围聚合后叠加
  let rollupSql = `SELECT COALESCE(SUM(request_count),0) as requests, COALESCE(SUM(input_tokens),0) as input, COALESCE(SUM(output_tokens),0) as output, COALESCE(SUM(cache_read_tokens),0) as cacheRead, COALESCE(SUM(cache_creation_tokens),0) as cacheWrite, COALESCE(SUM(CAST(total_cost_usd AS REAL)),0) as cost FROM usage_daily_rollups WHERE app_type='pi'`;
  const rollupParams: unknown[] = [];
  if (filter.model) {
    rollupSql += ` AND model = ?`;
    rollupParams.push(filter.model);
  }
  // rollups 按 date 字符串，since/until 转 date
  if (filter.since) {
    rollupSql += ` AND date >= ?`;
    rollupParams.push(filter.since.slice(0, 10));
  }
  if (filter.until) {
    rollupSql += ` AND date <= ?`;
    rollupParams.push(filter.until.slice(0, 10));
  }
  const roll = db.prepare(rollupSql).get(...(rollupParams as string[])) as { requests: number; input: number; output: number; cacheRead: number; cacheWrite: number; cost: number };
  const totals = emptyTotals();
  totals.requests = row.requests + roll.requests;
  totals.input = row.input + roll.input;
  totals.output = row.output + roll.output;
  totals.cacheRead = row.cacheRead + roll.cacheRead;
  totals.cacheWrite = row.cacheWrite + roll.cacheWrite;
  totals.cost = row.cost + roll.cost;
  finalizeTotals(totals);
  return totals;
}

export function queryGroups(db: Database, by: string, filter: Record<string, unknown>): GroupRow[] {
  if (by === "model") {
    const model = filter.model as string | undefined;
    const since = filter.since as string | undefined;
    const until = filter.until as string | undefined;
    const { sinceTs, untilTs } = parseSinceUntil(since, until);
    let sql = `SELECT model, COUNT(*) as requests, COALESCE(SUM(input_tokens),0) as input, COALESCE(SUM(output_tokens),0) as output, COALESCE(SUM(cache_read_tokens),0) as cacheRead, COALESCE(SUM(cache_creation_tokens),0) as cacheWrite, COALESCE(SUM(CAST(total_cost_usd AS REAL)),0) as cost FROM proxy_request_logs WHERE app_type='pi' AND data_source='pi_session'`;
    const params: unknown[] = [];
    if (model) { sql += ` AND model = ?`; params.push(model); }
    if (sinceTs !== undefined) { sql += ` AND created_at >= ?`; params.push(sinceTs); }
    if (untilTs !== undefined) { sql += ` AND created_at <= ?`; params.push(untilTs); }
    sql += ` GROUP BY model`;
    const rows = db.prepare(sql).all(...(params as string[])) as { model: string; requests: number; input: number; output: number; cacheRead: number; cacheWrite: number; cost: number }[];
    // 合并 rollups 的同 model 分组
    let rollupSql = `SELECT model, COALESCE(SUM(request_count),0) as requests, COALESCE(SUM(input_tokens),0) as input, COALESCE(SUM(output_tokens),0) as output, COALESCE(SUM(cache_read_tokens),0) as cacheRead, COALESCE(SUM(cache_creation_tokens),0) as cacheWrite, COALESCE(SUM(CAST(total_cost_usd AS REAL)),0) as cost FROM usage_daily_rollups WHERE app_type='pi'`;
    const rollupParams: unknown[] = [];
    if (model) { rollupSql += ` AND model = ?`; rollupParams.push(model); }
    if (since) { rollupSql += ` AND date >= ?`; rollupParams.push(since.slice(0, 10)); }
    if (until) { rollupSql += ` AND date <= ?`; rollupParams.push(until.slice(0, 10)); }
    rollupSql += ` GROUP BY model`;
    const rollups = db.prepare(rollupSql).all(...(rollupParams as string[])) as { model: string; requests: number; input: number; output: number; cacheRead: number; cacheWrite: number; cost: number }[];
    const map = new Map<string, { requests: number; input: number; output: number; cacheRead: number; cacheWrite: number; cost: number }>();
    for (const r of rows) map.set(r.model, { ...r });
    for (const r of rollups) {
      const cur = map.get(r.model) ?? { requests: 0, input: 0, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0 };
      cur.requests += r.requests; cur.input += r.input; cur.output += r.output; cur.cacheRead += r.cacheRead; cur.cacheWrite += r.cacheWrite; cur.cost += r.cost;
      map.set(r.model, cur);
    }
    const out: GroupRow[] = [];
    for (const [model, r] of map) {
      const t = emptyTotals();
      t.requests = r.requests; t.input = r.input; t.output = r.output; t.cacheRead = r.cacheRead; t.cacheWrite = r.cacheWrite; t.cost = r.cost;
      finalizeTotals(t);
      out.push({ model, ...t });
    }
    return out;
  }
  if (by === "cwd" || by === "model,cwd") {
    // cwd 暂未在 proxy_request_logs 中持久化，返回空并由 API 层提示
    return [];
  }
  return [];
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
