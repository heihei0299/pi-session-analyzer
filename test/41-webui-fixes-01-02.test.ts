/**
 * Ticket 01 — 严格日期校验（API + npm/TS CLI）。
 * Ticket 02 — 会话管理行内重命名取消不报错。
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { runCli } from "../src/cli.ts";
import { startWebServer } from "../src/server.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

test("T1 非法纯日期 API 400，合法闰年/月末日期不变", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader({ timestamp: "2026-08-15T10:00:00.000Z" }), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    for (const [query, label] of [["since=2026-02-30", "since"], ["until=2026-07-32", "until"], ["since=2026-13-99", "since"]] as const) {
      const res = await fetch(new URL(`/api/totals?${query}`, server.url));
      assert.equal(res.status, 400, `${query} 应 400`);
      const body = (await res.json()) as { detail: string };
      assert.match(body.detail, new RegExp(`无效 ${label}`));
    }
    for (const query of ["since=2024-02-29", "since=2026-08-31", "until=2026-08-31"]) {
      const res = await fetch(new URL(`/api/totals?${query}`, server.url));
      assert.equal(res.status, 200, `${query} 合法日期应 200`);
    }
  } finally {
    await server.close();
    removeFixture(dir);
  }
});

test("T1 npm/TS CLI 非法 --since/--until 拒绝，合法日期不拒绝", async () => {
  const dir = makeFixture({});
  try {
    await assert.rejects(() => runCli(["--dir", dir, "--since", "2026-02-30", "--format", "json"]), /无效 since/);
    await assert.rejects(() => runCli(["--dir", dir, "--until", "2026-07-32", "--format", "json"]), /无效 until/);
    await assert.doesNotReject(() => runCli(["--dir", dir, "--since", "2024-02-29", "--format", "json"]));
    await assert.doesNotReject(() => runCli(["--dir", dir, "--until", "2026-08-31", "--format", "json"]));
  } finally {
    removeFixture(dir);
  }
});

test("T2 会话管理重命名取消静默，只有保存空名才提示", () => {
  const html = readFileSync(join("src", "webui.html"), "utf8");
  const fnStart = html.indexOf("function startSessionRename");
  const fnEnd = html.indexOf("function restoreSessionName", fnStart);
  assert.ok(fnStart >= 0 && fnEnd > fnStart, "应能定位 startSessionRename");
  const source = html.slice(fnStart, fnEnd);
  assert.match(source, /if\(!save\)\s*\{ restoreSessionName\(input,row,prevText\); return; \}/, "Esc/失焦取消应直接恢复且无错误");
  assert.match(source, /if\(!name\)\s*\{ errEl\.textContent="显示名不能为空"; restoreSessionName\(input,row,prevText\); return; \}/, "仅保存空名提示");
  assert.doesNotMatch(source, /if\(!save\|\|!name\)/, "取消和空名不能共用同一错误分支");
});
