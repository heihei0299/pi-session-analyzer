/**
 * Ticket 01 — serve 子命令与最小 HTTP 服务器。
 * Seams 全部在 CLI 参数层与 HTTP 公共接口上：parseArgs 校验、GET / 静态页、404 错误体、EADDRINUSE、SIGINT。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { homedir } from "node:os";
import { join } from "node:path";
import { spawn, type ChildProcess } from "node:child_process";
import { fileURLToPath } from "node:url";
import type { Readable } from "node:stream";
import { parseArgs } from "../src/cli.ts";
import { startWebServer } from "../src/server.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

const REPO_ROOT = fileURLToPath(new URL("..", import.meta.url));
const DEFAULT_DIR = join(homedir(), ".pi", "agent", "sessions");

test("S1a parseArgs serve 默认值：serve=true、port=50080、host=127.0.0.1、dir=默认目录", () => {
  const args = parseArgs(["serve"]);
  assert.equal(args.serve, true);
  assert.equal(args.port, 50080);
  assert.equal(args.host, "127.0.0.1");
  assert.equal(args.dir, DEFAULT_DIR);
});

test("S1b parseArgs serve --port/--host/--dir 覆盖生效", () => {
  const args = parseArgs(["serve", "--port", "8080", "--host", "0.0.0.0", "--dir", "/tmp/x"]);
  assert.equal(args.serve, true);
  assert.equal(args.port, 8080);
  assert.equal(args.host, "0.0.0.0");
  assert.equal(args.dir, "/tmp/x");
});

test("S1c serve 模式拒绝窗口位置参数与其它 CLI 参数", () => {
  for (const argv of [
    ["serve", "totals"],
    ["serve", "sessions"],
    ["serve", "requests"],
    ["serve", "--format", "json"],
    ["serve", "--by", "model"],
    ["serve", "--period", "day"],
    ["serve", "--watch"],
    ["serve", "--interval", "500"],
    ["serve", "--interval", "1000"],
    ["serve", "--model", "m1"],
    ["serve", "--cwd", "/x"],
    ["serve", "--since", "2026-01-01"],
    ["serve", "--until", "2026-01-02"],
  ]) {
    assert.throws(() => parseArgs(argv), /serve 模式仅支持 --port\/--host\/--dir/, `应拒绝: ${argv.join(" ")}`);
  }
  // 非 serve 模式的既有解析不受影响
  assert.equal(parseArgs(["totals", "--dir", "/tmp/x"]).window, "totals");
});

test("S1d 非法端口报错：非数字 / 越界", () => {
  assert.throws(() => parseArgs(["serve", "--port", "abc"]), /无效端口/);
  assert.throws(() => parseArgs(["serve", "--port", "70000"]), /无效端口/);
  assert.throws(() => parseArgs(["serve", "--port", "-1"]), /无效端口/);
});

test("S2 GET / 与 /index.html：200 text/html 且含标题", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const res = await fetch(server.url);
    assert.equal(res.status, 200);
    assert.match(res.headers.get("content-type") ?? "", /^text\/html; charset=utf-8$/);
    const body = await res.text();
    assert.match(body, /Token Analyzer WebUI/);

    const res2 = await fetch(new URL("/index.html", server.url));
    assert.equal(res2.status, 200);
    assert.match(res2.headers.get("content-type") ?? "", /^text\/html; charset=utf-8$/);
    assert.match(await res2.text(), /Token Analyzer WebUI/);
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

test("S3 未知路径 → 404 统一 JSON 错误体 { error, detail }", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const res = await fetch(new URL("/bogus", server.url));
    assert.equal(res.status, 404);
    assert.match(res.headers.get("content-type") ?? "", /^application\/json/);
    const body = (await res.json()) as { error: string; detail: string };
    assert.equal(typeof body.error, "string");
    assert.ok(body.error.length > 0);
    assert.equal(typeof body.detail, "string");
    assert.ok(body.detail.length > 0);
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

test("S4 端口被占用 → 友好错误（含「已被占用，可用 --port 更换」）", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  const first = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    await assert.rejects(
      startWebServer({ dir, host: "127.0.0.1", port: first.port }),
      /已被占用，可用 --port 更换/,
    );
  } finally {
    await first.close();
    removeFixture(dir);
  }
});

/** 等待子进程 stdout 出现匹配行（带超时） */
function waitForStdout(child: ChildProcess, pattern: RegExp, timeoutMs: number): Promise<void> {
  const stream = child.stdout as Readable | null;
  if (stream === null) return Promise.reject(new Error("child.stdout is null"));
  return new Promise((resolve, reject) => {
    let buf = "";
    const timer = setTimeout(() => {
      cleanup();
      reject(new Error(`等待输出超时（${timeoutMs}ms）：${pattern}`));
    }, timeoutMs);
    const onData = (chunk: Buffer): void => {
      buf += chunk.toString("utf8");
      if (pattern.test(buf)) {
        cleanup();
        resolve();
      }
    };
    const onExit = (): void => {
      cleanup();
      reject(new Error(`进程在匹配 ${pattern} 前退出`));
    };
    const cleanup = (): void => {
      clearTimeout(timer);
      stream.off("data", onData);
      child.off("exit", onExit);
    };
    stream.on("data", onData);
    child.on("exit", onExit);
  });
}

test("S5 子进程 serve：打印访问 URL，SIGINT 优雅退出（退出码 0）", { timeout: 20000 }, async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  const child = spawn(
    process.execPath,
    ["src/cli.ts", "serve", "--dir", dir, "--port", "0"],
    { cwd: REPO_ROOT, stdio: ["ignore", "pipe", "pipe"] },
  );
  try {
    await waitForStdout(child, /http:\/\/127\.0\.0\.1:\d+\//, 10000);
    child.kill("SIGINT");
    const code = await new Promise<number | null>((resolve) => {
      child.on("exit", (c) => resolve(c));
    });
    assert.equal(code, 0, "SIGINT 后应优雅退出（退出码 0）");
  } finally {
    if (child.exitCode === null && child.signalCode === null) child.kill("SIGKILL");
    removeFixture(dir);
  }
});
