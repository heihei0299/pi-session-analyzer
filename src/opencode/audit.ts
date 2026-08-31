/**
 * OpenCode 对账引擎 — 本地 SessionData 聚合 vs OpenCode 官方明细求和
 * ponytail: 仅做月度 token/cost 求和对比，localCost 直接用 usage.cost.total 聚合；差异解释为固定文案
 */
import type { Totals } from "../aggregate.ts";
import type { OpenCodeUsageRecord } from "./types.ts";

export interface OpencodeTotals {
  requests: number;
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  reasoning: number;
  totalTokens: number;
  cost: number;
}

export interface AuditDiff {
  requests: number;
  tokens: number;
  cost: number;
}

export interface AuditDiffRate {
  requests: number;
  tokens: number;
  cost: number;
}

export interface AuditResult {
  opencodeTotals: OpencodeTotals;
  diff: AuditDiff;
  diffRate: AuditDiffRate;
  comparison: string;
}

/** 计算 OpenCode 记录的总和（按 ADR-0002 totalTokens = input+cacheRead+output） */
export function computeOpencodeTotals(records: OpenCodeUsageRecord[]): OpencodeTotals {
  let input = 0;
  let output = 0;
  let cacheRead = 0;
  let cacheWrite = 0;
  let reasoning = 0;
  let cost = 0;
  for (const r of records) {
    input += typeof r.inputTokens === "number" && Number.isFinite(r.inputTokens) ? r.inputTokens : 0;
    output += typeof r.outputTokens === "number" && Number.isFinite(r.outputTokens) ? r.outputTokens : 0;
    cacheRead += typeof r.cacheReadTokens === "number" && Number.isFinite(r.cacheReadTokens) ? r.cacheReadTokens : 0;
    const cw5 = typeof r.cacheWrite5mTokens === "number" && Number.isFinite(r.cacheWrite5mTokens) ? r.cacheWrite5mTokens : 0;
    const cw1 = typeof r.cacheWrite1hTokens === "number" && Number.isFinite(r.cacheWrite1hTokens) ? r.cacheWrite1hTokens : 0;
    cacheWrite += cw5 + cw1;
    reasoning += typeof r.reasoningTokens === "number" && Number.isFinite(r.reasoningTokens) ? r.reasoningTokens : 0;
    cost += typeof r.cost === "number" && Number.isFinite(r.cost) ? r.cost : 0;
  }
  const totalTokens = input + cacheRead + output;
  return { requests: records.length, input, output, cacheRead, cacheWrite, reasoning, totalTokens, cost };
}

/**
 * 对账：对比本地 SessionData Totals 与 OpenCode 明细
 * - diff = opencode - local
 * - diffRate = diff / opencode（opencode 为 0 时记 0，避免除零）
 * - comparison 为结构性差异固定说明
 */
export function buildAudit(localTotals: Totals, opencodeRecords: OpenCodeUsageRecord[]): AuditResult {
  const opencodeTotals = computeOpencodeTotals(opencodeRecords);
  const diff: AuditDiff = {
    requests: opencodeTotals.requests - localTotals.requests,
    tokens: opencodeTotals.totalTokens - localTotals.totalTokens,
    cost: opencodeTotals.cost - localTotals.cost,
  };
  const diffRate: AuditDiffRate = {
    requests: opencodeTotals.requests === 0 ? 0 : diff.requests / opencodeTotals.requests,
    tokens: opencodeTotals.totalTokens === 0 ? 0 : diff.tokens / opencodeTotals.totalTokens,
    cost: opencodeTotals.cost === 0 ? 0 : diff.cost / opencodeTotals.cost,
  };
  const comparison =
    "本地统计仅含 pi 会话中计入的 assistant 消息（MessageTimeRange），未计入内部 compaction/分支同步等非会话请求及非 pi 客户端请求；OpenCode 官方账单含全部扣费请求，差额为预期结构性差异。localCost 为本地 usage.cost.total 求和（若未定价则可能为 0），opencode 成本为官方 cost 求和。";
  return { opencodeTotals, diff, diffRate, comparison };
}
