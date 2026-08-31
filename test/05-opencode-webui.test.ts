/**
 * 05 WebUI OpenCode 对账 Tab 骨架断言
 * 手工验收 + 骨架标记（ponytail 极简，复用现有 tab/CSS/JS 模式，不引图表库）
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

function html(): string {
  return readFileSync(resolve("src/webui.html"), "utf8");
}

test("T1 Tab 导航：data-tab=opencode 按钮及 panel", () => {
  const h = html();
  assert.match(h, /data-tab="opencode"/, "应有 data-tab=opencode 导航");
  assert.match(h, /id="tab-opencode"/, "应有 tab-panel#tab-opencode");
  assert.match(h, /class="tab-panel"/, "panel 应有 tab-panel 类");
  assert.match(h, /OpenCode 对账/, "按钮文案含 OpenCode 对账");
});

test("T2 Audit Summary Bar：汇总卡片区与格式化", () => {
  const h = html();
  // 审计汇总容器
  assert.match(h, /id="opencode-audit"|class="audit-summary"|opencode-audit/, "应有 audit summary 容器");
  // 需展示本地/官方/diff 相关文案
  assert.match(h, /v-opencode-local|opencode-local|本地/, "应有本地统计占位");
  assert.match(h, /v-opencode-remote|opencode-remote|官方|OpenCode/, "应有官方计费占位");
  // 已同步记录数与最后同步时间占位
  assert.match(h, /opencode-last-sync|最后同步/, "应有最后同步时间占位");
  // 成本格式化 1e-8 相关逻辑或 cost 展示
  assert.match(h, /1e-8|1e8|totalCost|fmtCost|micro/, "应含微单位 1e-8 成本换算逻辑或 fmtCost");
});

test("T3 一键同步按钮与加载反馈", () => {
  const h = html();
  assert.match(h, /id="opencode-sync"/, "应有同步按钮 #opencode-sync");
  assert.match(h, /立即同步/, "按钮文案含立即同步");
  assert.match(h, /opencode-sync.*disabled|disabled.*opencode-sync|syncing|旋转|spinner/i, "应有同步中 disabled/动画相关逻辑");
  assert.match(h, /Toast|toast|opencode-toast/, "应有 Toast 提示容器/逻辑");
  assert.match(h, /\/api\/opencode\/sync/, "JS 应调用 POST /api/opencode/sync");
});

test("T4 月度成本堆叠柱状图 Canvas + 月份切换", () => {
  const h = html();
  assert.match(h, /<canvas[^>]*id="opencode-cost-chart"/, "应有 canvas#opencode-cost-chart");
  assert.match(h, /opencode-month|type="month"|month-picker/, "应有月份切换控件");
  assert.match(h, /\/api\/opencode\/costs/, "JS 应 fetch /api/opencode/costs");
  // Canvas 绘制逻辑（无图表库）
  assert.match(h, /getContext\("2d"\)|getContext\('2d'\)/, "应有 Canvas 2d 绘制");
  assert.doesNotMatch(h, /Chart\.js|chart\.js|cdn\.chartjs/i, "不应引入 Chart.js");
});

test("T5 使用历史明细分页表格 + 模型筛选", () => {
  const h = html();
  assert.match(h, /id="opencode-history"/, "应有历史明细表 #opencode-history");
  assert.match(h, /opencode-model-filter|id="opencode-model"/, "应有模型筛选 select");
  assert.match(h, /opencode-page|opencode-prev|opencode-next|pager/, "应有翻页控件");
  assert.match(h, /\/api\/opencode\/history/, "JS 应 fetch /api/opencode/history");
  // 表列文案
  assert.match(h, /输入/, "表头应含输入");
  assert.match(h, /输出/, "表头应含输出");
  assert.match(h, /成本|费用/, "表头应含成本");
  assert.match(h, /会话/, "表头应含会话 ID");
});

test("T6 首屏本地缓存秒开：优先 fetch 本地持久化数据", () => {
  const h = html();
  // 无需等待 sync，chart/table 立即渲染的 JS 逻辑：初始化即 fetch costs/history/audit
  assert.match(h, /fetchOpencodeCosts|opencode.*costs/i, "应有 fetchOpencodeCosts 相关逻辑");
  assert.match(h, /fetchOpencodeHistory|opencode.*history/i, "应有 fetchOpencodeHistory 相关逻辑");
  assert.match(h, /fetchOpencodeAudit|opencode.*audit/i, "应有 fetchOpencodeAudit 相关逻辑");
  // 确保 opencode tab 的 refresh 在 DOMContentLoaded/init 中注册
  assert.match(h, /DOMContentLoaded|init\(\)|refreshOpencode/, "应有初始化自动加载逻辑");
});

test("T6 样式与复用：复用暗色仪表盘变量，无新依赖", () => {
  const h = html();
  assert.match(h, /--brass|--bench|--enamel|--paper/, "应复用现有 CSS 变量");
  assert.doesNotMatch(h, /npm.*chart|yarn add.*chart/i, "不应引入新依赖说明");
});
