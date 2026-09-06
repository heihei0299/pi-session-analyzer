/**
 * 指纹增量同步（直切 cc-switch）— 修正版
 */
import { statSync, readFileSync, openSync, readSync, closeSync } from "node:fs";
import { basename } from "node:path";
import { createHash } from "node:crypto";
import { parsePiUsageRecord } from "./pi-parse.ts";
import { piRequestIdentity } from "./pi-identity.ts";
import { costForRecord } from "./cost/calculator.ts";
import { defaultSessionData } from "./session-data.ts";
import type { Database } from "./db.ts";

export interface PiFileRevision {
  fileSize: number;
  tailFingerprint: number;
  complete: boolean;
  modifiedMs: number;
}

const REVISION_MARKER = 0b101;
const REVISION_MARKER_SHIFT = 61n;
const REVISION_COMPLETE_SHIFT = 60n;
const REVISION_SIZE_SHIFT = 32n;
const REVISION_SIZE_MASK = (1 << 28) - 1;

function encodeRevision(rev: PiFileRevision): bigint {
  return (BigInt(REVISION_MARKER) << REVISION_MARKER_SHIFT) |
    (BigInt(rev.complete ? 1 : 0) << REVISION_COMPLETE_SHIFT) |
    (BigInt(rev.fileSize) << REVISION_SIZE_SHIFT) |
    BigInt(rev.tailFingerprint >>> 0);
}

function tailFingerprint(buf: Buffer): number {
  const h = createHash("sha256");
  const label = Buffer.from("pi-session-tail-v1");
  const len = Buffer.allocUnsafe(8);
  len.writeBigUInt64BE(BigInt(label.length));
  h.update(len);
  h.update(label);
  const len2 = Buffer.allocUnsafe(8);
  len2.writeBigUInt64BE(BigInt(buf.length));
  h.update(len2);
  h.update(buf);
  const digest = h.digest();
  return digest.readUInt32BE(0);
}

export function piFileRevision(filePath: string): PiFileRevision {
  const st = statSync(filePath);
  const fileSize = Number(st.size);
  const modifiedMs = st.mtimeMs;
  const tailLen = Math.min(fileSize, 4096);
  let tail = Buffer.alloc(0);
  let complete = false;
  if (tailLen > 0) {
    const fd = openSync(filePath, "r");
    try {
      const buf = Buffer.alloc(tailLen);
      readSync(fd, buf, 0, tailLen, fileSize - tailLen);
      tail = buf;
      complete = buf[buf.length - 1] === 0x0a;
    } finally {
      closeSync(fd);
    }
  } else {
    complete = true;
  }
  return {
    fileSize,
    tailFingerprint: tailFingerprint(tail),
    complete,
    modifiedMs,
  };
}

/** 计算文件在 offset（已提交字节数）处的 tail 指纹（末 4096B 截止于 offset） */
function tailFingerprintAt(filePath: string, offset: number): number {
  const len = Math.min(offset, 4096);
  if (len <= 0) return tailFingerprint(Buffer.alloc(0));
  const fd = openSync(filePath, "r");
  try {
    const buf = Buffer.alloc(len);
    readSync(fd, buf, 0, len, offset - len);
    return tailFingerprint(buf);
  } finally {
    closeSync(fd);
  }
}

/** 读取文件首行（header） */
function readHeaderLine(filePath: string): string | null {
  const fd = openSync(filePath, "r");
  try {
    const buf = Buffer.alloc(4096);
    const n = readSync(fd, buf, 0, 4096, 0);
    if (n <= 0) return null;
    const str = buf.subarray(0, n).toString("utf8");
    const idx = str.indexOf("\n");
    if (idx !== -1) return str.slice(0, idx);
    // 首行超长（极少），回退全量读取
    const content = readFileSync(filePath, "utf8");
    return content.split("\n")[0] ?? null;
  } finally {
    closeSync(fd);
  }
}

/** 读取文件从 offset 起的后缀内容 */
function readSuffix(filePath: string, offset: number): Buffer {
  const st = statSync(filePath);
  const size = Number(st.size);
  if (offset >= size) return Buffer.alloc(0);
  const len = size - offset;
  const fd = openSync(filePath, "r");
  try {
    const buf = Buffer.alloc(len);
    readSync(fd, buf, 0, len, offset);
    return buf;
  } finally {
    closeSync(fd);
  }
}

export interface SyncResult {
  imported: number;
  skipped: number;
}

function truncateLabel(s: string): string {
  const buf = Buffer.from(s);
  if (buf.length <= 512) return s;
  let end = 512;
  while (end > 0 && (buf[end] & 0xc0) === 0x80) end--;
  return buf.subarray(0, end).toString();
}

/** 从 message.entries 提取首条 user 文本（与 analyzeFile 同逻辑，供 displayName） */
function extractFirstUserText(lines: string[]): string | undefined {
  for (let i = 1; i < lines.length; i++) {
    const line = lines[i].trim();
    if (!line) continue;
    let entry: Record<string, unknown>;
    try {
      entry = JSON.parse(line);
    } catch {
      continue;
    }
    if (entry.type !== "message") continue;
    const msg = entry.message as Record<string, unknown> | undefined;
    if (!msg || msg.role !== "user") continue;
    const content = msg.content;
    if (!Array.isArray(content)) continue;
    for (const part of content) {
      if (part !== null && typeof part === "object" && !Array.isArray(part) && (part as Record<string, unknown>).type === "text") {
        const text = String((part as Record<string, unknown>).text ?? "");
        if (text.trim()) return text.trim();
      }
    }
  }
  return undefined;
}

export async function syncPiUsage(db: Database, files: string[], opts?: { full?: boolean }): Promise<SyncResult> {
  let imported = 0;
  let skipped = 0;
  const full = opts?.full ?? false;
  // model_pricing 一次加载
  const pricingByModel = new Map<string, { inputCostPerMillion: string; outputCostPerMillion: string; cacheReadCostPerMillion: string; cacheCreationCostPerMillion: string }>();
  try {
    const pricingRows = db.prepare(`SELECT model_id, input_cost_per_million, output_cost_per_million, cache_read_cost_per_million, cache_creation_cost_per_million FROM model_pricing`).all() as Record<string, string>[];
    for (const r of pricingRows) {
      pricingByModel.set(r.model_id, { inputCostPerMillion: r.input_cost_per_million, outputCostPerMillion: r.output_cost_per_million, cacheReadCostPerMillion: r.cache_read_cost_per_million, cacheCreationCostPerMillion: r.cache_creation_cost_per_million });
    }
  } catch {}
  for (const file of files) {
    const rev = piFileRevision(file);
    let cursor: { last_modified: number; last_line_offset: number; last_synced_at: number; last_byte_offset: number | null; last_tail_fingerprint: number | null } | undefined;
    let canSeek = false;
    if (!full) {
      const row = db.prepare(`SELECT last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint FROM session_log_sync WHERE file_path = ?`).get(file) as
        | { last_modified: number; last_line_offset: number; last_synced_at: number; last_byte_offset: number | null; last_tail_fingerprint: number | null }
        | undefined;
      if (row && row.last_byte_offset !== null && row.last_tail_fingerprint !== null) {
        cursor = row;
        if (rev.fileSize >= row.last_byte_offset) {
          const actualFp = tailFingerprintAt(file, row.last_byte_offset);
          if (actualFp === row.last_tail_fingerprint) canSeek = true;
          else canSeek = false;
        } else {
          // 截断
          canSeek = false;
        }
      }
    }
    // 快跳：已seek且无新数据
    if (canSeek && cursor && rev.fileSize === cursor.last_byte_offset && rev.tailFingerprint === cursor.last_tail_fingerprint) {
      continue;
    }
    let headerLine: string | null = null;
    let linesToProcess: string[] = [];
    let header: Record<string, unknown> | null = null;
    let sessionId = "";
    let headerTs = "";
    let headerCwd = "";
    let parentSessionId: string | undefined;
    let sessionTimestamp: number | null = null;
    let forkTs: number | null = null;
    let newCommittedByte: number;
    let newCommittedLines: number;
    let newTailFp: number;
    if (canSeek && cursor) {
      headerLine = readHeaderLine(file);
      if (!headerLine) continue;
      try {
        header = JSON.parse(headerLine);
        if (!header || (header.type as string) !== "session") continue;
      } catch { continue; }
      // header 元信息
      sessionId = (header.id as string) ?? "";
      headerTs = typeof header.timestamp === "string" ? header.timestamp : "";
      headerCwd = typeof header.cwd === "string" ? header.cwd : "";
      const ts = header.timestamp as string | undefined;
      if (ts) {
        const t = Date.parse(ts);
        if (!Number.isNaN(t)) sessionTimestamp = Math.floor(t / 1000);
      }
      const parentSession = header.parentSession as string | undefined;
      if (typeof parentSession === "string" && parentSession.length > 0) parentSessionId = parentSession;
      if (parentSession && (parentSession.includes("/") || parentSession.includes("\\"))) {
        if (ts) {
          const t = Date.parse(ts);
          if (!Number.isNaN(t)) forkTs = t;
        }
      }
      if (!header || (header.type as string) !== "session") continue;
      const suffixBuf = readSuffix(file, cursor!.last_byte_offset!);
      if (suffixBuf.length === 0) {
        // 无新增，继续（已在快跳处理）
        continue;
      }
      const suffixStr = suffixBuf.toString("utf8");
      // 计算新增的完整行（半行不计入）
      const parts = suffixStr.split("\n");
      const completeNewLines = parts.length - 1;
      // 本次可提交的完整行（排除末尾半行）
      if (completeNewLines <= 0) {
        // 只有半行，无完整新增，暂不推进游标（下轮重验）
        // 但仍需处理？ 当前 suffix 仅半行，linesToProcess 为空，跳过插入但不更新游标
        continue;
      }
      linesToProcess = parts.slice(0, -1);
      // 新提交位点
      if (rev.complete) {
        newCommittedByte = rev.fileSize;
      } else {
        const lastNL = suffixStr.lastIndexOf("\n");
        newCommittedByte = cursor!.last_byte_offset! + lastNL + 1;
      }
      newCommittedLines = cursor!.last_line_offset + completeNewLines;
      newTailFp = tailFingerprintAt(file, newCommittedByte);
    } else {
      // 全量
      const content = readFileSync(file, "utf8");
      const parts = content.split("\n");
      headerLine = parts[0] ?? null;
      if (!headerLine) continue;
      try {
        header = JSON.parse(headerLine);
        if (!header || (header.type as string) !== "session") continue;
      } catch { continue; }
      sessionId = (header.id as string) ?? "";
      headerTs = typeof header.timestamp === "string" ? header.timestamp : "";
      headerCwd = typeof header.cwd === "string" ? header.cwd : "";
      const ts = header.timestamp as string | undefined;
      if (ts) {
        const t = Date.parse(ts);
        if (!Number.isNaN(t)) sessionTimestamp = Math.floor(t / 1000);
      }
      const parentSession = header.parentSession as string | undefined;
      if (typeof parentSession === "string" && parentSession.length > 0) parentSessionId = parentSession;
      if (parentSession && (parentSession.includes("/") || parentSession.includes("\\"))) {
        if (ts) {
          const t = Date.parse(ts);
          if (!Number.isNaN(t)) forkTs = t;
        }
      }
      if (!header || (header.type as string) !== "session") continue;
      // 完整行处理：排除末尾半行或空
      if (parts.length <= 1) {
        linesToProcess = [];
      } else {
        linesToProcess = parts.slice(1, -1);
        // parts.slice(1, -1) 对完整（末空）和半行（末 partial）均排除末元素，符合“半行不推进”
      }
      if (rev.complete) {
        newCommittedByte = rev.fileSize;
        newCommittedLines = parts.length - 1;
      } else {
        const lastNL = content.lastIndexOf("\n");
        newCommittedByte = lastNL !== -1 ? lastNL + 1 : 0;
        newCommittedLines = parts.length - 1;
      }
      newTailFp = tailFingerprintAt(file, newCommittedByte);
    }
    if (!header) continue;
    const seenInFile = new Map<string, NonNullable<ReturnType<typeof parsePiUsageRecord>>>();
    const identities = new Map<string, ReturnType<typeof piRequestIdentity>>();
    const tsTextById = new Map<string, string>();
    for (const rawLine of linesToProcess) {
      const line = rawLine.trim();
      if (!line) continue;
      let entry: Record<string, unknown>;
      try { entry = JSON.parse(line); } catch { continue; }
      if (forkTs !== null) {
        const tsStr = entry.timestamp as string | undefined;
        if (tsStr) {
          const t = Date.parse(tsStr);
          if (!Number.isNaN(t) && t < forkTs) continue;
        }
      }
      const rec = parsePiUsageRecord(entry, sessionId, sessionTimestamp, rev.modifiedMs);
      if (!rec) continue;
      const usageRaw = (() => {
        if (entry.type === "message") {
          const msg = entry.message as Record<string, unknown>;
          return (msg.usage as Record<string, unknown>) ?? {};
        }
        return (entry.usage as Record<string, unknown>) ?? {};
      })();
      const identity = piRequestIdentity(entry, rec.kind, usageRaw as Record<string, unknown>, (entry.message as Record<string, unknown>) ?? null);
      const existing = seenInFile.get(identity.requestId);
      if (existing) {
        const existingStop = (existing as unknown as { stopReason?: string }).stopReason;
        const newStop = (rec as unknown as { stopReason?: string }).stopReason;
        const shouldReplace = (newStop && !existingStop) || (Boolean(newStop) === Boolean(existingStop) && rec.output > existing.output);
        if (!shouldReplace) continue;
      }
      seenInFile.set(identity.requestId, rec);
      identities.set(identity.requestId, identity);
      tsTextById.set(identity.requestId, typeof entry.timestamp === "string" ? entry.timestamp : "");
      (rec as unknown as Record<string, unknown>).__identity = identity;
    }
    // 原子事务
    try { db.exec("BEGIN"); } catch {}
    try {
      for (const [requestId, rec] of seenInFile) {
        const identity = identities.get(requestId)!;
        const exists = db.prepare(`SELECT 1 FROM session_usage_dedup WHERE data_source = ? AND request_id = ?`).get("pi_session", requestId) as unknown;
        if (exists) { skipped++; continue; }
        if (!identity.hasEntryId) {
          const semExists = db.prepare(`SELECT 1 FROM session_usage_dedup WHERE data_source = ? AND semantic_id = ?`).get("pi_session", identity.semanticId) as unknown;
          if (semExists) { skipped++; continue; }
        }
        db.prepare(`INSERT OR IGNORE INTO session_usage_dedup (data_source, request_id, semantic_id, has_entry_id) VALUES (?, ?, ?, ?)`).run("pi_session", requestId, identity.semanticId, identity.hasEntryId ? 1 : 0);
        const pricing = pricingByModel.get(rec.model) ?? null;
        const cost = costForRecord({ input: rec.input, output: rec.output, cacheRead: rec.cacheRead, cacheWrite: rec.cacheWrite, cost: { total: rec.costTotal } }, pricing);
        const tsText = tsTextById.get(requestId) ?? "";
        db.prepare(`INSERT OR IGNORE INTO proxy_request_logs (request_id, provider_id, app_type, model, request_model, pricing_model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, input_token_semantics, total_cost_usd, latency_ms, status_code, error_message, session_id, provider_type, is_streaming, cost_multiplier, created_at, data_source, kind, reasoning_tokens, cwd, timestamp_text) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`).run(requestId, rec.provider, "pi", rec.model, rec.requestModel, rec.model, rec.input, rec.output, rec.cacheRead, rec.cacheWrite, 0, String(cost), 0, rec.statusCode, rec.errorMessage ?? null, rec.sessionId, "pi_session", 1, "1.0", rec.createdAt, "pi_session", rec.kind, rec.reasoning, headerCwd, tsText);
        imported++;
      }
      // 会话元数据
      {
        const fileName = basename(file);
        // 提取首条 user 文本需全量内容；seek 路径可复用 header 已读，但为简化全量读首段
        let firstUserText: string | undefined;
        try {
          const allContent = readFileSync(file, "utf8");
          const allLines = allContent.split("\n");
          firstUserText = extractFirstUserText(allLines);
        } catch {}
        const displayName = defaultSessionData.displayNameOf(fileName, firstUserText);
        const isTask = file.includes("/tasks/") || file.includes("\\tasks\\") ? 1 : 0;
        db.prepare(`INSERT OR REPLACE INTO pi_sessions (session_id, header_ts, cwd, file_name, display_name, is_task, parent_session_id) VALUES (?, ?, ?, ?, ?, ?, ?)`).run(sessionId, headerTs, headerCwd, fileName, displayName, isTask, parentSessionId ?? null);
      }
      const encoded = encodeRevision(rev);
      try {
        db.prepare(`INSERT OR REPLACE INTO session_log_sync (file_path, last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint) VALUES (?, ?, ?, ?, ?, ?)`).run(file, rev.modifiedMs, newCommittedLines, encoded as unknown as number, newCommittedByte, newTailFp);
      } catch {
        db.prepare(`INSERT OR REPLACE INTO session_log_sync (file_path, last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint) VALUES (?, ?, ?, ?, ?, ?)`).run(file, rev.modifiedMs, newCommittedLines, Number(encoded), newCommittedByte, newTailFp);
      }
      try { db.exec("COMMIT"); } catch {}
    } catch (e) {
      try { db.exec("ROLLBACK"); } catch {}
      throw e;
    }
  }
  return { imported, skipped };
}
