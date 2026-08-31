/**
 * OpenCode 云端数据类型契约 — 按 spec §Protocol & Data Contract Types
 */

export interface OpenCodeUsageRecord {
  id: string; // "usg_..."
  workspaceID: string; // "wrk_..."
  timeCreated: string; // ISO string
  timeUpdated: string;
  timeDeleted: string | null;
  model: string; // e.g. "x-preview-f-free", "deepseek-v4-flash"
  provider: string; // "inf.oa-compat"
  inputTokens: number;
  outputTokens: number;
  reasoningTokens: number | null;
  cacheReadTokens: number | null;
  cacheWrite5mTokens: number | null;
  cacheWrite1hTokens: number | null;
  cost: number; // micro-unit / floating dollars
  keyID: string;
  sessionID: string | null;
  enrichment: unknown | null;
}

export interface OpenCodeMonthlyCostItem {
  date: string | null;
  model: string;
  totalCost: number; // scaled integer (e.g. 891912946 -> $8.9191) factor 1e-8
  keyId: string;
  plan: string; // "lite" | "standard"
}

export interface OpenCodeCostsResult {
  usage: OpenCodeMonthlyCostItem[];
  keys: Array<{ id: string; name: string }>;
}

export interface WorkspaceInfo {
  id: string;
  name: string;
  slug?: string;
  // 兼容后端可能字段
  displayName?: string;
  workspaceID?: string;
}

// 别名保持 spec 与 issue 兼容
export type MonthlyCostsResult = OpenCodeCostsResult;
export type UsageRecord = OpenCodeUsageRecord;
