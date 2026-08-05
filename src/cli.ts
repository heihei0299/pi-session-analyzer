/**
 * CLI 入口：token-analyzer --dir <path>
 * 默认数据目录 ~/.pi/agent/sessions/；--dir 可注入 fixture 目录（测试 seam）。
 */
import { homedir } from "node:os";
import { realpathSync } from "node:fs";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import { analyzeSessionDir } from "./analyze.ts";
import { renderTotalsTable } from "./render.ts";

const DEFAULT_DIR = join(homedir(), ".pi", "agent", "sessions");
export function parseArgs(argv: string[]): { dir: string } {
  let dir = DEFAULT_DIR;
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] === "--dir" && argv[i + 1]) {
      dir = argv[i + 1];
      i++;
    }
  }
  return { dir };
}

/** 运行分析，返回表格文本（供 CLI 打印与测试断言） */
export async function runCli(argv: string[]): Promise<string> {
  const { dir } = parseArgs(argv);
  const totals = await analyzeSessionDir(dir);
  return renderTotalsTable(totals);
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
