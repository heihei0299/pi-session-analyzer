/**
 * 性能回归测试（diagnosing-bugs 流程 Phase 5 产物，锁定异常 CPU 占用诊断 H1/H2/H5）。
 *
 * 背景（本文件锁定的性能回归）：
 * - H1：watch 每轮对全部文件重读首行（isSessionFile 无缓存）→ 实测 189ms/轮 ≈ 19% 单核常驻。
 *       修复：已跟踪文件跳过首行重验，仅新增 / inode 变化文件重验（构造器注入计数验证）。
 * - H2：serve 缓存失效 = 全量重解析（实测 486ms 尖峰）。修复：按文件级快照增量重读，
 *       仅重读变化的文件（fileLoader 注入计数验证）；保持「无变化同引用」契约
 *       （15-server-cache.test.ts 已断言，本文件补充增量语义断言）。
 * - H5：filterFiles 的 normalizeCwd（realpathSync）逐文件重复且无缓存。
 *       修复：补文件级缓存（与 groupRowsFromFiles 的 normCwdCache 对齐）。
 *       fs.realpathSync 为静态绑定无法注入，故以行为等价测试作为护栏
 *       （seam 受限说明：性能本身靠代码审查保障）。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { appendFileSync, rmSync, writeFileSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  makeFixture,
  removeFixture,
  sessionHeader,
  messageEntry,
  assistantUsage,
} from "./helpers.ts";
import { IncrementalReader, applyIncrements } from "../src/watch.ts";
import { emptyTotals } from "../src/aggregate.ts";
import {
  readSessionFilesCached,
  readSessionFiles,
  filterFiles,
  normalizeCwd,
  analyzeFile,
  __setFileLoaderForTest,
} from "../src/analyze.ts";
import { groupRowsFromFiles } from "../src/analyze.ts";

// ---------- H1：watch 首行重验计数 ----------

test("H1 未变化/追加文件不重读首行（每轮仅 stat，不触发 isSessionFile）", async () => {
  const dir = makeFixture({
    "a.jsonl": [
      sessionHeader({ id: "a", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    ],
  });
  const a = join(dir, "a.jsonl");
  let verifyCalls = 0;
  const spy = async (): Promise<boolean> => {
    verifyCalls++;
    return true;
  };
  const reader = new IncrementalReader(dir, spy);
  try {
    await reader.readIncrements();
    assert.equal(verifyCalls, 1, "首次扫描应验证 1 次（新增文件）");

    await reader.readIncrements();
    assert.equal(verifyCalls, 1, "未变化文件不应重读首行");

    // 追加写入（append-only，首行类型不变）→ 不应重读首行
    appendFileSync(a, JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 200 }) })) + "\n");
    await reader.readIncrements();
    assert.equal(verifyCalls, 1, "追加写入不应重读首行");
  } finally {
    removeFixture(dir);
  }
});

test("H1b 替换为残留文件后不再纳入（inode 变化触发首行重验）", async () => {
  const dir = makeFixture({
    "sess.jsonl": [
      sessionHeader({ id: "s1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
    ],
  });
  const file = join(dir, "sess.jsonl");
  const reader = new IncrementalReader(dir);
  const totals = emptyTotals();
  try {
    await applyIncrements(reader, totals);
    assert.equal(totals.requests, 1);
    assert.equal(totals.input, 100);

    // 替换为残留文件（新 inode，首行 type=message 非 session）
    rmSync(file);
    writeFileSync(file, JSON.stringify(messageEntry({ role: "user", content: [] })) + "\n");

    const inc = await reader.readIncrements();
    assert.equal(inc.length, 0, "替换成残留文件后不应有增量（被排除）");
    // 原贡献保留在 totals（历史已发生，不扣减）
    assert.equal(totals.requests, 1);
    assert.equal(totals.input, 100);
  } finally {
    removeFixture(dir);
  }
});

// ---------- H2：serve 缓存增量重读 ----------

function makeDir(): string {
  return mkdtempSync(join(tmpdir(), "token-analyzer-perf-"));
}
function writeSession(dir: string, name: string, input: number): void {
  const lines = [
    sessionHeader({ timestamp: "2026-07-31T01:55:30.577Z" }),
    messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input }) }),
  ]
    .map((e) => JSON.stringify(e))
    .join("\n") + "\n";
  writeFileSync(join(dir, name), lines);
}

test("H2 数据变化时只重读变化的文件（增量失效，不全量重解析）", async () => {
  const dir = makeDir();
  writeSession(dir, "a.jsonl", 100);
  writeSession(dir, "b.jsonl", 200);
  const a = join(dir, "a.jsonl");
  try {
    await readSessionFilesCached(dir); // 首次：全量
    let loads: string[] = [];
    __setFileLoaderForTest(async (f) => {
      loads.push(f);
      return analyzeFile(f);
    });
    try {
      // 追加 a.jsonl → 只应重读 a
      appendFileSync(a, JSON.stringify(messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 999 }) })) + "\n");
      const r = await readSessionFilesCached(dir);
      assert.equal(r[0].items.length, 2, "重读后 a 含追加消息");
      assert.equal(loads.length, 1, "只重读 1 个变化文件");
      assert.ok(loads[0]!.endsWith("a.jsonl"), "重读的是变化的 a.jsonl");

      // 无变化 → 0 重读 + 同一引用（缓存契约）
      loads = [];
      const r2 = await readSessionFilesCached(dir);
      assert.equal(loads.length, 0, "无变化不应重读");
      assert.equal(r2, r, "无变化应命中缓存（同一引用）");

      // 新增 c.jsonl → 只重读 c
      loads = [];
      writeSession(dir, "c.jsonl", 300);
      const r3 = await readSessionFilesCached(dir);
      assert.equal(loads.length, 1, "新增只重读新文件");
      assert.ok(loads[0]!.endsWith("c.jsonl"));
      assert.equal(r3.length, 3);

      // 删除 b.jsonl → 不触发任何重读，结果剔除 b
      loads = [];
      rmSync(join(dir, "b.jsonl"));
      const r4 = await readSessionFilesCached(dir);
      assert.equal(loads.length, 0, "删除文件不触发重读");
      assert.equal(r4.length, 2, "删除后结果剔除 b");
    } finally {
      __setFileLoaderForTest(analyzeFile); // 还原注入
    }
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

// ---------- H5：filterFiles cwd 规范化与分组一致 ----------

test("H5 cwd 筛选与分组使用同一规范化结果（realpath 归一一致）", async () => {
  const dir = makeFixture({
    "s.jsonl": [
      sessionHeader({ id: "s1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj/a" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100 }) }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 50 }) }),
    ],
  });
  try {
    const files = await readSessionFiles(dir);
    // 传尾斜杠变体：应经 normalizeCwd 归一后命中
    const filtered = filterFiles(files, { cwd: "/proj/a/" });
    assert.equal(filtered.length, 1, "cwd 尾斜杠变体应命中");
    assert.equal(filtered[0]!.sessionId, "s1");
    // 分组（内部有 normCwdCache）与筛选的规范化结果一致
    const groups = groupRowsFromFiles(files, "cwd");
    assert.equal(groups.length, 1);
    assert.equal(groups[0]!.cwd, normalizeCwd("/proj/a/"), "分组规范化结果与 normalizeCwd 一致");
    assert.equal(groups[0]!.requests, 2);
  } finally {
    removeFixture(dir);
  }
});
