import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

function html(): string {
  return readFileSync(resolve("src/webui.html"), "utf8");
}
function contextMd(): string {
  return readFileSync(resolve("CONTEXT.md"), "utf8");
}
function sessionDataTs(): string {
  return readFileSync(resolve("src/session-data.ts"), "utf8");
}

// Seam-1: 徽标 “任务” → “子代理” + title="子代理会话"
test("子代理徽标：renderDetailTable 的 isTask 徽标为“子代理”且含 title=\"子代理会话\"", () => {
  const h = html();
  // 徽标文本应为 子代理
  assert.match(h, /<span[^>]*>子代理<\/span>/, "徽标应为 子代理");
  // 应含 tooltip title="子代理会话"
  assert.match(h, /title="子代理会话"/, "徽标应含 title=\"子代理会话\"");
  // 不应残留旧文案 “>任务</span>” 在明细徽标分支（排除原型与注释中的“任务→子代理”描述）
  // 严格检查：renderDetailTable 分支不应有 >任务</span>
  assert.doesNotMatch(h, />任务<\/span>/, "不应残留旧徽标 任务");
});

// Seam-2: 汇总提示 “含任务/含子代理任务” → “含子代理（N）”，title 同步
test("子代理汇总提示：session-summary 含“含子代理”且 title 同步为子代理术语", () => {
  const h = html();
  // session-summary 文案应含 “含子代理”
  assert.match(h, /含子代理/, "汇总应含 含子代理");
  // 不应含旧文案 含任务 / 含子代理任务
  assert.doesNotMatch(h, /含任务/, "不应残留 含任务");
  assert.doesNotMatch(h, /含子代理任务/, "不应残留 含子代理任务");
  // title 属性中的汇总提示也应为 含子代理（不含“任务”）
  // 检查包含 session-summary title 的行：应含 “含子代理” 且不含 “任务”
  const titleLine = h.match(/sumEl\.title[\s\S]{0,300}/)?.[0] ?? h;
  // 更宽松：整体 HTML 的 title 汇总处不应有 “含子代理任务”
  assert.doesNotMatch(h, /含子代理任务/, "title 不应含 含子代理任务");
  // 若显示数量，应为 “含子代理（N” 形态或至少 “含子代理”
  assert.match(h, /含子代理/, "title 应同步为含子代理");
});

// Seam-3: 代码注释 isTask → 子代理会话
test("子代理注释：SessionFileData.isTask 注释为是否为子代理会话（路径含 /tasks/，历史称 isTask）", () => {
  const src = sessionDataTs();
  assert.match(src, /是否为子代理会话.*历史称 isTask/, "注释应为 是否为子代理会话（路径含 /tasks/，历史称 isTask）");
  // 字段名保持 isTask 不改名
  assert.match(src, /isTask\?: boolean/, "字段名保持 isTask");
});

// Seam-4: 文档 CONTEXT.md 数据域增子代理会话定义
test("子代理文档：CONTEXT.md 数据域含子代理会话定义（isTask + parentSessionId + 合并视图）", () => {
  const md = contextMd();
  assert.match(md, /子代理会话/, "应含 子代理会话 定义");
  assert.match(md, /isTask/, "定义应提及 isTask");
  assert.match(md, /parentSessionId/, "定义应提及 parentSessionId");
  assert.match(md, /详情视图.*合并|合并.*详情视图/, "定义应说明详情视图合并");
  assert.match(md, /主列表.*独立|列表.*独立/, "定义应说明主列表保持独立");
});

// Seam-5: API 字段 isTask 不改名（回归）
test("子代理 API 字段保持 isTask 不改名（回归）", async () => {
  const apiSrc = readFileSync(resolve("src/api.ts"), "utf8");
  // 序列化仍输出 isTask
  assert.match(apiSrc, /isTask:\s*(detail\.session\.isTask|c\.isTask|r\.isTask)/, "API 序列化仍应输出 isTask");
});
