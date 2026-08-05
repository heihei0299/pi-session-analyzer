#!/usr/bin/env node
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
import { IncrementalReader, applyIncrements } from "./watch.ts";
import { emptyTotals, type GroupBy, type Period, type Totals } from "./aggregate.ts";
import { renderTotalsTable, renderSessionTable, renderRequestTable, renderGroupTable, renderPeriodTable } from "./render.ts";
import { serializeJson, serializeCsv, serializeGroupJson, serializeGroupCsv, serializePeriodJson, serializePeriodCsv } from "./serialize.ts";
import { startWebServer } from "./server.ts";

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
  /** --watch：实时监控模式（长驻，跟随追加） */
  watch: boolean;
  /** --interval <ms>：watch 轮询间隔（默认 1000） */
  interval: number;
  /** serve：Web 服务器模式（serve 子命令） */
  serve: boolean;
  /** --port <n>：serve 监听端口（默认 50080） */
  port: number;
  /** --host <h>：serve 监听地址（默认 127.0.0.1） */
  host: string;
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
  let watch = false;
  let interval = 1000;
  let serve = false;
  let port = 50080;
  let host = "127.0.0.1";
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "--dir" && argv[i + 1]) {
      dir = argv[i + 1];
      i++;
    } else if (a === "--port" && argv[i + 1]) {
      const n = Number(argv[i + 1]);
      if (!Number.isInteger(n) || n < 0 || n > 65535) {
        throw new Error(`无效端口: ${argv[i + 1]}（需为 0-65535 的整数）`);
      }
      port = n;
      i++;
    } else if (a === "--host" && argv[i + 1]) {
      host = argv[i + 1];
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
    } else if (a === "--watch") {
      watch = true;
    } else if (a === "--interval" && argv[i + 1]) {
      const ms = Number(argv[i + 1]);
      if (!Number.isFinite(ms) || ms <= 0) {
        throw new Error(`无效间隔: ${argv[i + 1]}（需为正数毫秒）`);
      }
      interval = ms;
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
    } else if (a === "serve") {
      serve = true;
    }
  }
  if (serve) {
    validateServeMode(argv);
  }
  return { window, dir, format, model, cwd, by, since, until, period, watch, interval, serve, port, host };
}

/** serve 模式参数校验：仅允许 --port/--host/--dir，其余一律拒绝（避免静默忽略） */
function validateServeMode(argv: string[]): void {
  const FORBIDDEN = [
    "totals",
    "sessions",
    "requests",
    "--format",
    "--by",
    "--period",
    "--watch",
    "--interval",
    "--model",
    "--cwd",
    "--since",
    "--until",
  ];
  const hit = argv.find((a) => FORBIDDEN.includes(a));
  if (hit !== undefined) {
    throw new Error(`serve 模式仅支持 --port/--host/--dir 参数（收到 ${hit}）`);
  }
}

/** serve 子命令：启动 Web 服务器、打印访问 URL、Ctrl+C 优雅退出（长驻） */
async function runServeCli(args: { dir: string; host: string; port: number }): Promise<string> {
  const server = await startWebServer({ dir: args.dir, host: args.host, port: args.port });
  // SIGINT handler 先于 URL 打印注册：URL 打印即代表优雅退出已就绪（消除 kill 竞态）
  const exited = new Promise<void>((resolve) => {
    process.once("SIGINT", () => {
      server.close().then(resolve);
    });
  });
  process.stdout.write(`Token Analyzer WebUI: ${server.url}\n`);
  await exited;
  return "";
}
/** 运行分析，返回输出文本（供 CLI 打印与测试断言）；目录只扫描一次，派生三窗口 */
export async function runCli(argv: string[]): Promise<string> {
  const { window, dir, format, model, cwd, by, since, until, period, watch, interval, serve, port, host } = parseArgs(argv);
  if (serve) {
    // Web 服务器模式：启动、打印访问 URL、Ctrl+C 优雅退出（长驻，正常退出时返回空串）
    return runServeCli({ dir, host, port });
  }
  validateArgs({ window, by, period });
  if (watch) {
    // 实时监控模式：长驻循环（测试通过 runWatch 单步驱动，此处仅打印初始状态并进入循环）
    return runWatchCli({ dir, format, model, cwd, since, until, window, interval });
  }
  const files = await readSessionFiles(dir);
  const filtered = filterFiles(files, { model, cwd, since, until });
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

/** 参数合法性校验（IO 之前） */
function validateArgs(args: { window: WindowName; by?: GroupBy; period?: Period }): void {
  if (args.period !== undefined && args.window !== "totals") {
    throw new Error(`--period 汇总仅支持 totals 窗口（当前 ${args.window}）`);
  }
  if (args.by !== undefined && args.window !== "totals") {
    throw new Error(`--by 分组仅支持 totals 窗口（当前 ${args.window}）`);
  }
  if (args.period !== undefined && args.by !== undefined) {
    throw new Error(`--period 与 --by 不能同时使用（当前 period=${args.period}, by=${args.by}）`);
  }
}

/**
 * --watch 实时监控：增量读取器单步 + 刷新回调。
 * 测试直接驱动（传入自建 reader 与回调）；CLI 用 runWatchCli 提供长驻循环。
 * 返回刷新次数（供测试断言）。
 */
export async function runWatch(
  reader: IncrementalReader,
  totals: Totals,
  onRefresh: (totals: Totals, changed: boolean) => void,
  intervalMs: number,
  iterations = Infinity,
): Promise<number> {
  let refreshes = 0;
  for (let i = 0; i < iterations; i++) {
    const changed = await applyIncrements(reader, totals);
    if (changed || i === 0) {
      onRefresh(totals, changed);
      refreshes++;
    }
    if (i < iterations - 1) await sleep(intervalMs);
  }
  return refreshes;
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

/** CLI --watch 长驻模式：初始输出当前 totals，之后每间隔刷新 */
async function runWatchCli(args: {
  dir: string;
  format: FormatName;
  model?: string;
  cwd?: string;
  since?: string;
  until?: string;
  window: WindowName;
  interval: number;
}): Promise<string> {
  // 实时模式仅支持 totals 表格（长驻刷新语义；结构化输出无意义）
  if (args.format !== "table" || args.window !== "totals") {
    throw new Error(`--watch 仅支持 totals 窗口 + table 格式（当前 window=${args.window}, format=${args.format}）`);
  }
  // --watch 不支持筛选组合（实时增量边界是全量新行，筛选语义不清）
  if (args.model !== undefined || args.cwd !== undefined || args.since !== undefined || args.until !== undefined) {
    throw new Error("--watch 不支持 --model/--cwd/--since/--until 组合（实时模式统计全部会话增量）");
  }
  const reader = new IncrementalReader(args.dir);
  const totals: Totals = emptyTotals();
  let last = "";
  await runWatch(reader, totals, (t) => {
    const out = renderTotalsTable(t);
    if (out !== last) {
      process.stdout.write("\u001b[2J\u001b[H" + out); // 清屏刷新
      last = out;
    }
  }, args.interval);
  return last; // 循环仅在迭代耗尽时返回（CLI 直接运行时为长驻）
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
