/**
 * 四载体解析与门控（直切 cc-switch）
 */
export type PiKind = "assistant" | "tool_result" | "compaction" | "branch_summary";

export interface PiRecord {
  kind: PiKind;
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  provider: string;
  requestModel: string;
  model: string;
  statusCode: number;
  errorMessage?: string;
  createdAt: number;
  sessionId: string;
  costTotal: number;
}

function toFinite(n: unknown): number {
  return typeof n === "number" && Number.isFinite(n) ? n : 0;
}

function parseTimestamp(v: unknown): number | null {
  if (typeof v === "string") {
    const t = Date.parse(v);
    if (!Number.isNaN(t)) return Math.floor(t / 1000);
  }
  if (typeof v === "number") {
    // 可能是 ms 或 s
    if (v > 1e12) return Math.floor(v / 1000);
    if (v > 1e9) return Math.floor(v / 1000);
    return Math.floor(v);
  }
  return null;
}

function truncateLabel(s: string): string {
  const buf = Buffer.from(s);
  if (buf.length <= 512) return s;
  let end = 512;
  while (end > 0 && (buf[end] & 0xc0) === 0x80) end--;
  return buf.subarray(0, end).toString();
}

export function parsePiUsageRecord(
  entry: Record<string, unknown>,
  sessionId: string,
  sessionTimestamp: number | null,
  fileMtime: number,
): PiRecord | null {
  const type = entry.type as string | undefined;
  let kind: PiKind | null = null;
  let usageRaw: Record<string, unknown> | null = null;
  let message: Record<string, unknown> | null = null;
  let stopReason: string | undefined;

  if (type === "message") {
    const msg = entry.message as Record<string, unknown> | undefined;
    if (!msg) return null;
    message = msg;
    const role = msg.role as string | undefined;
    if (role === "assistant") {
      kind = "assistant";
      usageRaw = msg.usage as Record<string, unknown> | null | undefined ?? null;
      stopReason = msg.stopReason as string | undefined;
    } else if (role === "toolResult") {
      kind = "tool_result";
      usageRaw = msg.usage as Record<string, unknown> | null | undefined ?? null;
    } else {
      return null;
    }
  } else if (type === "compaction") {
    kind = "compaction";
    usageRaw = entry.usage as Record<string, unknown> | null | undefined ?? null;
  } else if (type === "branch_summary") {
    kind = "branch_summary";
    usageRaw = entry.usage as Record<string, unknown> | null | undefined ?? null;
  } else {
    return null;
  }

  if (!usageRaw) return null;

  const input = toFinite(usageRaw.input);
  const output = toFinite(usageRaw.output);
  const cacheRead = toFinite(usageRaw.cacheRead);
  const cacheWrite = toFinite(usageRaw.cacheWrite);
  const costObj = usageRaw.cost as Record<string, unknown> | undefined;
  const costTotal = costObj ? toFinite(costObj.total) : 0;

  const hasBillable = input > 0 || output > 0 || cacheRead > 0 || cacheWrite > 0;
  const hasCost = costTotal > 0;
  const failed = stopReason === "error" || stopReason === "aborted";

  if (!hasBillable && !hasCost && !failed) return null;

  let provider: string;
  let requestModel: string;
  let model: string;
  if (kind === "assistant") {
    const p = message ? (message.provider as string | undefined) : undefined;
    provider = p ? truncateLabel(p) : "_pi_session";
    const req = message ? (message.model as string | undefined) : undefined;
    requestModel = req ? truncateLabel(req) : "unknown";
    const rm = message ? (message.responseModel as string | undefined) : undefined;
    model = rm ? truncateLabel(rm) : requestModel;
  } else {
    provider = "_pi_session";
    requestModel = "unknown";
    model = "unknown";
  }

  const tsFromEntry = parseTimestamp(entry.timestamp);
  const tsFromMsg = message ? parseTimestamp(message.timestamp) : null;
  let createdAt = tsFromEntry ?? tsFromMsg ?? sessionTimestamp ?? Math.floor(fileMtime / 1000);
  // 夹逼 SQLite 范围（简化：直接用）
  if (!Number.isFinite(createdAt)) createdAt = Math.floor(fileMtime / 1000);

  let statusCode = 200;
  let errorMessage: string | undefined;
  if (failed) {
    if (stopReason === "aborted") {
      statusCode = 499;
      errorMessage = "Pi request aborted";
    } else {
      statusCode = 500;
      const msgErr = message ? (message.errorMessage as string | undefined) : undefined;
      errorMessage = msgErr ?? "Pi request failed";
    }
  }

  return {
    kind,
    input,
    output,
    cacheRead,
    cacheWrite,
    provider,
    requestModel,
    model,
    statusCode,
    errorMessage,
    createdAt,
    sessionId,
    costTotal,
  };
}
