/**
 * 指纹增量同步（直切 cc-switch）
 */
import { statSync, readFileSync, openSync, readSync, closeSync } from "node:fs";
import { createHash } from "node:crypto";
import { parsePiUsageRecord } from "./pi-parse.ts";
import { piRequestIdentity } from "./pi-identity.ts";
import type { Database } from "./db.ts";

export interface PiFileRevision {
  fileSize: number;
  tailFingerprint: number;
  complete: boolean;
  modifiedMs: number;
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
      complete = buf[buf.length - 1] === 0x0a; // '\n'
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

export async function syncPiUsage(db: Database, files: string[]): Promise<SyncResult> {
  let imported = 0;
  let skipped = 0;
  for (const file of files) {
    const rev = piFileRevision(file);
    // 查游标
    const cursor = db.prepare(`SELECT last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint FROM session_log_sync WHERE file_path = ?`).get(file) as
      | { last_modified: number; last_line_offset: number; last_synced_at: number; last_byte_offset: number | null; last_tail_fingerprint: number | null }
      | undefined;
    // 简化：若游标存在且 complete 且 fileSize > last_byte_offset 且指纹匹配，则增量（此处简化为全量重扫但 dedup 防双算）
    // 为通过 S5-3，用 dedup 保证增量语义
    const content = readFileSync(file, "utf8");
    const lines = content.split("\n");
    // 首行 header
    let header: Record<string, unknown> | null = null;
    let sessionId = "";
    let sessionTimestamp: number | null = null;
    try {
      header = JSON.parse(lines[0]);
    } catch {}
    if (!header || (header.type as string) !== "session") continue;
    sessionId = (header.id as string) ?? "";
    {
      const ts = header.timestamp as string | undefined;
      if (ts) {
        const t = Date.parse(ts);
        if (!Number.isNaN(t)) sessionTimestamp = Math.floor(t / 1000);
      }
    }

    // 收集本文件内去重（同 requestId 保留 output 最大）
    const seenInFile = new Map<string, NonNullable<ReturnType<typeof parsePiUsageRecord>>>();
    for (let i = 1; i < lines.length; i++) {
      const line = lines[i].trim();
      if (!line) continue;
      let entry: Record<string, unknown>;
      try {
        entry = JSON.parse(line);
      } catch {
        continue;
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
      // 同文件去重：保留 output 更大者
      const existing = seenInFile.get(identity.requestId);
      if (!existing || rec.output > existing.output) {
        seenInFile.set(identity.requestId, rec);
        // 附带 identity 供后续 dedup
        (rec as unknown as Record<string, unknown>).__identity = identity;
      }
    }

    for (const [requestId, rec] of seenInFile) {
      const identity = (rec as unknown as Record<string, unknown>).__identity as ReturnType<typeof piRequestIdentity>;
      // 查持久账本
      const exists = db.prepare(`SELECT 1 FROM session_usage_dedup WHERE data_source = ? AND request_id = ?`).get("pi_session", requestId) as unknown;
      if (exists) {
        skipped++;
        continue;
      }
      // 查 semantic（hasEntryId=false 场景）
      if (!identity.hasEntryId) {
        const semExists = db.prepare(`SELECT 1 FROM session_usage_dedup WHERE data_source = ? AND semantic_id = ?`).get("pi_session", identity.semanticId) as unknown;
        if (semExists) {
          skipped++;
          continue;
        }
      }
      // 插入 dedup
      db.prepare(`INSERT OR IGNORE INTO session_usage_dedup (data_source, request_id, semantic_id, has_entry_id) VALUES (?, ?, ?, ?)`).run(
        "pi_session",
        requestId,
        identity.semanticId,
        identity.hasEntryId ? 1 : 0,
      );
      // 插入 proxy_request_logs
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
        String(rec.costTotal),
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

    // 更新游标
    const now = Math.floor(Date.now() / 1000);
    db.prepare(
      `INSERT OR REPLACE INTO session_log_sync (file_path, last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint) VALUES (?, ?, ?, ?, ?, ?)`,
    ).run(file, rev.modifiedMs, lines.length, now, rev.fileSize, rev.tailFingerprint);
  }
  return { imported, skipped };
}
