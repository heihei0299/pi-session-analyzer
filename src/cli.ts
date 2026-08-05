/**
 * CLI 入口：token-analyzer [totals|sessions|requests] --dir <path> [--format <table|json|csv>]
 * 默认窗口 totals（issue 01 行为），默认格式 table，默认数据目录 ~/.pi/agent/sessions/。
 */
import { homedir } from "node:os";
import { realpathSync } from "node:fs";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import {
  readSessionFiles,
  totalsFromFiles,
  sessionRowsFromFiles,
  requestRowsFromFiles,
  groupRowsFromFiles,
  filterFiles,
  periodRowsFromFiles,
} from "./analyze.ts";
import type { GroupBy, Period } from "./aggregate.ts";
import { renderTotalsTable, renderSessionTable, renderRequestTable, renderGroupTable, renderPeriodTable } from "./render.ts";
import { serializeJson, serializeCsv, serializeGroupJson, serializeGroupCsv, serializePeriodJson, serializePeriodCsv } from "./serialize.ts";

const DEFAULT_DIR = join(homedir(), ".pi", "agent", "sessions");

export type WindowName = "totals" | "sessions" | "requests";
export type FormatName = "table" | "json" | "csv";

export interface CliArgs {
  window: WindowName;
  dir: string;
  format: FormatName;
  /** --model <id>：只统计指定模型（对所有窗口生效） */
  model?: string;
  /** --cwd <path>：只统计指定项目（对所有窗口生效，规范化比较） */
  cwd?: string;
  /** --by <model|cwd|model,cwd>：totals 窗口按维度分组 */
  by?: GroupBy;
  /** --since <时间>：只统计会话时间戳 ≥ 该值的会话（含端点） */
  since?: string;
  /** --until <时间>：只统计会话时间戳 ≤ 该值的会话（含端点） */
  until?: string;
  /** --period <day|week|month>：totals 窗口按周期汇总 */
  period?: Period;
}

export function parseArgs(argv: string[]): CliArgs {
  let window: WindowName = "totals";
  let dir = DEFAULT_DIR;
  let format: FormatName = "table";
  let model: string | undefined;
  let cwd: string | undefined;
  let by: GroupBy | undefined;
  let since: string | undefined;
  let until: string | undefined;
  let period: Period | undefined;
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--dir" && argv[i + 1]) {
      dir = argv[i + 1];
      i++;
    } else if (a === "--format" && argv[i + 1]) {
      const f = argv[i + 1];
      if (f === "json" || f === "csv" || f === "table") {
        format = f;
      } else {
        throw new Error(`未知格式: ${f}（支持 table/json/csv）`);
      }
      i++;
    } else if (a === "--model" && argv[i + 1]) {
      model = argv[i + 1];
      i++;
    } else if (a === "--cwd" && argv[i + 1]) {
      cwd = argv[i + 1];
      i++;
    } else if (a === "--since" && argv[i + 1]) {
      since = argv[i + 1];
      i++;
    } else if (a === "--until" && argv[i + 1]) {
      until = argv[i + 1];
      i++;
    } else if (a === "--period" && argv[i + 1]) {
      const p = argv[i + 1];
      if (p === "day" || p === "week" || p === "month") {
        period = p;
      } else {
        throw new Error(`未知周期: ${p}（支持 day/week/month）`);
      }
      i++;
    } else if (a === "--by" && argv[i + 1]) {
      const b = argv[i + 1];
      if (b === "model" || b === "cwd" || b === "model,cwd") {
        by = b;
      } else {
        throw new Error(`未知分组: ${b}（支持 model/cwd/model,cwd）`);
      }
      i++;
    } else if (a === "totals" || a === "sessions" || a === "requests") {
      window = a;
    }
  }
  return { window, dir, format, model, cwd, by, since, until, period };
}

/** 运行分析，返回输出文本（供 CLI 打印与测试断言）；目录只扫描一次，派生三窗口 */
export async function runCli(argv: string[]): Promise<string> {
  const { window, dir, format, model, cwd, by, since, until, period } = parseArgs(argv);
  // 参数合法性校验先行（IO 之前）
  if (period !== undefined && window !== "totals") {
    throw new Error(`--period 汇总仅支持 totals 窗口（当前 ${window}）`);
  }
  if (by !== undefined && window !== "totals") {
    throw new Error(`--by 分组仅支持 totals 窗口（当前 ${window}）`);
  }
  if (period !== undefined && by !== undefined) {
    throw new Error(`--period 与 --by 不能同时使用（当前 period=${period}, by=${by}）`);
  }
  const files = await readSessionFiles(dir);
  const filtered = filterFiles(files, { model, cwd, since, until });
  if (period !== undefined && window !== "totals") {
    throw new Error(`--period 汇总仅支持 totals 窗口（当前 ${window}）`);
  }
  if (by !== undefined && window !== "totals") {
    throw new Error(`--by 分组仅支持 totals 窗口（当前 ${window}）`);
  }
  if (period !== undefined && by !== undefined) {
    throw new Error(`--period 与 --by 不能同时使用（当前 period=${period}, by=${by}）`);
  }
  if (period !== undefined) {
    const rows = periodRowsFromFiles(filtered, period);
    if (format === "json") return serializePeriodJson(period, rows);
    if (format === "csv") return serializePeriodCsv(period, rows);
    return renderPeriodTable(rows, period);
  }
  if (by !== undefined) {
    const rows = groupRowsFromFiles(filtered, by);
    if (format === "json") return serializeGroupJson(by, rows);
    if (format === "csv") return serializeGroupCsv(by, rows);
    return renderGroupTable(rows, by);
  }
  const totals = totalsFromFiles(filtered);
  if (format === "json") {
    return serializeJson(window, totals, sessionRowsFromFiles(filtered), requestRowsFromFiles(filtered));
  }
  if (format === "csv") {
    return serializeCsv(window, totals, sessionRowsFromFiles(filtered), requestRowsFromFiles(filtered));
  }
  switch (window) {
    case "totals":
      return renderTotalsTable(totals);
    case "sessions":
      return renderSessionTable(sessionRowsFromFiles(filtered));
    case "requests":
      return renderRequestTable(requestRowsFromFiles(filtered));
  }
}

// 直接执行时打印（兼容 npm bin symlink：解析 realpath 后比较）
const invoked = process.argv[1];
const isDirectRun =
  invoked !== undefined &&
  import.meta.url === pathToFileURL(realpathSync(invoked)).href;
if (isDirectRun) {
  runCli(process.argv.slice(2)).then(
    (out) => process.stdout.write(out),
    (err) => {
      console.error(err);
      process.exitCode = 1;
    },
  );
}
