import { test } from "node:test";
import assert from "node:assert/strict";
import { parseArgs, runCli } from "../src/cli.ts";

test("parseArgs -h 标记 help", () => {
  const args = parseArgs(["-h"]);
  assert.equal((args as unknown as { help: boolean }).help, true);
});

test("parseArgs --help 标记 help", () => {
  const args = parseArgs(["--help"]);
  assert.equal((args as unknown as { help: boolean }).help, true);
});

test("runCli -h 返回帮助文本而非 totals", async () => {
  const out = await runCli(["-h"]);
  assert.match(out, /用法|token-analyzer/);
  assert.doesNotMatch(out, /请求数/);
});

test("未知命令 1 应抛错", () => {
  assert.throws(() => parseArgs(["1"]), /未知命令/);
});

test("未知参数 --unknown 应抛错", () => {
  assert.throws(() => parseArgs(["--unknown"]), /未知参数/);
});
