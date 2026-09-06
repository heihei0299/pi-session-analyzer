/**
 * 双账本去重（直切 cc-switch）
 */
import { createHash } from "node:crypto";

export interface PiIdentity {
  requestId: string;
  semanticId: string;
  hasEntryId: boolean;
}

function writeField(hasher: ReturnType<typeof createHash>, data: Buffer | string): void {
  const buf = typeof data === "string" ? Buffer.from(data) : data;
  const len = Buffer.allocUnsafe(8);
  len.writeBigUInt64BE(BigInt(buf.length));
  hasher.update(len);
  hasher.update(buf);
}

function hashJson(hasher: ReturnType<typeof createHash>, value: unknown): void {
  if (value === null) {
    writeField(hasher, "null");
    return;
  }
  const t = typeof value;
  if (t === "boolean") {
    writeField(hasher, "bool");
    writeField(hasher, value ? "true" : "false");
    return;
  }
  if (t === "number") {
    writeField(hasher, "number");
    writeField(hasher, String(value));
    return;
  }
  if (t === "string") {
    writeField(hasher, "string");
    writeField(hasher, value as string);
    return;
  }
  if (Array.isArray(value)) {
    writeField(hasher, "array");
    const len = Buffer.allocUnsafe(8);
    len.writeBigUInt64BE(BigInt(value.length));
    hasher.update(len);
    for (const v of value) hashJson(hasher, v);
    return;
  }
  if (t === "object") {
    writeField(hasher, "object");
    const obj = value as Record<string, unknown>;
    const keys = Object.keys(obj).sort();
    const len = Buffer.allocUnsafe(8);
    len.writeBigUInt64BE(BigInt(keys.length));
    hasher.update(len);
    for (const k of keys) {
      writeField(hasher, k);
      hashJson(hasher, obj[k]);
    }
    return;
  }
}

export function hashField(data: Buffer): Buffer {
  const h = createHash("sha256");
  const len = Buffer.allocUnsafe(8);
  len.writeBigUInt64BE(BigInt(data.length));
  h.update(len);
  h.update(data);
  return h.digest();
}

export function piRequestIdentity(
  entry: Record<string, unknown>,
  kind: string,
  usage: Record<string, unknown>,
  message: Record<string, unknown> | null | undefined,
): PiIdentity {
  // semantic
  const sem = createHash("sha256");
  writeField(sem, "pi-session-semantic-v1");
  writeField(sem, kind);
  const entryTs = entry.timestamp as string | undefined;
  if (entryTs) {
    writeField(sem, "entry_timestamp");
    writeField(sem, entryTs);
  }
  if (message) {
    const mt = message.timestamp as unknown;
    if (mt !== undefined) {
      writeField(sem, "message_timestamp");
      // hash the timestamp value via hashJson
      hashJson(sem, mt);
    }
    for (const key of ["provider", "model", "responseModel", "responseId", "api", "toolCallId", "toolName", "stopReason", "errorMessage"]) {
      const v = message[key];
      if (v !== undefined) {
        writeField(sem, key);
        hashJson(sem, v);
      }
    }
    const content = message.content;
    if (content !== undefined) {
      writeField(sem, "content");
      hashJson(sem, content);
    }
  } else {
    const summary = entry.summary;
    if (summary !== undefined) {
      writeField(sem, "summary");
      hashJson(sem, summary);
    }
  }
  writeField(sem, "usage");
  hashJson(sem, usage);
  const semanticId = `pi_session_semantic:${sem.digest("hex")}`;

  const entryId = entry.id as string | undefined;
  const hasEntryId = typeof entryId === "string" && entryId !== "";
  let requestId: string;
  if (hasEntryId) {
    const req = createHash("sha256");
    writeField(req, "pi-session-request-v3");
    writeField(req, kind);
    writeField(req, entryId);
    if (entryTs) hashJson(req, entryTs);
    requestId = `pi_session:${req.digest("hex")}`;
  } else {
    requestId = semanticId;
  }

  return { requestId, semanticId, hasEntryId };
}
