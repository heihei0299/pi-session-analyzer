/**
 * 数据读取层：遍历会话目录（含子目录），识别合法会话（首行 type==session），
 * 逐行解析并仅提取 type=message && role=assistant 且携带 usage 的消息（口径 A）。
 * 三个窗口（总 / 会话级 / 单请求级）共用同一份文件级原始数据。
 */
import { readdirSync, createReadStream } from "node:fs";
import { join } from "node:path";
import { createInterface } from "node:readline";
import {
  addUsage,
  emptyTotals,
  finalizeTotals,
  type Totals,
  type Usage,
  type SessionRow,
  type RequestRow,
} from "./aggregate.ts";

/** 递归收集目录下所有 .jsonl 文件 */
export function collectJsonlFiles(dir: string): string[] {
  const out: string[] = [];
  const walk = (d: string): void => {
    for (const entry of readdirSync(d, { withFileTypes: true })) {
      const p = join(d, entry.name);
      if (entry.isDirectory()) walk(p);
      else if (entry.isFile() && entry.name.endsWith(".jsonl")) out.push(p);
    }
  };
  walk(dir);
  return out.sort();
}

/** 单个会话文件解析出的原始数据（仅计入口径的消息） */
export interface SessionFileData {
  sessionId: string;
  timestamp: string;
  cwd: string;
  items: { timestamp: string; model: string; usage: Usage }[];
}

/**
 * 扫描会话目录，解析全部合法会话文件的计入口径消息。
 * 残留文件（首行非 session）被跳过；返回空数组表示目录无合法会话。
 */
export async function readSessionFiles(dir: string): Promise<SessionFileData[]> {
  const out: SessionFileData[] = [];
  for (const file of collectJsonlFiles(dir)) {
    const data = await analyzeFile(file);
    if (data !== null) out.push(data);
  }
  return out;
}

/** 从文件级原始数据派生总窗口（不重扫目录） */
export function totalsFromFiles(files: SessionFileData[]): Totals {
  const totals = emptyTotals();
  for (const file of files) {
    for (const item of file.items) addUsage(totals, item.usage);
  }
  finalizeTotals(totals);
  return totals;
}

/** 从文件级原始数据派生会话级窗口；无计入消息的会话 model 为 "-" */
export function sessionRowsFromFiles(files: SessionFileData[]): SessionRow[] {
  return files.map((file) => {
    const totals = emptyTotals();
    const models = new Set<string>();
    for (const item of file.items) {
      addUsage(totals, item.usage);
      models.add(item.model);
    }
    finalizeTotals(totals);
    return {
      sessionId: file.sessionId,
      timestamp: file.timestamp,
      cwd: file.cwd,
      model: models.size === 0 ? "-" : models.size === 1 ? [...models][0] : "mixed",
      ...totals,
    };
  });
}

/** 从文件级原始数据派生单请求级窗口 */
export function requestRowsFromFiles(files: SessionFileData[]): RequestRow[] {
  const rows: RequestRow[] = [];
  for (const file of files) {
    for (const item of file.items) {
      const totals = emptyTotals();
      addUsage(totals, item.usage); // 单请求：requests 恒 1
      finalizeTotals(totals);
      rows.push({
        sessionId: file.sessionId,
        timestamp: item.timestamp,
        model: item.model,
        ...totals,
      });
    }
  }
  return rows;
}

/** 解析单个 JSONL 文件；残留文件返回 null */
async function analyzeFile(file: string): Promise<SessionFileData | null> {
  const rl = createInterface({ input: createReadStream(file, "utf8"), crlfDelay: Infinity });
  let header: { id?: unknown; timestamp?: unknown; cwd?: unknown } | null = null;
  const items: SessionFileData["items"] = [];
  try {
    let firstLine = true;
    for await (const line of rl) {
      if (firstLine) {
        firstLine = false;
        const entry = parseJson(line);
        if (entry === null || entry.type !== "session") return null;
        header = {
          id: entry.id,
          timestamp: entry.timestamp,
          cwd: entry.cwd,
        };
        continue;
      }
      if (!line.trim()) continue;
      const entry = parseJson(line);
      if (entry === null) continue; // 坏行跳过（不中断整个文件）
      if (entry.type === "message") {
        const msg = entry.message;
        if (
          msg !== null &&
          typeof msg === "object" &&
          !Array.isArray(msg) &&
          (msg as Record<string, unknown>).role === "assistant" &&
          (msg as Record<string, unknown>).usage != null
        ) {
          const m = msg as Record<string, unknown>;
          items.push({
            timestamp: typeof entry.timestamp === "string" ? entry.timestamp : "",
            model: typeof m.model === "string" ? m.model : "",
            usage: m.usage as Usage,
          });
        }
      }
    }
  } finally {
    rl.close();
  }
  if (header === null) return null;
  return {
    sessionId: typeof header.id === "string" ? header.id : "",
    timestamp: typeof header.timestamp === "string" ? header.timestamp : "",
    cwd: typeof header.cwd === "string" ? header.cwd : "",
    items,
  };
}

function parseJson(line: string): Record<string, unknown> | null {
  try {
    const v = JSON.parse(line) as unknown;
    if (v !== null && typeof v === "object" && !Array.isArray(v)) {
      return v as Record<string, unknown>;
    }
    return null;
  } catch {
    return null;
  }
}
