/**
 * 数据读取层：遍历会话目录（含子目录），识别合法会话（首行 type==session），
 * 逐行解析并仅提取 type=message && role=assistant 且携带 usage 的消息（口径 A）。
 * 三个窗口（总 / 会话级 / 单请求级）共用同一份文件级原始数据。
 */
import { readdirSync, createReadStream, realpathSync, statSync } from "node:fs";
import { join, resolve, basename } from "node:path";
import { createInterface } from "node:readline";
import {
  addUsage,
  emptyTotals,
  finalizeTotals,
  type Totals,
  type Usage,
  type SessionRow,
  type RequestRow,
  type GroupRow,
  type GroupBy,
  type PeriodRow,
  type Period,
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

/** 单个会话文件解析出的原始数据（计入口径的消息 + 会话元数据） */
export interface SessionFileData {
  sessionId: string;
  timestamp: string;
  cwd: string;
  /** 来源文件名（webui /api/sessions 扩展字段 fileName 用） */
  fileName?: string;
  /** 首条 user 消息的文本内容（webui 会话管理显示名用；无 user 消息时为 undefined） */
  firstUserText?: string;
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

/**
 * readSessionFiles 结果缓存（webui serve 专用）：目录文件 (path, mtimeMs, size) 快照失效，
 * 数据不变时直接返回缓存，避免每次请求全量重读重解析（222 会话 ≈ 0.5s 单核 CPU）。
 * CLI/watch 仍走无缓存 readSessionFiles，保证实时语义。
 */
const sessionFileCache = new Map<string, { sig: string; data: SessionFileData[] }>();
const sessionFileInflight = new Map<string, Promise<SessionFileData[]>>();

export async function readSessionFilesCached(dir: string): Promise<SessionFileData[]> {
  const files = collectJsonlFiles(dir);
  const sig = files
    .map((f) => {
      const st = statSync(f);
      return `${f}:${st.mtimeMs}:${st.size}`;
    })
    .join("|");
  const hit = sessionFileCache.get(dir);
  if (hit !== undefined && hit.sig === sig) return hit.data;
  // 并发去重：同一快照的读取共享一个 Promise（打开页面 3 并发请求只重读一次，避免峰值放大）
  const key = dir + "|" + sig;
  const pending = sessionFileInflight.get(key);
  if (pending !== undefined) return pending;
  const p = readSessionFiles(dir)
    .then((data) => {
      sessionFileCache.set(dir, { sig, data });
      return data;
    })
    .finally(() => sessionFileInflight.delete(key));
  sessionFileInflight.set(key, p);
  return p;
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


/** 规范化 cwd：绝对路径、去尾斜杠、解析符号链接（不存在时回退 resolve） */
export function normalizeCwd(cwd: string): string {
  // 相对路径也 resolve 为绝对路径（相对当前工作目录），再规范化
  const abs = resolve(cwd).replace(/\/+$/, "") || "/";
  try {
    return realpathSync(abs);
  } catch {
    return abs;
  }
}

/**
 * 从文件级原始数据派生维度分组窗口（--by model|cwd|model,cwd）。
 * 模型键取每条消息的 model（请求级权威归属）；cwd 键取规范化后的 header cwd。
 */
export function groupRowsFromFiles(files: SessionFileData[], by: GroupBy): GroupRow[] {
  const byModel = by === "model" || by === "model,cwd";
  const byCwd = by === "cwd" || by === "model,cwd";
  // 文件级 cwd 规范化缓存（避免逐消息 realpath）
  const normCwdCache = new Map<string, string>();
  const normOf = (f: SessionFileData): string => {
    let n = normCwdCache.get(f.cwd);
    if (n === undefined) {
      n = normalizeCwd(f.cwd);
      normCwdCache.set(f.cwd, n);
    }
    return n;
  };
  const map = new Map<string, GroupRow>();
  for (const file of files) {
    for (const item of file.items) {
      const keyParts: string[] = [];
      const row: GroupRow = { ...emptyTotals() };
      if (byModel) {
        row.model = item.model;
        keyParts.push(`m:${item.model}`);
      }
      if (byCwd) {
        row.cwd = normOf(file);
        keyParts.push(`c:${row.cwd}`);
      }
      const key = keyParts.join("|");
      let g = map.get(key);
      if (!g) {
        g = row;
        map.set(key, g);
      }
      addUsage(g, item.usage);
    }
  }
  for (const g of map.values()) finalizeTotals(g);
  return [...map.values()];
}

/**
 * 按模型/cwd/时间范围过滤文件级原始数据。
 * - model 过滤：仅保留 items 中 model 匹配的消息；完全无匹配消息的会话被隐藏（不输出全 0 行）
 * - cwd 过滤：仅保留 header cwd 规范化后等于过滤值的会话
 * - since/until 过滤：仅保留会话 header timestamp 落在闭区间 [since, until] 内的会话
 */
export function filterFiles(
  files: SessionFileData[],
  filters: { model?: string; cwd?: string; since?: string; until?: string },
): SessionFileData[] {
  const { model, cwd, since, until } = filters;
  const normCwd = cwd !== undefined ? normalizeCwd(cwd) : undefined;
  // since 取当天开始（00:00）；until 取当天末尾（23:59:59.999）——日期参数含整天
  const sinceMs = since !== undefined ? parseTimestamp(since, false) : undefined;
  const untilMs = until !== undefined ? parseTimestamp(until, true) : undefined;
  return files
    .filter((f) => normCwd === undefined || normalizeCwd(f.cwd) === normCwd)
    .filter((f) => {
      if (sinceMs === undefined && untilMs === undefined) return true;
      const ts = parseUtcTimestamp(f.timestamp);
      if (Number.isNaN(ts)) return true; // 无有效时间戳的会话不参与时间筛选
      if (sinceMs !== undefined && ts < sinceMs) return false;
      if (untilMs !== undefined && ts > untilMs) return false;
      return true;
    })
    .map((f) => (model === undefined ? f : { ...f, items: f.items.filter((i) => i.model === model) }))
    .filter((f) => model === undefined || f.items.length > 0);
}

/**
 * 统一按 UTC 解析时间戳：无时区标记的字符串（如 "2026-08-01T10:00:00"）
 * 补 Z 按 UTC 解释，与带 Z 后缀的真实 pi 会话时间戳同一基准。
 */
export function parseUtcTimestamp(s: string): number {
  const hasTz = /(?:Z|[+-]\d{2}:?\d{2})$/.test(s);
  return Date.parse(hasTz ? s : s + "Z");
}
/**
 * 解析时间参数（--since/--until）：ISO 日期（2026-08-01）或完整时间戳。
 * 日期参数：since 用 endOfDay=false（当天 00:00 起）；until 用 endOfDay=true（当天 23:59:59.999 止）。
 * 完整时间戳统一按 UTC 解释（无时区后缀补 Z），与 parseUtcTimestamp/README「按 UTC 解释」一致。
 */
export function parseTimestamp(s: string, endOfDay: boolean): number {
  // 纯日期（YYYY-MM-DD）：手动构造，避免 Date.parse 的 UTC 00:00 语义歧义
  const dateOnly = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s);
  if (dateOnly) {
    const [, y, mo, d] = dateOnly;
    const base = Date.UTC(Number(y), Number(mo) - 1, Number(d));
    return endOfDay ? base + 86_400_000 - 1 : base;
  }
  const ms = Date.parse(/(?:Z|[+-]\d{2}:?\d{2})$/.test(s) ? s : s + "Z");
  if (Number.isNaN(ms)) throw new Error(`无效时间: ${s}（支持 ISO 日期或时间戳）`);
  return ms;
}

/**
 * 从文件级原始数据派生时间周期汇总窗口（--period day|week|month）。
 * 归属基准：会话 header timestamp（spec 决策锚）；周期键 = 周期起始日期。
 */
export function periodRowsFromFiles(files: SessionFileData[], period: Period): PeriodRow[] {
  const map = new Map<string, PeriodRow>();
  for (const file of files) {
    const key = periodKey(file.timestamp, period);
    if (key === null) continue; // 无有效时间戳的会话不归属任何周期
    let g = map.get(key);
    if (!g) {
      g = { period: key, ...emptyTotals() };
      map.set(key, g);
    }
    for (const item of file.items) addUsage(g, item.usage);
  }
  for (const g of map.values()) finalizeTotals(g);
  // 按周期起始日期升序
  return [...map.values()].sort((a, b) => a.period.localeCompare(b.period));
}

/** 计算会话时间戳归属的周期键（ISO 日期字符串）；无有效时间戳返回 null */
export function periodKey(timestamp: string, period: Period): string | null {
  // 统一 UTC 基准解析（无时区后缀补 Z），与 filterFiles 同一基准
  const d = new Date(parseUtcTimestamp(timestamp));
  if (Number.isNaN(d.getTime())) return null;
  // 用 UTC 字段避免时区偏移（会话时间戳为 UTC ISO 格式）
  const y = d.getUTCFullYear();
  const m = d.getUTCMonth();
  const day = d.getUTCDate();
  if (period === "day") {
    return `${pad(y, 4)}-${pad(m + 1, 2)}-${pad(day, 2)}`;
  }
  if (period === "month") {
    return `${pad(y, 4)}-${pad(m + 1, 2)}-01`;
  }
  // week：ISO 周，周一起始 → 该周周一的日期
  const dow = (d.getUTCDay() + 6) % 7; // 0=周一 ... 6=周日
  const monday = new Date(Date.UTC(y, m, day - dow));
  return `${pad(monday.getUTCFullYear(), 4)}-${pad(monday.getUTCMonth() + 1, 2)}-${pad(monday.getUTCDate(), 2)}`;
}

function pad(n: number, w: number): string {
  return String(n).padStart(w, "0");
}

/** 解析单个 JSONL 文件；残留文件返回 null */
async function analyzeFile(file: string): Promise<SessionFileData | null> {
  const rl = createInterface({ input: createReadStream(file, "utf8"), crlfDelay: Infinity });
  let header: { id?: unknown; timestamp?: unknown; cwd?: unknown } | null = null;
  const items: SessionFileData["items"] = [];
  let firstUserText: string | undefined;
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
        // 首条 user 消息的文本内容（会话管理显示名用；取 content 第一个 text）
        if (
          firstUserText === undefined &&
          msg !== null &&
          typeof msg === "object" &&
          !Array.isArray(msg) &&
          (msg as Record<string, unknown>).role === "user"
        ) {
          const content = (msg as Record<string, unknown>).content;
          if (Array.isArray(content)) {
            for (const part of content) {
              if (part !== null && typeof part === "object" && !Array.isArray(part) && (part as Record<string, unknown>).type === "text") {
                const text = String((part as Record<string, unknown>).text ?? "");
                if (text.trim()) { firstUserText = text.trim(); break; }
              }
            }
          }
        }
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
    fileName: basename(file),
    firstUserText,
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
