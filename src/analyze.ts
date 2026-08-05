/**
 * 数据读取层：遍历会话目录（含子目录），识别合法会话（首行 type==session），
 * 逐行解析并仅提取 type=message && role=assistant 且携带 usage 的消息（口径 A）。
 */
import { readdirSync, createReadStream } from "node:fs";
import { join } from "node:path";
import { createInterface } from "node:readline";
import { addUsage, emptyTotals, finalizeTotals, type Totals, type Usage } from "./aggregate.ts";

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

/**
 * 扫描会话目录，返回总消耗量窗口指标。
 * 合法会话判定：文件首行 type == "session"；其余（message/custom 等）跳过。
 */
export async function analyzeSessionDir(dir: string): Promise<Totals> {
  const totals = emptyTotals();
  for (const file of collectJsonlFiles(dir)) {
    await analyzeFile(file, totals);
  }
  finalizeTotals(totals);
  return totals;
}

/** 解析单个 JSONL 文件，将计入口径的 usage 累加到 totals */
async function analyzeFile(file: string, totals: Totals): Promise<void> {
  const rl = createInterface({ input: createReadStream(file, "utf8"), crlfDelay: Infinity });
  let isSession = false;
  let firstLine = true;
  try {
    for await (const line of rl) {
      // 首行（含空行）决定会话合法性；非 session 立即跳过整个文件
      if (firstLine) {
        firstLine = false;
        isSession = parseType(line) === "session";
        if (!isSession) break;
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
          (msg as Record<string, unknown>).role === "assistant" &&
          (msg as Record<string, unknown>).usage != null
        ) {
          addUsage(totals, (msg as Record<string, unknown>).usage as Usage);
        }
      }
    }
  } finally {
    rl.close();
  }
}

function parseType(line: string): unknown {
  const entry = parseJson(line);
  return entry === null ? undefined : entry.type;
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
