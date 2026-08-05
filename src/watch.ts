/**
 * 实时监控增量读取器（--watch）。
 * 基于会话 JSONL append-only + 消息级即时写盘：为每个会话文件维护读取 offset，
 * 续读新追加行，仅解析完整一行 JSON 记录；增量边界 = 一行完整
 * type=message && role=assistant 且含 usage 的 entry（口径 A）。
 *
 * 重同步：轮询比对 inode / size / mtime，检测文件替换或截断后整体重读，
 * 并通过「扣减旧贡献 + 累加新内容」保证不丢不重。
 */
import { openSync, readSync, statSync, closeSync, createReadStream } from "node:fs";
import { createInterface } from "node:readline";
import { emptyTotals, finalizeTotals, addUsage, type Totals, type Usage } from "./aggregate.ts";
import { collectJsonlFiles } from "./analyze.ts";

/** 单个文件的增量读取状态 */
interface FileState {
  /** 已读取到的字节 offset（完整行边界，下次从此续读） */
  offset: number;
  /** inode（用于文件替换检测） */
  inode: number;
  /** mtime ms（用于截断重写到同尺寸检测） */
  mtimeMs: number;
}

/** 增量读取结果：本次新增的计入口径消息（单请求 Totals 形状；替换重读含负值扣减项） */
export interface Increment extends Totals {
  /** 来源会话文件路径（便于定位） */
  file: string;
}


export class IncrementalReader {
  private states = new Map<string, FileState>();
  /** 每个文件的累计贡献（用于替换/截断重读时扣减，避免重复计数） */
  private contrib = new Map<string, Totals>();
  private dir: string;

  constructor(dir: string) {
    this.dir = dir;
  }

  /**
   * 读取自上次以来的增量，返回新计入口径消息。
   * - 首次读取（未跟踪文件）：从头解析全部内容（= 静态统计基线）
   * - 正常追加：从上次 offset 续读到文件尾（仅完整行）
   * - 文件被替换（inode 变化）/截断（size < offset）/重写（mtime 变化）：
   *   整体重读，先输出负的旧贡献（扣减）再输出新内容，不丢不重
   * - 残留文件（首行非 session）不纳入（与静态分析口径一致）
   */
  async readIncrements(): Promise<Increment[]> {
    const increments: Increment[] = [];
    // 异步过滤：仅保留合法会话文件（首行 type==session）
    const files: string[] = [];
    for (const f of collectJsonlFiles(this.dir)) {
      if (await isSessionFile(f)) files.push(f);
    }
    const seen = new Set(files);

    for (const [file, state] of [...this.states]) {
      if (!seen.has(file)) {
        this.states.delete(file);
        this.contrib.delete(file); // 文件删除：其贡献保留在 totals 中（历史已发生）
        continue;
      }
      const st = statSync(file);
      // 替换（inode 变）/截断（size < offset）/同尺寸重写（mtime 变且 size 未增）→ 整体重读
      if (st.ino !== state.inode || st.size < state.offset || (st.size === state.offset && st.mtimeMs !== state.mtimeMs)) {
        // 文件替换/截断/重写：扣减旧贡献，重读新内容
        const prev = this.contrib.get(file);
        if (prev) increments.push(negate(prev, file));
        const { totals: added } = await readEntriesFrom(file, 0, increments);
        this.states.set(file, stateOf(st));
        this.contrib.set(file, added);
      } else if (st.size > state.offset) {
        // 正常追加：从 offset 续读（offset 恒为完整行边界）
        const { totals: added, bytesRead } = await readEntriesFrom(file, state.offset, increments);
        const prev = this.contrib.get(file);
        this.contrib.set(file, merge(prev, added));
        this.states.set(file, { ...state, offset: state.offset + bytesRead, mtimeMs: st.mtimeMs });
      }
    }

    // 处理未跟踪文件（首次出现：从头读取全部；新建文件）
    for (const file of files) {
      if (!this.states.has(file)) {
        const st = statSync(file);
        const { totals: added } = await readEntriesFrom(file, 0, increments);
        this.states.set(file, stateOf(st));
        this.contrib.set(file, added);
      }
    }

    return increments;
  }
}

/** 记录文件状态（offset=当前 size，inode，mtimeMs） */
function stateOf(st: { size: number | bigint; ino: number | bigint; mtimeMs: number }): FileState {
  return { offset: Number(st.size), inode: Number(st.ino), mtimeMs: st.mtimeMs };
}

/** 合法会话判定：文件首行 type == "session"（与静态分析口径一致，流式读首行无长度限制） */
async function isSessionFile(file: string): Promise<boolean> {
  const rl = createInterface({ input: createReadStream(file, "utf8"), crlfDelay: Infinity });
  try {
    for await (const line of rl) {
      if (!line.trim()) continue;
      const entry = JSON.parse(line) as Record<string, unknown>;
      return entry.type === "session";
    }
    return false;
  } catch {
    return false;
  } finally {
    rl.close();
  }
}

/**
 * 从指定字节 offset 读取文件中的完整行，解析计入口径消息并 push 到 out。
 * 返回该文件的本次净贡献（Totals）；out 中同步追加解析出的单请求条目。
 * offset 语义：调用方保证其位于完整行边界；读取后文件尾若有不完整行（无 \n），
 * 不解析且新的 offset 停在最后完整行之后（下轮从此续读，不丢半行）。
 */
async function readEntriesFrom(
  file: string,
  offset: number,
  out: Increment[],
): Promise<{ totals: Totals; bytesRead: number }> {
  const st = statSync(file);
  const fd = openSync(file, "r");
  const buf = Buffer.alloc(Math.max(0, st.size - offset));
  let total = 0;
  if (buf.length > 0) {
    let pos = 0;
    while (pos < buf.length) {
      const n = readSync(fd, buf, pos, buf.length - pos, offset + pos);
      if (n <= 0) break;
      pos += n;
    }
    total = pos;
  }
  closeSync(fd);

  const text = buf.subarray(0, total).toString("utf8");
  const lines = text.split("\n");
  let bytesRead = total;
  if (text.endsWith("\n")) {
    lines.pop(); // split 产生的尾部空串
  } else {
    // 最后一段无 \n：可能是不完整行，丢弃且不回读（bytesRead 停在最后完整行之后）
    const last = lines.pop() ?? "";
    bytesRead -= Buffer.byteLength(last);
  }

  const totals = emptyTotals();
  for (const line of lines) {
    if (!line.trim()) continue;
    const usage = extractUsage(line);
    if (usage === null) continue;
    addUsage(totals, usage);
    const t = emptyTotals();
    addUsage(t, usage);
    finalizeTotals(t);
    out.push({ ...t, file });
  }
  finalizeTotals(totals);
  return { totals, bytesRead };
}

/** 合并两份 Totals（contrib 累计用） */
function merge(a: Totals | undefined, b: Totals): Totals {
  const out = emptyTotals();
  if (a) {
    out.requests = a.requests;
    out.input = a.input;
    out.output = a.output;
    out.cacheRead = a.cacheRead;
    out.cacheWrite = a.cacheWrite;
    out.reasoning = a.reasoning;
    out.cost = a.cost;
  }
  out.requests += b.requests;
  out.input += b.input;
  out.output += b.output;
  out.cacheRead += b.cacheRead;
  out.cacheWrite += b.cacheWrite;
  out.reasoning += b.reasoning;
  out.cost += b.cost;
  finalizeTotals(out);
  return out;
}

/** 生成负贡献（重读扣减项） */
function negate(t: Totals, file: string): Increment {
  return {
    requests: -t.requests,
    input: -t.input,
    output: -t.output,
    cacheRead: -t.cacheRead,
    cacheWrite: -t.cacheWrite,
    reasoning: -t.reasoning,
    totalTokens: 0,
    cost: -t.cost,
    cacheRate: 0,
    file,
  };
}

/** 从一行 JSON 提取计入口径 usage；非口径 A 行返回 null */
function extractUsage(line: string): Usage | null {
  try {
    const entry = JSON.parse(line) as Record<string, unknown>;
    if (entry.type !== "message") return null;
    const msg = entry.message as Record<string, unknown> | null;
    if (msg === null || typeof msg !== "object" || Array.isArray(msg)) return null;
    if (msg.role !== "assistant" || msg.usage == null) return null;
    return msg.usage as Usage;
  } catch {
    return null;
  }
}

/** Totals → Usage 转换（contrib 累加复用 addUsage） */
function toUsage(t: Totals): Usage {
  return {
    input: t.input,
    output: t.output,
    cacheRead: t.cacheRead,
    cacheWrite: t.cacheWrite,
    reasoning: t.reasoning,
    cost: { total: t.cost },
  };
}

/**
 * --watch 单步：读取增量并累加到 totals。
 * 返回是否有新增量（供外层循环判断是否刷新输出）。
 */
export async function applyIncrements(reader: IncrementalReader, totals: Totals): Promise<boolean> {
  const increments = await reader.readIncrements();
  for (const inc of increments) {
    totals.requests += inc.requests;
    totals.input += inc.input;
    totals.output += inc.output;
    totals.cacheRead += inc.cacheRead;
    totals.cacheWrite += inc.cacheWrite;
    totals.reasoning += inc.reasoning;
    totals.cost += inc.cost;
  }
  finalizeTotals(totals);
  return increments.length > 0;
}
