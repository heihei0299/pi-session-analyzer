/**
 * DB 聚合与剪枝（直切）
 */
import type { Database } from "./db.ts";
import { emptyTotals, finalizeTotals, type Totals, type GroupRow } from "./aggregate.ts";

export function queryTotals(db: Database, filter: { since?: string; until?: string; model?: string; cwd?: string }): Totals {
  let sql = `SELECT COUNT(*) as requests, COALESCE(SUM(input_tokens),0) as input, COALESCE(SUM(output_tokens),0) as output, COALESCE(SUM(cache_read_tokens),0) as cacheRead, COALESCE(SUM(cache_creation_tokens),0) as cacheWrite, COALESCE(SUM(CAST(total_cost_usd AS REAL)),0) as cost FROM proxy_request_logs WHERE app_type='pi' AND data_source='pi_session'`;
  const params: unknown[] = [];
  if (filter.model) {
    sql += ` AND model = ?`;
    params.push(filter.model);
  }
  // since/until 暂略（按 created_at）
  const row = db.prepare(sql).get(...(params as string[])) as { requests: number; input: number; output: number; cacheRead: number; cacheWrite: number; cost: number };
  const totals = emptyTotals();
  totals.requests = row.requests;
  totals.input = row.input;
  totals.output = row.output;
  totals.cacheRead = row.cacheRead;
  totals.cacheWrite = row.cacheWrite;
  totals.cost = row.cost;
  // reasoning 暂 0，totalTokens 由 finalize 计算
  finalizeTotals(totals);
  return totals;
}

export function queryGroups(db: Database, by: string, filter: Record<string, unknown>): GroupRow[] {
  // by=model|cwd|model,cwd 简化：仅实现 model
  if (by === "model") {
    const rows = db.prepare(`SELECT model, COUNT(*) as requests, COALESCE(SUM(input_tokens),0) as input, COALESCE(SUM(output_tokens),0) as output, COALESCE(SUM(cache_read_tokens),0) as cacheRead, COALESCE(SUM(cache_creation_tokens),0) as cacheWrite, COALESCE(SUM(CAST(total_cost_usd AS REAL)),0) as cost FROM proxy_request_logs WHERE app_type='pi' AND data_source='pi_session' GROUP BY model`).all() as { model: string; requests: number; input: number; output: number; cacheRead: number; cacheWrite: number; cost: number }[];
    return rows.map((r) => {
      const t = emptyTotals();
      t.requests = r.requests; t.input = r.input; t.output = r.output; t.cacheRead = r.cacheRead; t.cacheWrite = r.cacheWrite; t.cost = r.cost;
      finalizeTotals(t);
      return { model: r.model, ...t };
    });
  }
  // 其他 by 暂返回空
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
  // better-sqlite3 的 run 返回 { changes }
  const changes = (del as { changes?: number })?.changes ?? 0;
  // 兼容 DatabaseSync 的 run 返回：检查 changes
  // 若 changes 未取到，改用 SELECT COUNT
  if (rows.length > 0 && changes === 0) {
    // fallback: 已聚合但未删除（可能 run 返回不同）
    return { rolled: rows.length };
  }
  return { rolled: rows.length };
}
