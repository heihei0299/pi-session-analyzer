/**
 * OpenCode 本地数据仓 — data/opencode/ 分层持久化与增量同步
 * ponytail: 文件级同步无锁，单进程串行写已足够；如需并发需加文件锁
 */
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { OpenCodeCostsResult, OpenCodeUsageRecord } from "./types.ts";

export interface SyncOptions {
  workspaceId?: string;
  full?: boolean;
  limit?: number;
}

export interface SyncResult {
  added: number;
  pages: number;
  elapsedMs: number;
  lastSyncedTime: string | null;
}

export interface HistoryFilter {
  model?: string;
  sessionId?: string;
  sessionID?: string; // alias
  since?: string;
  until?: string;
  // also support time range via desde/hasta strings
}

interface HistoryFile {
  records: OpenCodeUsageRecord[];
  lastSyncedTime: string | null;
  updatedAt: string;
}

type CostsFile = Record<string, OpenCodeCostsResult>;

const CSV_FIELDS: (keyof OpenCodeUsageRecord)[] = [
  "id",
  "workspaceID",
  "timeCreated",
  "timeUpdated",
  "timeDeleted",
  "model",
  "provider",
  "inputTokens",
  "outputTokens",
  "reasoningTokens",
  "cacheReadTokens",
  "cacheWrite5mTokens",
  "cacheWrite1hTokens",
  "cost",
  "keyID",
  "sessionID",
  "enrichment",
];

function csvEscape(v: unknown): string {
  if (v === null || v === undefined) return "";
  let s: string;
  if (typeof v === "string") s = v;
  else if (typeof v === "number" || typeof v === "boolean") s = String(v);
  else s = JSON.stringify(v);
  if (s.includes('"') || s.includes(",") || s.includes("\n") || s.includes("\r")) {
    return `"${s.replace(/"/g, '""')}"`;
  }
  return s;
}

function keyFor(year: number, month: number): string {
  return `${year}-${String(month).padStart(2, "0")}`;
}

function sortDesc(a: OpenCodeUsageRecord, b: OpenCodeUsageRecord): number {
  const ta = Date.parse(a.timeCreated);
  const tb = Date.parse(b.timeCreated);
  if (Number.isNaN(ta) && Number.isNaN(tb)) return 0;
  if (Number.isNaN(ta)) return 1;
  if (Number.isNaN(tb)) return -1;
  return tb - ta; // 逆序
}

export class OpenCodeStorage {
  readonly dataDir: string;

  constructor(dataDir: string = "data/opencode") {
    this.dataDir = dataDir;
  }

  private costsPath(): string {
    return join(this.dataDir, "costs.json");
  }
  private historyPath(): string {
    return join(this.dataDir, "history.json");
  }
  private csvPath(): string {
    return join(this.dataDir, "history.csv");
  }

  // T1
  async ensureDataDir(): Promise<void> {
    await mkdir(this.dataDir, { recursive: true });
    // costs.json
    try {
      await readFile(this.costsPath(), "utf-8");
    } catch {
      await writeFile(this.costsPath(), JSON.stringify({}, null, 2), "utf-8");
    }
    // history.json
    try {
      await readFile(this.historyPath(), "utf-8");
    } catch {
      const init: HistoryFile = { records: [], lastSyncedTime: null, updatedAt: new Date().toISOString() };
      await writeFile(this.historyPath(), JSON.stringify(init, null, 2), "utf-8");
    }
  }

  private async readCostsFile(): Promise<CostsFile> {
    try {
      const txt = await readFile(this.costsPath(), "utf-8");
      const parsed = JSON.parse(txt) as unknown;
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
        // support both { "2026-08": result } and { entries: {...} }
        const obj = parsed as Record<string, unknown>;
        if ("entries" in obj && obj.entries && typeof obj.entries === "object" && !Array.isArray(obj.entries)) {
          return obj.entries as CostsFile;
        }
        return parsed as CostsFile;
      }
      return {};
    } catch {
      return {};
    }
  }

  private async writeCostsFile(data: CostsFile): Promise<void> {
    await mkdir(this.dataDir, { recursive: true });
    await writeFile(this.costsPath(), JSON.stringify(data, null, 2), "utf-8");
  }

  private async readHistoryFile(): Promise<HistoryFile> {
    try {
      const txt = await readFile(this.historyPath(), "utf-8");
      const parsed = JSON.parse(txt) as unknown;
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
        const obj = parsed as Record<string, unknown>;
        const records = Array.isArray(obj.records) ? (obj.records as OpenCodeUsageRecord[]) : [];
        const lastSyncedTime = typeof obj.lastSyncedTime === "string" ? (obj.lastSyncedTime as string) : null;
        const updatedAt = typeof obj.updatedAt === "string" ? (obj.updatedAt as string) : new Date().toISOString();
        return { records, lastSyncedTime, updatedAt };
      }
      return { records: [], lastSyncedTime: null, updatedAt: new Date().toISOString() };
    } catch {
      return { records: [], lastSyncedTime: null, updatedAt: new Date().toISOString() };
    }
  }

  private async writeHistoryFile(data: HistoryFile): Promise<void> {
    await mkdir(this.dataDir, { recursive: true });
    await writeFile(this.historyPath(), JSON.stringify(data, null, 2), "utf-8");
  }

  // T2
  async saveCosts(year: number, month: number, result: OpenCodeCostsResult): Promise<void> {
    await this.ensureDataDir();
    const costs = await this.readCostsFile();
    costs[keyFor(year, month)] = result;
    await this.writeCostsFile(costs);
  }

  async getCosts(year: number, month: number): Promise<OpenCodeCostsResult | null> {
    const costs = await this.readCostsFile();
    const k = keyFor(year, month);
    return costs[k] ?? null;
  }

  async listCosts(): Promise<Array<{ year: number; month: number; result: OpenCodeCostsResult }>> {
    const costs = await this.readCostsFile();
    const out: Array<{ year: number; month: number; result: OpenCodeCostsResult }> = [];
    for (const [k, v] of Object.entries(costs)) {
      const m = k.match(/^(\d{4})-(\d{1,2})$/);
      if (!m) continue;
      out.push({ year: Number(m[1]), month: Number(m[2]), result: v });
    }
    out.sort((a, b) => a.year !== b.year ? a.year - b.year : a.month - b.month);
    return out;
  }

  // 支持查询全部 costs map（供 04 使用）
  async getAllCosts(): Promise<CostsFile> {
    return this.readCostsFile();
  }

  // T3
  async mergeHistory(records: OpenCodeUsageRecord[]): Promise<{ added: number; total: number }> {
    await this.ensureDataDir();
    const file = await this.readHistoryFile();
    const existing = file.records;
    const existingMap = new Map<string, OpenCodeUsageRecord>();
    for (const r of existing) existingMap.set(r.id, r);

    let added = 0;
    // 去重：保留最新（后到的覆盖）
    for (const r of records) {
      if (!existingMap.has(r.id)) added++;
      existingMap.set(r.id, r);
    }

    const merged = Array.from(existingMap.values());
    merged.sort(sortDesc);

    const lastSyncedTime = merged.length > 0 ? merged[0].timeCreated : file.lastSyncedTime;
    // 若 merged 为空保留原游标（可能 null）
    const next: HistoryFile = {
      records: merged,
      lastSyncedTime,
      updatedAt: new Date().toISOString(),
    };
    await this.writeHistoryFile(next);
    // 自动生成 CSV
    await this.exportCsv();
    return { added, total: merged.length };
  }

  // T4
  async exportCsv(filePath?: string): Promise<string> {
    const file = await this.readHistoryFile();
    const records = file.records;
    const header = CSV_FIELDS.join(",");
    const lines: string[] = [header];
    for (const r of records) {
      const row = CSV_FIELDS.map((f) => csvEscape((r as unknown as Record<string, unknown>)[f as string])).join(",");
      lines.push(row);
    }
    const csv = lines.join("\n") + "\n";
    const target = filePath ?? this.csvPath();
    await mkdir(this.dataDir, { recursive: true });
    await writeFile(target, csv, "utf-8");
    return csv;
  }

  // T6
  async loadHistory(): Promise<HistoryFile> {
    await this.ensureDataDir();
    return this.readHistoryFile();
  }

  async getHistory(filter?: HistoryFilter): Promise<OpenCodeUsageRecord[]> {
    const file = await this.readHistoryFile();
    let records = file.records;
    if (!filter) return records;
    const model = filter.model;
    const sessionId = filter.sessionId ?? filter.sessionID;
    const since = filter.since;
    const until = filter.until;

    if (model) records = records.filter((r) => r.model === model);
    if (sessionId) records = records.filter((r) => r.sessionID === sessionId);
    if (since) {
      const s = Date.parse(since);
      if (!Number.isNaN(s)) records = records.filter((r) => Date.parse(r.timeCreated) >= s);
    }
    if (until) {
      const u = Date.parse(until);
      if (!Number.isNaN(u)) records = records.filter((r) => Date.parse(r.timeCreated) <= u);
    }
    return records;
  }

  async clear(): Promise<void> {
    await this.ensureDataDir();
    const empty: HistoryFile = { records: [], lastSyncedTime: null, updatedAt: new Date().toISOString() };
    await this.writeHistoryFile(empty);
    await this.writeCostsFile({});
    // csv 也清空为仅表头
    await this.exportCsv();
  }

  async reset(): Promise<void> {
    return this.clear();
  }

  // T5 sync
  async sync(client: { getUsageInfo: (workspaceId: string, page: number) => Promise<OpenCodeUsageRecord[]> }, opts: SyncOptions): Promise<SyncResult>;
  async sync(workspaceId: string, client: { getUsageInfo: (workspaceId: string, page: number) => Promise<OpenCodeUsageRecord[]> }, opts?: SyncOptions): Promise<SyncResult>;
  async sync(client: { getUsageInfo: (workspaceId: string, page: number) => Promise<OpenCodeUsageRecord[]> }, workspaceId: string, opts?: SyncOptions): Promise<SyncResult>;
  async sync(
    clientOrWorkspaceId: string | { getUsageInfo: (workspaceId: string, page: number) => Promise<OpenCodeUsageRecord[]> },
    workspaceIdOrOpts?: string | SyncOptions | { getUsageInfo: (workspaceId: string, page: number) => Promise<OpenCodeUsageRecord[]> },
    optsMaybe?: SyncOptions,
  ): Promise<SyncResult> {
    let client: { getUsageInfo: (workspaceId: string, page: number) => Promise<OpenCodeUsageRecord[]> };
    let workspaceId: string | undefined;
    let opts: SyncOptions | undefined;

    // overload 1: sync(workspaceId, client, opts)
    if (typeof clientOrWorkspaceId === "string") {
      workspaceId = clientOrWorkspaceId;
      client = workspaceIdOrOpts as unknown as typeof client;
      opts = optsMaybe;
    } else {
      client = clientOrWorkspaceId;
      if (typeof workspaceIdOrOpts === "string") {
        workspaceId = workspaceIdOrOpts;
        opts = optsMaybe;
      } else {
        opts = workspaceIdOrOpts as SyncOptions | undefined;
        workspaceId = opts?.workspaceId;
      }
    }

    if (!workspaceId) throw new Error("sync 需要 workspaceId（传 workspaceId 参数或 opts.workspaceId）");
    if (!client || typeof client.getUsageInfo !== "function") throw new Error("sync 需要提供含 getUsageInfo 的 client");

    const full = opts?.full ?? false;
    const limit = opts?.limit;

    await this.ensureDataDir();
    const file = await this.readHistoryFile();
    const existingIds = new Set(file.records.map((r) => r.id));
    const lastSyncedTime = file.lastSyncedTime;
    const lastTs = lastSyncedTime ? Date.parse(lastSyncedTime) : null;

    const collected: OpenCodeUsageRecord[] = [];
    let pages = 0;
    const start = Date.now();
    // eslint-disable-next-line no-constant-condition
    for (let page = 0; true; page++) {
      if (limit !== undefined && pages >= limit) break;
      const batch = await client.getUsageInfo(workspaceId, page);
      pages++;
      if (!batch || batch.length === 0) break;

      if (full) {
        collected.push(...batch);
        continue;
      }

      // 检查截断点
      let cutoffIdx = -1;
      for (let i = 0; i < batch.length; i++) {
        const r = batch[i];
        if (existingIds.has(r.id)) {
          cutoffIdx = i;
          break;
        }
        if (lastTs !== null) {
          const t = Date.parse(r.timeCreated);
          if (!Number.isNaN(t) && t <= lastTs) {
            cutoffIdx = i;
            break;
          }
        }
      }
      if (cutoffIdx !== -1) {
        // 只收集截断点之前的
        collected.push(...batch.slice(0, cutoffIdx));
        break;
      } else {
        collected.push(...batch);
      }
      // 若本页已出现截断或空，已 break；否则继续下一页
      // 为了避免无限循环，若未截断但 batch 为空已在顶部 break
    }

    let added = 0;
    if (collected.length > 0) {
      const res = await this.mergeHistory(collected);
      added = res.added;
    } else {
      // 即使无新增，也需刷新 updatedAt？保持 lastSyncedTime 不变，但若全空且原无记录则仍 null
      // 不额外写文件以免无意义抖动，但 sync 返回需读最新游标
    }

    const after = await this.readHistoryFile();
    const elapsedMs = Date.now() - start;
    return { added, pages, elapsedMs, lastSyncedTime: after.lastSyncedTime };
  }
}
