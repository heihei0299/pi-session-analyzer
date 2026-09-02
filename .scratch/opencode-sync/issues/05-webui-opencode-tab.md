# 05: WebUI「OpenCode 对账」面板与可视化图表

**What to build:** 在 Web 界面（`src/webui.html`）中增加专属「OpenCode 对账」Tab，呈现月度模型使用成本的堆叠柱状图、原生请求历史明细表、本地 Pi 会话与云端官方扣费的对账指标条，以及一键「🔄 立即同步」交互，实现云端账单与本地统计的一站式对比与审查。

**Blocked by:** 04: OpenCode HTTP API 服务端端点

**Status:** resolved

- [x] 在 `webui.html` 顶部 Tab 栏增加「OpenCode 对账」导航项（`data-tab="opencode"`）
- [x] 构建对账汇总卡片区（Audit Summary Bar），展示：本地 Pi 统计 Tokens / 费用 vs OpenCode 官方计费、差额百分比、已同步记录数与最后同步时间
- [x] 构建一键「🔄 立即同步」按钮与加载反馈（包含同步中动画与完成/错误 Toast 提示）
- [x] 构建月度使用成本堆叠柱状图（支持月份切换如 2026-07、2026-08，图例与颜色与整体暗色仪表盘风格一致）
- [x] 构建使用历史明细分页表格，展示日期、模型、输入（非缓存+缓存）、输出、推理、成本、会话 ID，支持按模型筛选与翻页
- [x] 确保首屏加载优先从本地缓存渲染（毫秒级呈现），无网络阻塞感
- [x] 进行端到端浏览器交互冒烟测试与视觉对齐

## 实施总结

**Commit:** 48b48ff `feat(opencode-sync): WebUI OpenCode audit tab (#05)` — 基于 f7566f1 单 issue 闭环

**实现范围（严格单 issue 边界）：**
- 仅修改 `src/webui.html`（单 HTML 内联前端，+390 行）与新增骨架测试 `test/05-opencode-webui.test.ts`；未改动 01-04 核心逻辑（仅读 `/api/opencode/*` 四端点）
- 复用现有暗色仪表盘样式变量（--bench/--brass/--enamel/--paper）与 tab 切换逻辑（class active），无新依赖，未引入 Chart.js

**Seams 落地：**
- T1 Tab 导航：顶部 `.tabs` 新增 `data-tab="opencode"` 按钮及对应 `.tab-panel#tab-opencode`，复用现有 `switchTab`/`refreshAll` 模式
- T2 对账汇总卡片区：`#opencode-audit.audit-summary` 4 卡片（本地 Pi / 官方 / 差额% / 同步状态），数据来自 `GET /api/opencode/audit` 与 `GET /api/opencode/history`，成本格式化 1e-8 微单位→$X.XXXX（`fmtMicroCost` / `fmtCost`）与 token `fmtCompact`
- T3 一键同步：`#opencode-sync` → `POST /api/opencode/sync`，同步中 `disabled+syncing` 旋转动画，完成/错误 Toast（`#opencode-toast`），成功后并行刷新 audit/chart/history 并更新 `lastSync`
- T4 月度成本堆叠柱状图：`canvas#opencode-cost-chart` 2D 绘制（暗色仪表盘 + 按模型配色 `OPENCODE_COLORS`），X 轴日期 Y 轴 cost 堆叠按 model，`input type="month"#opencode-month` 切换时重 fetch `GET /api/opencode/costs?year&month`，1e-8 换算
- T5 历史明细分页表：`table#opencode-history` + `select#opencode-model-filter` 动态选项（首次拉取全量去重模型） + `pager#opencode-prev/next` 与 `select#opencode-page-size`，调用 `GET /api/opencode/history?page&size&model` 服务端分页
- T6 首屏本地缓存秒开：`init()` 中 `initOpencodeMonth()+refreshOpencodeAll()` 并行拉取 costs/audit/history（均本地 JSON，无需 sync），`Promise.allSettled` 毫秒级渲染；sync 为可选后台动作

**TDD 证据：**
- 红：`test/05-opencode-webui.test.ts` 7 用例初始全失败（缺 opencode 标记）
- 绿：补丁后 7/7 通过；`TZ=Asia/Shanghai node --test 'test/**/*.test.ts'` 全量 250/250 通过（含原 08-overview 等），`npm run typecheck` 通过，`npm run build` 成功（`dist/webui.html` 同步）

**Code Review 自检（双轴）：**
- Standards：复用现有 tab/card/tape 样式与 fetch 模式，最小 JS 行数，无图表库，ponytail 极简，样式与 bench/brass 变量一致
- Spec：6 验收全绿 — Tab/审计卡片/同步反馈/堆叠柱状图+月份切换/历史表+筛选翻页/本地缓存秒开

**卫生：** 工作区仅预期改动（`src/webui.html` + `test/05-opencode-webui.test.ts`），无临时产物，`git merge-base --is-ancestor f7566f1 HEAD` 通过
