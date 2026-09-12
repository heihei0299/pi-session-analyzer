import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

function filesUnder(dir: string): string[] {
  const out: string[] = [];
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) out.push(...filesUnder(path));
    else out.push(path);
  }
  return out;
}

test("Ticket 01: token-analyzer 不再暴露或依赖 OpenCode runtime", () => {
  const surfaces = [
    "cmd/token-analyzer/main.go",
    "internal/server/server.go",
    "src/cli.ts",
    "src/api.ts",
    "src/webui.html",
  ];
  for (const path of surfaces) {
    const source = readFileSync(path, "utf8");
    assert.doesNotMatch(source, /api\/opencode|handleOpencode|opencode sync|data-tab="opencode"/i, path);
    assert.doesNotMatch(source, /OPENCODE_AUTH|OPENCODE_WORKSPACE|OPENCODE_DATA_DIR/, path);
  }
});

test("Ticket 01: opencode-analyzer 是无反向私有依赖的独立 module", () => {
  assert.match(readFileSync("opencode-analyzer/go.mod", "utf8"), /^module github\.com\/heihei0299\/opencode-analyzer/m);
  const sources = filesUnder("opencode-analyzer").filter((path) => path.endsWith(".go"));
  assert.ok(sources.length > 0);
  for (const path of sources) {
    const source = readFileSync(path, "utf8");
    assert.doesNotMatch(source, /pi-session-anylize\/internal|token-analyzer\/internal/, path);
  }
  const html = readFileSync("opencode-analyzer/internal/server/webui.html", "utf8");
  assert.match(html, /\/api\/opencode\/sync/);
  assert.match(html, /\/api\/opencode\/audit/);
  assert.match(html, /\/api\/opencode\/history/);
});
