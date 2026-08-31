import { test, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync, readFileSync, writeFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { parseArgs, runCli } from "../src/cli.ts";
import { resolveCredentials, loadCredentials } from "../src/opencode/credentials.ts";
import { runOpencodeSync, runOpencodeExport } from "../src/opencode/cli.ts";
import { OpenCodeStorage } from "../src/opencode/storage.ts";
import type { OpenCodeUsageRecord } from "../src/opencode/types.ts";

function tmpDir(): string {
  return mkdtempSync(join(tmpdir(), "opencode-cli-test-"));
}
function cleanup(dir: string) {
  rmSync(dir, { recursive: true, force: true });
}
function rec(overrides: Partial<OpenCodeUsageRecord> & { id: string; timeCreated: string }): OpenCodeUsageRecord {
  return {
    workspaceID: "wrk_test",
    timeUpdated: overrides.timeCreated,
    timeDeleted: null,
    model: "x-preview-f-free",
    provider: "inf.oa-compat",
    inputTokens: 100,
    outputTokens: 50,
    reasoningTokens: 10,
    cacheReadTokens: 5,
    cacheWrite5mTokens: null,
    cacheWrite1hTokens: null,
    cost: 0.001,
    keyID: "key_1",
    sessionID: "sess_001",
    enrichment: null,
    ...overrides,
  } as OpenCodeUsageRecord;
}

// Preserve env
let savedEnv: Record<string, string | undefined> = {};
function saveEnv() {
  savedEnv = { ...process.env };
}
function restoreEnv() {
  // delete added keys
  for (const k of Object.keys(process.env)) {
    if (!(k in savedEnv)) delete process.env[k];
  }
  for (const [k, v] of Object.entries(savedEnv)) {
    if (v === undefined) delete process.env[k];
    else process.env[k] = v;
  }
}

// ---------- T1 parseArgs opencode sync ----------
test("T1 parseArgs opencode sync 完整参数解析", () => {
  const a = parseArgs(["opencode", "sync", "--auth", "tok123", "--workspace", "wrk_1", "--full", "--limit", "5", "--data-dir", "my/data"]);
  assert.equal(a.opencode, "sync");
  assert.equal(a.opencodeSync?.auth, "tok123");
  assert.equal(a.opencodeSync?.workspace, "wrk_1");
  assert.equal(a.opencodeSync?.full, true);
  assert.equal(a.opencodeSync?.limit, 5);
  assert.equal(a.opencodeSync?.dataDir, "my/data");
});

test("T1 parseArgs opencode sync 默认值", () => {
  const a = parseArgs(["opencode", "sync"]);
  assert.equal(a.opencode, "sync");
  assert.equal(a.opencodeSync?.full, false);
  assert.equal(a.opencodeSync?.limit, undefined);
  assert.equal(a.opencodeSync?.dataDir, "data/opencode");
  assert.equal(a.opencodeSync?.auth, undefined);
});

test("T1 parseArgs opencode sync --limit 非整数抛错", () => {
  assert.throws(() => parseArgs(["opencode", "sync", "--limit", "abc"]), /无效 limit/);
  assert.throws(() => parseArgs(["opencode", "sync", "--limit", "0"]), /无效 limit/);
});

test("T1 parseArgs opencode sync 缺少 --auth 值抛错", () => {
  assert.throws(() => parseArgs(["opencode", "sync", "--auth"]), /缺少参数/);
});

test("T1 parseArgs opencode sync 未知参数抛错", () => {
  assert.throws(() => parseArgs(["opencode", "sync", "--unknown"]), /未知参数/);
});

test("T1 parseArgs opencode export 完整参数解析", () => {
  const a = parseArgs(["opencode", "export", "--format", "csv", "--output", "out.csv", "--month", "2026-08", "--data-dir", "d2"]);
  assert.equal(a.opencode, "export");
  assert.equal(a.opencodeExport?.format, "csv");
  assert.equal(a.opencodeExport?.output, "out.csv");
  assert.equal(a.opencodeExport?.month, "2026-08");
  assert.equal(a.opencodeExport?.dataDir, "d2");
});

test("T1 parseArgs opencode export 默认 format json", () => {
  const a = parseArgs(["opencode", "export"]);
  assert.equal(a.opencodeExport?.format, "json");
  assert.equal(a.opencodeExport?.month, undefined);
});

test("T1 parseArgs opencode export 非法 format 抛错", () => {
  assert.throws(() => parseArgs(["opencode", "export", "--format", "xml"]), /未知格式/);
});

test("T1 parseArgs opencode export 非法 month 抛错", () => {
  assert.throws(() => parseArgs(["opencode", "export", "--month", "bad"]), /无效月份/);
  assert.throws(() => parseArgs(["opencode", "export", "--month", "2026-13"]), /无效月份/);
});

test("T1 parseArgs opencode 缺少子命令抛错", () => {
  assert.throws(() => parseArgs(["opencode"]), /缺少 opencode 子命令/);
});

test("T1 parseArgs opencode 未知子命令抛错", () => {
  assert.throws(() => parseArgs(["opencode", "foo"]), /未知 opencode 子命令/);
});

test("T1 resolveCredentials 优先级 CLI > env", () => {
  const r1 = resolveCredentials({ auth: "cli", workspace: "cliW" }, { OPENCODE_AUTH: "env", OPENCODE_WORKSPACE_ID: "envW" });
  assert.equal(r1.auth, "cli");
  assert.equal(r1.workspace, "cliW");
  const r2 = resolveCredentials({}, { OPENCODE_AUTH: "env", OPENCODE_WORKSPACE_ID: "envW" });
  assert.equal(r2.auth, "env");
  assert.equal(r2.workspace, "envW");
  const r3 = resolveCredentials({}, {});
  assert.equal(r3.auth, undefined);
});

// ---------- T2 loadCredentials ----------
test("T2 loadCredentials CLI 优先于 env", () => {
  saveEnv();
  try {
    process.env.OPENCODE_AUTH = "env_auth";
    process.env.OPENCODE_WORKSPACE_ID = "env_wrk";
    const c = loadCredentials({ auth: "cli_auth", workspace: "cli_wrk" });
    assert.equal(c.auth, "cli_auth");
    assert.equal(c.workspace, "cli_wrk");
  } finally { restoreEnv(); }
});

test("T2 loadCredentials env 优先于 .env", () => {
  saveEnv();
  const dir = tmpDir();
  try {
    writeFileSync(join(dir, ".env"), "OPENCODE_AUTH=file_auth\nOPENCODE_WORKSPACE_ID=file_wrk\n");
    process.env.OPENCODE_AUTH = "env_auth";
    const c = loadCredentials({ dataDir: dir });
    assert.equal(c.auth, "env_auth");
  } finally { cleanup(dir); restoreEnv(); }
});

test("T2 loadCredentials 读取 .env 文件", () => {
  saveEnv();
  const dir = tmpDir();
  try {
    delete process.env.OPENCODE_AUTH;
    delete process.env.OPENCODE_WORKSPACE_ID;
    writeFileSync(join(dir, ".env"), "OPENCODE_AUTH=\"file_auth_quoted\"\nOPENCODE_WORKSPACE_ID=wrk_file\n# comment\n");
    const c = loadCredentials({ dataDir: dir });
    assert.equal(c.auth, "file_auth_quoted");
    assert.equal(c.workspace, "wrk_file");
    assert.equal(c.dataDir, dir);
  } finally { cleanup(dir); restoreEnv(); }
});

test("T2 loadCredentials .env 去引号与注释", () => {
  saveEnv();
  const dir = tmpDir();
  try {
    delete process.env.OPENCODE_AUTH;
    writeFileSync(join(dir, ".env"), "OPENCODE_AUTH='single_quoted' # inline comment?\n");
    // 单引号包裹的值应去掉引号，行内注释在引号外不应误截
    const c = loadCredentials({ dataDir: dir });
    assert.equal(c.auth, "single_quoted");
  } finally { cleanup(dir); restoreEnv(); }
});

test("T2 loadCredentials 缺少认证抛友好错误", () => {
  saveEnv();
  const dir = tmpDir();
  try {
    delete process.env.OPENCODE_AUTH;
    // 确保 cwd 下无 .env 干扰：使用隔离 dataDir 且 cwd 无相关 env
    // 将 process.cwd 的 .env 临时规避：我们传一个空 dataDir 并清理 env
    // 此时 loadCredentials 应抛
    assert.throws(() => loadCredentials({ dataDir: dir }), /缺少认证信息.*OPENCODE_AUTH/);
  } finally { cleanup(dir); restoreEnv(); }
});

test("T2 loadCredentials dataDir 默认值", () => {
  saveEnv();
  try {
    process.env.OPENCODE_AUTH = "tok";
    delete process.env.OPENCODE_WORKSPACE_ID;
    const c = loadCredentials({});
    assert.equal(c.dataDir, "data/opencode");
  } finally { restoreEnv(); }
});

// ---------- T3 runOpencodeSync ----------
test("T3 runOpencodeSync 成功返回进度文本（mock storage/client）", async () => {
  saveEnv();
  const dir = tmpDir();
  try {
    process.env.OPENCODE_AUTH = "tok_sync";
    const storage = new OpenCodeStorage(dir);
    const mockClient = {
      async getWorkspaces() { return [{ id: "wrk_test", name: "test" }]; },
      async getUsageInfo(_wid: string, page: number) {
        if (page === 0) return [rec({ id: "usg_1", timeCreated: "2026-08-20T00:00:00.000Z" })];
        return [];
      },
    };
    const out = await runOpencodeSync({ workspace: "wrk_test", dataDir: dir, storage, client: mockClient as unknown as never });
    assert.match(out, /同步完成/);
    assert.match(out, /抓取 \d+ 页/);
    assert.match(out, /新增 \d+ 条/);
    assert.match(out, /耗时 \d+ms/);
  } finally { cleanup(dir); restoreEnv(); }
});

test("T3 runOpencodeSync 支持 --full 与 --limit 透传", async () => {
  saveEnv();
  const dir = tmpDir();
  try {
    process.env.OPENCODE_AUTH = "tok";
    let capturedLimit: number | undefined;
    let capturedFull: boolean | undefined;
    const storage = new OpenCodeStorage(dir);
    const origSync = storage.sync.bind(storage);
    // wrap to capture opts
    const mockClient = {
      async getWorkspaces() { return [{ id: "wrk_test", name: "t" }]; },
      async getUsageInfo(_wid: string, page: number) {
        if (page === 0) return [rec({ id: "usg_a", timeCreated: "2026-08-21T00:00:00.000Z" })];
        return [];
      },
    };
    // 用代理 storage 捕获参数
    const fakeStorage = {
      dataDir: dir,
      async sync(c: unknown, opts: { workspaceId: string; full?: boolean; limit?: number }) {
        capturedFull = opts.full;
        capturedLimit = opts.limit;
        // delegate to real
        return (storage as unknown as { sync: typeof origSync }).sync(c as never, opts);
      },
    } as unknown as OpenCodeStorage;
    await runOpencodeSync({ workspace: "wrk_test", full: true, limit: 1, dataDir: dir, storage: fakeStorage, client: mockClient as unknown as never });
    assert.equal(capturedFull, true);
    assert.equal(capturedLimit, 1);
  } finally { cleanup(dir); restoreEnv(); }
});

test("T3 runOpencodeSync 401 抛 cookie 过期提示", async () => {
  saveEnv();
  const dir = tmpDir();
  try {
    process.env.OPENCODE_AUTH = "bad";
    const storage = new OpenCodeStorage(dir);
    const mockClient = {
      async getWorkspaces() { throw new Error("认证失效: OpenCode 凭证已过期或无效（HTTP 401），请刷新 auth cookie（凭证过期）"); },
      async getUsageInfo() { return []; },
    };
    await assert.rejects(() => runOpencodeSync({ dataDir: dir, storage, client: mockClient as unknown as never }), (e: Error) => {
      assert.match(e.message, /凭证过期|认证失效|cookie 过期/);
      return true;
    });
  } finally { cleanup(dir); restoreEnv(); }
});

test("T3 runOpencodeSync 自动发现 workspace（未传时调用 getWorkspaces）", async () => {
  saveEnv();
  const dir = tmpDir();
  try {
    process.env.OPENCODE_AUTH = "tok_auto";
    const storage = new OpenCodeStorage(dir);
    let wsCalled = false;
    const mockClient = {
      async getWorkspaces() { wsCalled = true; return [{ id: "wrk_auto", name: "auto" }]; },
      async getUsageInfo(_wid: string, page: number) {
        if (page === 0) return [];
        return [];
      },
    };
    const out = await runOpencodeSync({ dataDir: dir, storage, client: mockClient as unknown as never });
    assert.equal(wsCalled, true);
    assert.match(out, /wrk_auto/);
  } finally { cleanup(dir); restoreEnv(); }
});

// ---------- T4 runOpencodeExport ----------
test("T4 runOpencodeExport json 默认导出", async () => {
  const dir = tmpDir();
  try {
    const storage = new OpenCodeStorage(dir);
    await storage.mergeHistory([
      rec({ id: "usg_1", timeCreated: "2026-08-20T10:00:00.000Z", model: "m1" }),
      rec({ id: "usg_2", timeCreated: "2026-08-19T10:00:00.000Z", model: "m2" }),
    ]);
    const out = await runOpencodeExport({ dataDir: dir, format: "json", storage });
    const parsed = JSON.parse(out);
    assert.equal(parsed.length, 2);
    assert.equal(parsed[0].id, "usg_1"); // desc order: latest first (storage sorts desc)
  } finally { cleanup(dir); }
});

test("T4 runOpencodeExport csv 导出与表头", async () => {
  const dir = tmpDir();
  try {
    const storage = new OpenCodeStorage(dir);
    await storage.mergeHistory([rec({ id: "usg_1", timeCreated: "2026-08-20T10:00:00.000Z" })]);
    const out = await runOpencodeExport({ dataDir: dir, format: "csv", storage });
    assert.match(out, /^id,workspaceID,timeCreated/);
    assert.match(out, /usg_1/);
  } finally { cleanup(dir); }
});

test("T4 runOpencodeExport 按月过滤", async () => {
  const dir = tmpDir();
  try {
    const storage = new OpenCodeStorage(dir);
    await storage.mergeHistory([
      rec({ id: "usg_08", timeCreated: "2026-08-15T00:00:00.000Z" }),
      rec({ id: "usg_09", timeCreated: "2026-09-01T00:00:00.000Z" }),
    ]);
    const outJson = await runOpencodeExport({ dataDir: dir, format: "json", month: "2026-08", storage });
    const parsed = JSON.parse(outJson);
    assert.equal(parsed.length, 1);
    assert.equal(parsed[0].id, "usg_08");
    const outCsv = await runOpencodeExport({ dataDir: dir, format: "csv", month: "2026-08", storage });
    assert.match(outCsv, /usg_08/);
    assert.doesNotMatch(outCsv, /usg_09/);
  } finally { cleanup(dir); }
});

test("T4 runOpencodeExport --output 写入文件与格式校验", async () => {
  const dir = tmpDir();
  try {
    const storage = new OpenCodeStorage(dir);
    await storage.mergeHistory([rec({ id: "usg_1", timeCreated: "2026-08-20T00:00:00.000Z" })]);
    const outPath = join(dir, "sub", "out.json");
    const summary = await runOpencodeExport({ dataDir: dir, format: "json", output: outPath, storage });
    assert.ok(existsSync(outPath));
    const onDisk = readFileSync(outPath, "utf8");
    assert.match(onDisk, /usg_1/);
    assert.match(summary, /导出完成/);
    assert.match(summary, /out\.json/);
  } finally { cleanup(dir); }
});

test("T4 runOpencodeExport 非法格式抛错", async () => {
  const dir = tmpDir();
  try {
    await assert.rejects(() => runOpencodeExport({ dataDir: dir, format: "xml" }), /未知格式/);
  } finally { cleanup(dir); }
});

test("T4 runOpencodeExport 非法 month 抛错", async () => {
  const dir = tmpDir();
  try {
    await assert.rejects(() => runOpencodeExport({ dataDir: dir, month: "2026-13" }), /无效月份/);
    await assert.rejects(() => runOpencodeExport({ dataDir: dir, month: "bad" }), /无效月份/);
  } finally { cleanup(dir); }
});

test("T4 runOpencodeExport 空历史 csv 仅表头", async () => {
  const dir = tmpDir();
  try {
    const storage = new OpenCodeStorage(dir);
    const out = await runOpencodeExport({ dataDir: dir, format: "csv", storage });
    const lines = out.trim().split("\n");
    assert.equal(lines.length, 1);
    assert.match(lines[0], /^id,/);
  } finally { cleanup(dir); }
});

// ---------- T5 主 CLI 注册 ----------
test("T5 parseArgs opencode 帮助文本包含 opencode", async () => {
  const out = await runCli(["-h"]);
  assert.match(out, /opencode sync/);
  assert.match(out, /opencode export/);
});

test("T5 runCli opencode export 集成（无网络）", async () => {
  const dir = tmpDir();
  try {
    const storage = new OpenCodeStorage(dir);
    await storage.mergeHistory([rec({ id: "usg_cli", timeCreated: "2026-08-20T00:00:00.000Z" })]);
    // 由于 runCli 内部会新建 storage，我们需要让数据落在它读取的 dataDir
    // 因此先用同一个 dir 通过 storage 预置，再调用 runCli 指向同一 dir
    // runCli 内部新建 storage 会读到相同 dir 的文件
    const out = await runCli(["opencode", "export", "--data-dir", dir, "--format", "json"]);
    const parsed = JSON.parse(out);
    assert.equal(parsed[0].id, "usg_cli");
  } finally { cleanup(dir); }
});

test("T5 runCli opencode sync 集成 mock fetch（注入 auth）", async () => {
  saveEnv();
  const dir = tmpDir();
  const origFetch = globalThis.fetch;
  try {
    // mock fetch for client
    const mockFetch = async (url: string, init?: RequestInit) => {
      const headers = (init?.headers ?? {}) as Record<string, string>;
      const fn = headers["x-opencode-fn"] ?? headers["X-Opencode-Fn"] ?? "";
      // decode body to decide
      let bodyStr = String((init as unknown as { body?: string })?.body ?? "");
      let data: unknown = [];
      // 简单：若 header 含 workspaces id 则返回 workspaces，否则返回 usage
      // 我们的 client FN.workspaces = def..., usage = bfd...
      if (fn.includes("def399")) {
        data = [{ id: "wrk_cli", name: "cli" }];
      } else if (fn.includes("bfd684")) {
        // usageHistory: body contains page
        // 首次调用返回一条，第二次空
        // 通过计数判断
        (mockFetch as unknown as { _cnt?: number })._cnt = ((mockFetch as unknown as { _cnt?: number })._cnt ?? 0) + 1;
        const cnt = (mockFetch as unknown as { _cnt?: number })._cnt!;
        if (cnt === 1) {
          data = [{ id: "usg_cli_sync", workspaceID: "wrk_cli", timeCreated: "2026-08-20T00:00:00.000Z", timeUpdated: "2026-08-20T00:00:00.000Z", timeDeleted: null, model: "m1", provider: "inf.oa-compat", inputTokens: 10, outputTokens: 5, reasoningTokens: null, cacheReadTokens: null, cacheWrite5mTokens: null, cacheWrite1hTokens: null, cost: 0.001, keyID: "k1", sessionID: null, enrichment: null }];
        } else {
          data = [];
        }
      } else {
        data = [];
      }
      const { encodePayload } = await import("../src/opencode/seroval.ts");
      const txt = encodePayload([data]);
      return {
        ok: true,
        status: 200,
        async text() { return txt; },
      } as unknown as Response;
    };
    globalThis.fetch = mockFetch as unknown as typeof fetch;
    const out = await runCli(["opencode", "sync", "--auth", "tok_cli", "--workspace", "wrk_cli", "--data-dir", dir]);
    assert.match(out, /同步完成/);
    assert.match(out, /新增/);
    assert.match(out, /耗时/);
  } finally {
    globalThis.fetch = origFetch;
    cleanup(dir);
    restoreEnv();
  }
});

test("T5 runCli opencode 未知参数抛错", () => {
  assert.throws(() => parseArgs(["opencode", "sync", "--by", "model"]), /未知参数/);
  assert.throws(() => parseArgs(["opencode", "export", "--full"]), /未知参数/);
});

test("T5 runCli opencode sync 缺少认证抛友好错误", async () => {
  saveEnv();
  const dir = tmpDir();
  try {
    delete process.env.OPENCODE_AUTH;
    // 使用空 dataDir 且不传 --auth
    await assert.rejects(() => runCli(["opencode", "sync", "--data-dir", dir, "--workspace", "wrk_x"]), /缺少认证信息/);
  } finally { cleanup(dir); restoreEnv(); }
});
