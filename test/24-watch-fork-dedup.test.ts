/**
 * watch 模式 fork 会话去重（issue 01-watch-fork-dedup）。
 *
 * 背景：CLI/webui 数据读取层（analyzeFile）已剔除 fork 会话的复制历史
 * （message.timestamp < forkTs，ticket 25）；本文件补齐 --watch 增量读取器
 * 同口径：首次读取 fork 会话时解析 header（parentSession + timestamp → forkTs），
 * 复制历史不计入实时 totals；forkTs 随文件跟踪状态持久化，替换/重读复用；
 * 追加路径不受影响。watch 输出与静态 CLI totals 逐字段一致。
 *
 * 断言：首读剔除复制历史 / 追加正常计入 / 替换重读复用 forkTs / 与 CLI 一致。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { appendFileSync, rmSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import {
  makeFixture,
  removeFixture,
  sessionHeader,
  messageEntry,
  assistantUsage,
} from "./helpers.ts";
import { IncrementalReader, applyIncrements } from "../src/watch.ts";
import { runCli } from "../src/cli.ts";
import { emptyTotals, type Totals } from "../src/aggregate.ts";

const FORK_FILE = "2026-08-05T10-10-00-000Z_fork.jsonl";
const FORK_TS = "2026-08-05T10:10:00.000Z";

/** fork 会话 fixture：header.parentSession + 2 条复制历史（ts < forkTs）+ 1 条 fork 后新增（ts >= forkTs） */
function buildForkDir(): string {
  return makeFixture({
    [FORK_FILE]: [
      { ...sessionHeader({ id: "f1", timestamp: FORK_TS, cwd: "/proj" }), parentSession: "/proj/2026-08-05T09-00-00-000Z_parent.jsonl" },
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }, { timestamp: "2026-08-05T10:00:00.000Z" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }, { timestamp: "2026-08-05T10:05:00.000Z" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 300 }) }, { timestamp: "2026-08-05T10:12:00.000Z" }),
    ],
  });
}

test("T1 watch 首读 fork 会话剔除复制历史（ts < forkTs），仅计 fork 后新增", async () => {
  const dir = buildForkDir();
  const reader = new IncrementalReader(dir);
  const totals: Totals = emptyTotals();
  try {
    await applyIncrements(reader, totals);
    assert.equal(totals.requests, 1, "复制历史不计入，仅 fork 后新增 1 条");
    assert.equal(totals.input, 300, "计入的是 fork 后新增消息的 usage");
  } finally {
    removeFixture(dir);
  }
});

test("T2 fork 后追加消息正常计入（追加路径不受 forkTs 影响）", async () => {
  const dir = buildForkDir();
  const file = join(dir, FORK_FILE);
  const reader = new IncrementalReader(dir);
  const totals: Totals = emptyTotals();
  try {
    await applyIncrements(reader, totals);
    assert.equal(totals.requests, 1, "首读仅 fork 后新增 1 条");

    // fork 后继续对话：追加 ts >= forkTs 的新消息
    appendFileSync(
      file,
      JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 400 }) }, { timestamp: "2026-08-05T10:15:00.000Z" })) + "\n",
    );
    const changed = await applyIncrements(reader, totals);
    assert.equal(changed, true, "追加后应有增量");
    assert.equal(totals.requests, 2, "fork 后新增消息照常累计");
    assert.equal(totals.input, 700, "复制历史始终不计入（100+200 未出现）");
  } finally {
    removeFixture(dir);
  }
});

test("T3 已跟踪 fork 会话替换/重读复用初始 forkTs，复制历史不重复计入", async () => {
  const dir = buildForkDir();
  const file = join(dir, FORK_FILE);
  const reader = new IncrementalReader(dir);
  const totals: Totals = emptyTotals();
  try {
    await applyIncrements(reader, totals);
    assert.equal(totals.requests, 1);

    // 替换文件（新 inode）：同 fork header + 复制历史 + 旧新增 + 1 条新消息
    rmSync(file);
    writeFileSync(
      file,
      [
        JSON.stringify({ ...sessionHeader({ id: "f1", timestamp: FORK_TS, cwd: "/proj" }), parentSession: "/proj/2026-08-05T09-00-00-000Z_parent.jsonl" }),
        JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }, { timestamp: "2026-08-05T10:00:00.000Z" })),
        JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }, { timestamp: "2026-08-05T10:05:00.000Z" })),
        JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 300 }) }, { timestamp: "2026-08-05T10:12:00.000Z" })),
        JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 500 }) }, { timestamp: "2026-08-05T10:20:00.000Z" })),
      ].join("\n") + "\n",
    );

    await applyIncrements(reader, totals);
    // 重读：扣减旧贡献（1 条 300）+ 新内容剔除复制历史（300+500）→ 净 +500
    assert.equal(totals.requests, 2, "替换重读后复制历史不重复计入（净 +1 条）");
    assert.equal(totals.input, 800, "净增的是新消息 500");
  } finally {
    removeFixture(dir);
  }
});

test("T4 watch 输出与静态 CLI totals 逐字段一致（含 fork 会话数据集）", async () => {
  const dir = makeFixture({
    // 父会话：2 条正常消息
    "2026-08-05T09-00-00-000Z_parent.jsonl": [
      sessionHeader({ id: "p1", timestamp: "2026-08-05T09:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }, { timestamp: "2026-08-05T10:00:00.000Z" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }, { timestamp: "2026-08-05T10:05:00.000Z" }),
    ],
    // fork 会话：复制 2 条历史（剔除）+ 1 条 fork 后新增
    [FORK_FILE]: [
      { ...sessionHeader({ id: "f1", timestamp: FORK_TS, cwd: "/proj" }), parentSession: "/proj/2026-08-05T09-00-00-000Z_parent.jsonl" },
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }, { timestamp: "2026-08-05T10:00:00.000Z" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }, { timestamp: "2026-08-05T10:05:00.000Z" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 300 }) }, { timestamp: "2026-08-05T10:12:00.000Z" }),
    ],
  });
  try {
    // watch 路径：单步驱动至稳态（首读即全量）
    const reader = new IncrementalReader(dir);
    const totals: Totals = emptyTotals();
    await applyIncrements(reader, totals);

    // 静态 CLI 路径
    const out = JSON.parse(await runCli(["--dir", dir, "totals", "--format", "json"])) as Record<string, unknown>;
    const fields = ["requests", "input", "output", "cacheRead", "cacheWrite", "reasoning", "totalTokens", "cost", "cacheRate"] as const;
    for (const f of fields) {
      assert.equal((totals as unknown as Record<string, unknown>)[f], out[f], `字段 ${f} 一致`);
    }
  } finally {
    removeFixture(dir);
  }
});

test("T5 边界：fork 后新增消息 ts == forkTs 保留（仅 ts < forkTs 剔除）", async () => {
  // 复制历史 2 条（ts < forkTs）+ 1 条 ts 恰好 == forkTs 的消息（fork 时刻发起，应保留）
  const dir = makeFixture({
    [FORK_FILE]: [
      { ...sessionHeader({ id: "f1", timestamp: FORK_TS, cwd: "/proj" }), parentSession: "/proj/2026-08-05T09-00-00-000Z_parent.jsonl" },
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }, { timestamp: "2026-08-05T10:00:00.000Z" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) }, { timestamp: "2026-08-05T10:05:00.000Z" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 999 }) }, { timestamp: FORK_TS }),
    ],
  });
  const reader = new IncrementalReader(dir);
  const totals: Totals = emptyTotals();
  try {
    await applyIncrements(reader, totals);
    assert.equal(totals.requests, 1, "ts == forkTs 的消息保留（不是复制历史）");
    assert.equal(totals.input, 999);
  } finally {
    removeFixture(dir);
  }
});
