#!/usr/bin/env node
/**
 * CLI 入口：token-analyzer [totals|sessions|requests] --dir <path> [--format <table|json|csv>]
 * 默认窗口 totals（issue 01 行为），默认格式 table，默认数据目录 ~/.pi/agent/sessions/。
 */
import { homedir } from "node:os";
import { readFileSync, realpathSync } from "node:fs";
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
import { Database, resolveDbPathFromEnv } from "./db.ts";
import { withDirDb, queryTotals, queryGroups, queryPeriod, querySessions, queryRequests, rollupAndPrune } from "./db-aggregation.ts";
import { collectPiJsonlFiles, resolvePiSessionRoot } from "./pi-discovery.ts";
import { syncPiUsage } from "./pi-sync.ts";
import { IncrementalReader, applyIncrements } from "./watch.ts";
import { emptyTotals, type GroupBy, type Period, type Totals } from "./aggregate.ts";
import { renderTotalsTable, renderSessionTable, renderRequestTable, renderGroupTable, renderPeriodTable } from "./render.ts";
import { serializeJson, serializeCsv, serializeGroupJson, serializeGroupCsv, serializePeriodJson, serializePeriodCsv } from "./serialize.ts";
import { startWebServer } from "./server.ts";
import { runOpencodeSync, runOpencodeExport } from "./opencode/cli.ts";

const DEFAULT_DIR = join(homedir(), ".pi", "agent", "sessions");

export type WindowName = "totals" | "sessions" | "requests";
export type FormatName = "table" | "json" | "csv";

export interface OpencodeSyncCliOpts {
  auth?: string;
  workspace?: string;
  full: boolean;
  limit?: number;
  dataDir: string;
}
export interface OpencodeExportCliOpts {
  format: "json" | "csv";
  output?: string;
  month?: string;
  dataDir: string;
}

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
  /** -h/--help：显示帮助 */
  help: boolean;
  /** -v/--version：显示版本 */
  version: boolean;
  /** sync：同步 pi 会话到 DB */
  sync: boolean;
  /** prune：剪枝旧数据 */
  prune: boolean;
  /** --db <path>：数据库路径 */
  dbPath?: string;
  /** --full：全量重扫 */
  full: boolean;
  /** --days <n>：prune 天数 */
  days: number;
  /** opencode 子命令 */
  opencode?: "sync" | "export";
  opencodeSync?: OpencodeSyncCliOpts;
  opencodeExport?: OpencodeExportCliOpts;
}

export function parseArgs(argv: string[]): CliArgs {
  // opencode 子命令优先处理（要求位于首位）
  if (argv.length > 0 && argv[0] === "opencode") {
    return parseOpencodeArgs(argv);
  }

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
  let help = false;
  let version = false;
  let sync = false;
  let prune = false;
  let dbPath: string | undefined;
  let full = false;
  let days = 30;
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === "-h" || a === "--help") {
      help = true;
    } else if (a === "-v" || a === "--version") {
      version = true;
    } else if (a === "--dir" && argv[i + 1]) {
      dir = argv[i + 1];
      i++;
    } else if (a === "--dir") {
      throw new Error(`缺少参数: --dir 需要路径`);
    } else if (a === "--port" && argv[i + 1]) {
      const n = Number(argv[i + 1]);
      if (!Number.isInteger(n) || n < 0 || n > 65535) {
        throw new Error(`无效端口: ${argv[i + 1]}（需为 0-65535 的整数）`);
      }
      port = n;
      i++;
    } else if (a === "--port") {
      throw new Error(`缺少参数: --port 需要端口号`);
    } else if (a === "--host" && argv[i + 1]) {
      host = argv[i + 1];
      i++;
    } else if (a === "--host") {
      throw new Error(`缺少参数: --host 需要地址`);
    } else if (a === "--format" && argv[i + 1]) {
      const f = argv[i + 1];
      if (f === "json" || f === "csv" || f === "table") {
        format = f;
      } else {
        throw new Error(`未知格式: ${f}（支持 table/json/csv）`);
      }
      i++;
    } else if (a === "--format") {
      throw new Error(`缺少参数: --format 需要值`);
    } else if (a === "--model" && argv[i + 1]) {
      model = argv[i + 1];
      i++;
    } else if (a === "--model") {
      throw new Error(`缺少参数: --model 需要模型名`);
    } else if (a === "--cwd" && argv[i + 1]) {
      cwd = argv[i + 1];
      i++;
    } else if (a === "--cwd") {
      throw new Error(`缺少参数: --cwd 需要路径`);
    } else if (a === "--since" && argv[i + 1]) {
      since = argv[i + 1];
      i++;
    } else if (a === "--since") {
      throw new Error(`缺少参数: --since 需要时间`);
    } else if (a === "--until" && argv[i + 1]) {
      until = argv[i + 1];
      i++;
    } else if (a === "--until") {
      throw new Error(`缺少参数: --until 需要时间`);
    } else if (a === "--period" && argv[i + 1]) {
      const p = argv[i + 1];
      if (p === "day" || p === "week" || p === "month") {
        period = p;
      } else {
        throw new Error(`未知周期: ${p}（支持 day/week/month）`);
      }
      i++;
    } else if (a === "--period") {
      throw new Error(`缺少参数: --period 需要值`);
    } else if (a === "--watch") {
      watch = true;
    } else if (a === "--interval" && argv[i + 1]) {
      const ms = Number(argv[i + 1]);
      if (!Number.isFinite(ms) || ms <= 0) {
        throw new Error(`无效间隔: ${argv[i + 1]}（需为正数毫秒）`);
      }
      interval = ms;
      i++;
    } else if (a === "--interval") {
      throw new Error(`缺少参数: --interval 需要毫秒`);
    } else if (a === "--by" && argv[i + 1]) {
      const b = argv[i + 1];
      if (b === "model" || b === "cwd" || b === "model,cwd") {
        by = b;
      } else {
        throw new Error(`未知分组: ${b}（支持 model/cwd/model,cwd）`);
      }
      i++;
    } else if (a === "--by") {
      throw new Error(`缺少参数: --by 需要值`);
    } else if (a === "totals" || a === "sessions" || a === "requests") {
      window = a;
    } else if (a === "sync") {
      sync = true;
    } else if (a === "prune") {
      prune = true;
    } else if (a === "--db" && argv[i + 1]) {
      dbPath = argv[i + 1];
      i++;
    } else if (a === "--db") {
      throw new Error(`缺少参数: --db 需要路径`);
    } else if (a === "--full") {
      full = true;
    } else if (a === "--days" && argv[i + 1]) {
      const n = Number(argv[i + 1]);
      if (!Number.isInteger(n) || n < 0) {
        throw new Error(`无效天数: ${argv[i + 1]}（需为非负整数）`);
      }
      days = n;
      i++;
    } else if (a === "--days") {
      throw new Error(`缺少参数: --days 需要天数`);
    } else if (a === "serve") {
      serve = true;
    } else if (a.startsWith("-")) {
      throw new Error(`未知参数: ${a}（可用 -h 查看帮助）`);
    } else {
      throw new Error(`未知命令: ${a}（支持 totals/sessions/requests/serve，可用 -h 查看帮助）`);
    }
  }
  if (help || version) {
    return { window, dir, format, model, cwd, by, since, until, period, watch, interval, serve, port, host, help, version, sync, prune, dbPath, full, days };
  }
  if (sync || prune) {
    validateSyncMode(argv);
  }
  if (serve) {
    validateServeMode(argv);
  }
  return { window, dir, format, model, cwd, by, since, until, period, watch, interval, serve, port, host, help, version, sync, prune, dbPath, full, days };
}

/** sync/prune 模式参数校验：窗口/分组/汇总/watch 参数无意义，拒绝 */
function validateSyncMode(argv: string[]): void {
  const FORBIDDEN = [
    "totals",
    "sessions",
    "requests",
    "serve",
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
    throw new Error(`sync/prune 模式不支持参数（收到 ${hit}）`);
  }
}

function parseOpencodeArgs(argv: string[]): CliArgs {
  // argv[0] === "opencode"
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
  let help = false;
  let version = false;

  // 全局 help/version 预检
  for (const a of argv) {
    if (a === "-h" || a === "--help") help = true;
    if (a === "-v" || a === "--version") version = true;
  }
  if (help || version) {
    // 帮助模式仍返回基础结构，opencode 子类型按需解析
    const sub = argv[1] as "sync" | "export" | undefined;
    if (sub === "sync" || sub === "export") {
      return {
        window: "totals",
        dir,
        format,
        model,
        cwd,
        by,
        since,
        until,
        period,
        watch,
        interval,
        serve,
        port,
        host,
        help,
        version,
        sync: false,
        prune: false,
        dbPath: undefined,
        full: false,
        days: 30,
        opencode: sub,
        opencodeSync: sub === "sync" ? { auth: undefined, workspace: undefined, full: false, limit: undefined, dataDir: "data/opencode" } : undefined,
        opencodeExport: sub === "export" ? { format: "json", output: undefined, month: undefined, dataDir: "data/opencode" } : undefined,
      };
    }
    return { window: "totals", dir, format, model, cwd, by, since, until, period, watch, interval, serve, port, host, help, version, sync: false, prune: false, dbPath: undefined, full: false, days: 30 };
  }

  const sub = argv[1];
  if (!sub) {
    throw new Error(`缺少 opencode 子命令（支持 sync/export）`);
  }
  if (sub !== "sync" && sub !== "export") {
    throw new Error(`未知 opencode 子命令: ${sub}（支持 sync/export）`);
  }

  if (sub === "sync") {
    let auth: string | undefined;
    let workspace: string | undefined;
    let full = false;
    let limit: number | undefined;
    let dataDir = "data/opencode";
    for (let i = 2; i < argv.length; i++) {
      const b = argv[i];
      if (b === "--auth" && argv[i + 1]) {
        auth = argv[i + 1];
        i++;
      } else if (b === "--auth") {
        throw new Error(`缺少参数: --auth 需要凭证`);
      } else if (b === "--workspace" && argv[i + 1]) {
        workspace = argv[i + 1];
        i++;
      } else if (b === "--workspace") {
        throw new Error(`缺少参数: --workspace 需要工作区ID`);
      } else if (b === "--full") {
        full = true;
      } else if (b === "--limit" && argv[i + 1]) {
        const n = Number(argv[i + 1]);
        if (!Number.isInteger(n) || n <= 0) {
          throw new Error(`无效 limit: ${argv[i + 1]}（需为正整数）`);
        }
        limit = n;
        i++;
      } else if (b === "--limit") {
        throw new Error(`缺少参数: --limit 需要页数`);
      } else if ((b === "--data-dir" || b === "--dataDir") && argv[i + 1]) {
        dataDir = argv[i + 1];
        i++;
      } else if (b === "--data-dir" || b === "--dataDir") {
        throw new Error(`缺少参数: --data-dir 需要目录`);
      } else if (b === "-h" || b === "--help" || b === "-v" || b === "--version") {
        // 已在顶部处理，此处忽略
        continue;
      } else if (b.startsWith("-")) {
        throw new Error(`未知参数: ${b}（可用 -h 查看帮助）`);
      } else {
        throw new Error(`未知参数: ${b}（可用 -h 查看帮助）`);
      }
    }
    return {
      window: "totals",
      dir,
      format,
      model,
      cwd,
      by,
      since,
      until,
      period,
      watch,
      interval,
      serve,
      port,
      host,
      help,
      version,
      sync: false,
      prune: false,
      dbPath: undefined,
      full: false,
      days: 30,
      opencode: "sync",
      opencodeSync: { auth, workspace, full, limit, dataDir },
    };
  } else {
    // export
    let expFormat: "json" | "csv" = "json";
    let output: string | undefined;
    let month: string | undefined;
    let dataDir = "data/opencode";
    for (let i = 2; i < argv.length; i++) {
      const b = argv[i];
      if (b === "--format" && argv[i + 1]) {
        const f = argv[i + 1];
        if (f !== "json" && f !== "csv") {
          throw new Error(`未知格式: ${f}（支持 json/csv）`);
        }
        expFormat = f as "json" | "csv";
        i++;
      } else if (b === "--format") {
        throw new Error(`缺少参数: --format 需要值`);
      } else if (b === "--output" && argv[i + 1]) {
        output = argv[i + 1];
        i++;
      } else if (b === "--output") {
        throw new Error(`缺少参数: --output 需要路径`);
      } else if (b === "--month" && argv[i + 1]) {
        const m = argv[i + 1];
        if (!/^\d{4}-\d{2}$/.test(m) || Number(m.slice(5)) < 1 || Number(m.slice(5)) > 12) {
          throw new Error(`无效月份: ${m}（需为 YYYY-MM）`);
        }
        month = m;
        i++;
      } else if (b === "--month") {
        throw new Error(`缺少参数: --month 需要值`);
      } else if ((b === "--data-dir" || b === "--dataDir") && argv[i + 1]) {
        dataDir = argv[i + 1];
        i++;
      } else if (b === "--data-dir" || b === "--dataDir") {
        throw new Error(`缺少参数: --data-dir 需要目录`);
      } else if (b === "-h" || b === "--help" || b === "-v" || b === "--version") {
        continue;
      } else if (b.startsWith("-")) {
        throw new Error(`未知参数: ${b}（可用 -h 查看帮助）`);
      } else {
        throw new Error(`未知参数: ${b}（可用 -h 查看帮助）`);
      }
    }
    return {
      window: "totals",
      dir,
      format,
      model,
      cwd,
      by,
      since,
      until,
      period,
      watch,
      interval,
      serve,
      port,
      host,
      help,
      version,
      sync: false,
      prune: false,
      dbPath: undefined,
      full: false,
      days: 30,
      opencode: "export",
      opencodeExport: { format: expFormat, output, month, dataDir },
    };
  }
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

function getVersion(): string {
  try {
    const pkg = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8")) as { version?: string };
    return pkg.version ?? "unknown";
  } catch {
    return "unknown";
  }
}

const HELP_TEXT = `用法: token-analyzer [totals|sessions|requests] --dir <path> [选项]

窗口（位置参数，默认 totals）:
  totals      总消耗量
  sessions    会话级（每会话一行）
  requests    单请求级（逐 assistant 消息）

选项:
  --dir <path>              数据目录（默认 ~/.pi/agent/sessions/）
  --format <table|json|csv> 输出格式（默认 table）
  --model <id>              只统计指定模型
  --cwd <path>              只统计指定项目
  --since <时间>            只统计会话时间戳 ≥ 该值的会话
  --until <时间>            只统计会话时间戳 ≤ 该值的会话
  --by <model|cwd|model,cwd> 仅 totals 窗口：按维度分组
  --period <day|week|month>   仅 totals 窗口：按周期汇总
  --watch [--interval <ms>] 实时监控（默认 1000ms）
  serve [--port <n>] [--host <h>] [--dir <path>] [--db <path>] 启动 Web 面板（默认 127.0.0.1:50080）
  sync [--dir <path>] [--db <path>] [--full] 同步 pi 会话到 DB（四载体直切）
  prune [--db <path>] [--days <n>] 剪枝旧数据（默认 30 天）
  -h, --help                显示帮助
  -v, --version             显示版本

OpenCode 数据同步（外部对比基准）:
  opencode sync [--auth <cookie>] [--workspace <id>] [--full] [--limit <n>] [--data-dir <dir>]
                            同步 OpenCode 云端用量到本地 data/opencode/
                            凭证优先级: --auth > OPENCODE_AUTH env > .env
                            --full 全量同步（忽略增量游标），--limit 限制最大页数
  opencode export [--format <json|csv>] [--output <file>] [--month <YYYY-MM>] [--data-dir <dir>]
                            导出本地已同步的 OpenCode 历史
                            --format 输出格式（默认 json），--month 按月过滤
`;

/** serve 子命令：启动 Web 服务器、打印访问 URL、Ctrl+C 优雅退出（长驻） */
async function runServeCli(args: { dir: string; host: string; port: number; dbPath?: string }): Promise<string> {
  const dbPath = resolveDbPathFromEnv(args.dbPath);
  const server = await startWebServer({ dir: args.dir, host: args.host, port: args.port });
  // SIGINT handler 先于 URL 打印注册：URL 打印即代表优雅退出已就绪（消除 kill 竞态）
  const exited = new Promise<void>((resolve) => {
    process.once("SIGINT", () => {
      server.close().then(resolve);
    });
  });
  process.stdout.write(`Token Analyzer WebUI: ${server.url}\n`);
  process.stdout.write(`DB: ${dbPath}\n`);
  await exited;
  return "";
}
/** 运行分析，返回输出文本（供 CLI 打印与测试断言）；目录只扫描一次，派生三窗口 */
export async function runCli(argv: string[]): Promise<string> {
  const parsed = parseArgs(argv);
  const { window, dir, format, model, cwd, by, since, until, period, watch, interval, serve, port, host, help, version } = parsed;
  if (help) return HELP_TEXT;
  if (version) return getVersion() + "\n";
  // opencode 子命令分发（优先于其他窗口）
  if (parsed.opencode === "sync" && parsed.opencodeSync) {
    const s = parsed.opencodeSync;
    return runOpencodeSync({ auth: s.auth, workspace: s.workspace, full: s.full, limit: s.limit, dataDir: s.dataDir });
  }
  if (parsed.opencode === "export" && parsed.opencodeExport) {
    const e = parsed.opencodeExport;
    return runOpencodeExport({ format: e.format, output: e.output, month: e.month, dataDir: e.dataDir });
  }
  if (serve) {
    return runServeCli({ dir, host, port, dbPath: parsed.dbPath });
  }
  if (parsed.sync) {
    const dbPath = resolveDbPathFromEnv(parsed.dbPath);
    const db = await Database.getInstance(dbPath);
    try {
      const root = resolvePiSessionRoot({ envDb: process.env.PI_CODING_AGENT_SESSION_DIR, defaultRoot: dir, piConfig: undefined });
      const files = collectPiJsonlFiles(root.root, root.layout);
      // 回退递归收集（兼容测试 fixture 扁平文件）
      const fallback = files.length === 0 ? (await import("./session-data.ts")).collectJsonlFiles(dir) : [];
      const allFiles = files.length > 0 ? files : fallback;
      const res = await syncPiUsage(db, allFiles, { full: parsed.full });
      return `同步完成: 新增 ${res.imported} 条, 跳过 ${res.skipped} 条, DB: ${dbPath}\n`;
    } finally { await db.close(); }
  }
  if (parsed.prune) {
    const dbPath = resolveDbPathFromEnv(parsed.dbPath);
    const db = await Database.getInstance(dbPath);
    try {
      const res = rollupAndPrune(db, parsed.days);
      return `清理完成: 聚合 ${res.rolled} 组, DB: ${dbPath}\n`;
    } finally { await db.close(); }
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
function validateArgs(args: { window: WindowName; by?: GroupBy; period?: Period; opencode?: string }): void {
  if (args.opencode) {
    if (args.by !== undefined) throw new Error(`opencode 模式不支持 --by`);
    if (args.period !== undefined) throw new Error(`opencode 模式不支持 --period`);
    return;
  }
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
