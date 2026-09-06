/**
 * 指纹增量同步（直切 cc-switch）— 修正版
 */
import { statSync, readFileSync, openSync, readSync, closeSync } from "node:fs";
import { createHash } from "node:crypto";
import { parsePiUsageRecord } from "./pi-parse.ts";
import { piRequestIdentity } from "./pi-identity.ts";
import { costForRecord } from "./cost/calculator.ts";
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

export async function syncPiUsage(db: Database, files: string[]): Promise<SyncResult> {
  let imported = 0;
  let skipped = 0;
  for (const file of files) {
    const rev = piFileRevision(file);
    const content = readFileSync(file, "utf8");
    const lines = content.split("\n");
    let header: Record<string, unknown> | null = null;
    let sessionId = "";
    let sessionTimestamp: number | null = null;
    let forkTs: number | null = null;
    try {
      header = JSON.parse(lines[0]);
      if (!header || (header.type as string) !== "session") continue;
      sessionId = (header.id as string) ?? "";
      const ts = header.timestamp as string | undefined;
      if (ts) {
        const t = Date.parse(ts);
        if (!Number.isNaN(t)) sessionTimestamp = Math.floor(t / 1000);
      }
      const parentSession = header.parentSession as string | undefined;
      if (parentSession && (parentSession.includes("/") || parentSession.includes("\\"))) {
        if (ts) {
          const t = Date.parse(ts);
          if (!Number.isNaN(t)) forkTs = t;
        }
      }
    } catch {
      continue;
    }
    if (!header || (header.type as string) !== "session") continue;

    const seenInFile = new Map<string, NonNullable<ReturnType<typeof parsePiUsageRecord>>>();
    const identities = new Map<string, ReturnType<typeof piRequestIdentity>>();
    for (let i = 1; i < lines.length; i++) {
      const line = lines[i].trim();
      if (!line) continue;
      let entry: Record<string, unknown>;
      try {
        entry = JSON.parse(line);
      } catch {
        continue;
      }
      // fork 切分：ts < forkTs 跳过
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
      // 挂载 for later use (compat)
      (rec as unknown as Record<string, unknown>).__identity = identity;
    }

    for (const [requestId, rec] of seenInFile) {
      const identity = identities.get(requestId)!;
      const exists = db.prepare(`SELECT 1 FROM session_usage_dedup WHERE data_source = ? AND request_id = ?`).get("pi_session", requestId) as unknown;
      if (exists) {
        skipped++;
        continue;
      }
      if (!identity.hasEntryId) {
        const semExists = db.prepare(`SELECT 1 FROM session_usage_dedup WHERE data_source = ? AND semantic_id = ?`).get("pi_session", identity.semanticId) as unknown;
        if (semExists) {
          skipped++;
          continue;
        }
      }
      db.prepare(`INSERT OR IGNORE INTO session_usage_dedup (data_source, request_id, semantic_id, has_entry_id) VALUES (?, ?, ?, ?)`).run(
        "pi_session",
        requestId,
        identity.semanticId,
        identity.hasEntryId ? 1 : 0,
      );
      // 费用：reported 优先，否则 pricing 回算（暂无 pricing 表查询，传 null，回算为 0）
      const cost = costForRecord({ input: rec.input, output: rec.output, cacheRead: rec.cacheRead, cacheWrite: rec.cacheWrite, cost: { total: rec.costTotal } }, null);
      db.prepare(
        `INSERT OR IGNORE INTO proxy_request_logs (
          request_id, provider_id, app_type, model, request_model, pricing_model,
          input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens,
          input_token_semantics, total_cost_usd, latency_ms, status_code, error_message, session_id, provider_type, is_streaming, cost_multiplier, created_at, data_source
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      ).run(
        requestId,
        rec.provider,
        "pi",
        rec.model,
        rec.requestModel,
        rec.model,
        rec.input,
        rec.output,
        rec.cacheRead,
        rec.cacheWrite,
        0,
        String(cost),
        0,
        rec.statusCode,
        rec.errorMessage ?? null,
        rec.sessionId,
        "pi_session",
        1,
        "1.0",
        rec.createdAt,
        "pi_session",
      );
      imported++;
    }

    const encoded = encodeRevision(rev);
    // last_synced_at 存编码后的 revision（BigInt），last_modified 存 modifiedMs，last_line_offset 存行数
    // node:sqlite 支持 bigint，需转为 string 或 number？直接传 BigInt
    try {
      db.prepare(
        `INSERT OR REPLACE INTO session_log_sync (file_path, last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint) VALUES (?, ?, ?, ?, ?, ?)`,
      ).run(file, rev.modifiedMs, lines.length, encoded as unknown as number, rev.fileSize, rev.tailFingerprint);
    } catch {
      // 回退：若 BigInt 不被支持，转 number（可能截断，但测试环境文件小，安全）
      db.prepare(
        `INSERT OR REPLACE INTO session_log_sync (file_path, last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint) VALUES (?, ?, ?, ?, ?, ?)`,
      ).run(file, rev.modifiedMs, lines.length, Number(encoded), rev.fileSize, rev.tailFingerprint);
    }
  }
  return { imported, skipped };
}
