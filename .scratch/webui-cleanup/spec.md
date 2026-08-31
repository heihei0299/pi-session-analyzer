# WebUI 清理 — 移除网关可比预设与会话管理默认收起（Spec）

**状态**: ready-for-agent（由 grill 轮次 Q1-Q8 共识产出，2026-09-01）

**前置**: 依赖 `src/webui.html` 既有实现（`GATEWAY_SINCE`、`applyPreset("gateway")`、`session-group` 展开逻辑）。本次为展示层局部清理，不涉及统计口径与 `SessionData` 深模块。

---

## Problem Statement

- 总览默认打开正确，但预设组中 `自 8/1（网关可比）` 为历史对账专用（网关数据起点 `2026-08-01T00:00:00Z`），当前用户日常关注“今天”，该按钮造成视觉噪音与默认窗口混淆。
- 会话管理页按 `cwd` 分组，当前组默认展开，组数多时首屏过长，需手动逐个收起。
- 默认预设当前为 `gateway`，与用户期望的“今天”不一致。

## Solution

前端展示层最小清理，不改统计口径：

1. **移除网关可比预设整套**：按钮 `data-preset="gateway"`、`GATEWAY_SINCE` 常量、`applyPreset("gateway")` 分支、状态行 `（网关可比）` 标注。
2. **默认预设改为 `today`**：`init()` 末尾 `applyPreset("gateway")` → `applyPreset("today")`，首屏即今天。
3. **会话管理默认收起**：`session-group` 创建时即 `class="session-group collapsed"`，保留标题点击 `toggle("collapsed")`，不加批量按钮。

## User Stories

1. 作为用户，我打开 `token-analyzer serve` 首屏即看到总览且预设选中“今天”，无需手动切换。
2. 作为用户，我在预设组中不再看到“自 8/1（网关可比）”，减少无关选项。
3. 作为用户，我打开会话管理看到各项目组默认收起，点击标题展开，首屏更紧凑。

## Implementation Decisions

- **预设组**：HTML 中删 `<button data-preset="gateway">`；JS 删 `const GATEWAY_SINCE`、`applyPreset` 中 `else if (preset === "gateway")` 整段（含 `state.since = GATEWAY_SINCE`）、`filterRangeText`/`updateStatusRange` 中对 `state.preset === "gateway"` 的标注拼接；预设顺序保持 `今天/7天/30天/全部/自定义`。
- **默认预设**：`init()` 末尾 `applyPreset("gateway")` → `applyPreset("today")`；`state.preset` 初始值仍为 `"gateway"` 仅作占位，`applyPreset` 会立即覆盖为 `"today"`，或同步改为 `"today"` 初始值。
- **会话管理**：`refreshSessionGroups` 中 `g = document.createElement("div"); g.className = "session-group";` → `g.className = "session-group collapsed";`，其余折叠逻辑不变。
- **文档**：`README.md` 中“自 8/1（网关可比，默认）”改为“今天（默认）”，`CONTEXT.md` 网关可比窗口段落保留历史说明但注明前端已移除。

## Testing Decisions

- **自动化**：无新增自动化（前端展示层）；`npm test` 132 用例回归通过即可。
- **手工验收**：
  - `npm run build && TZ=Asia/Shanghai node dist/cli.js serve --port 0` 启动后浏览器访问：总览为默认 tab，预设“今天”高亮，状态行显示今天范围且无“（网关可比）”；
  - 会话管理 tab 各组默认收起，点击标题展开/收起正常；
  - 预设按钮组无“自 8/1”。

## Out of Scope

- 统计口径、`SessionData`/`TimeRange`、HTTP API、CLI 参数、分组/ period 汇总、自动刷新、导出、重命名逻辑均不改。
- 不新增批量展开/收起按钮。
- 不清理后端 `SessionData` 中对 `gateway` 的历史注释（仅前端）。

## Further Notes

- 本次为纯前端清理，`dist/` 不入库，`npm run build` 后 `dist/webui.html` 随 `src/webui.html` 同步。
- 若后续需恢复网关可比窗口，可通过自定义预设输入 `2026-08-01T00:00:00Z` 实现，无需保留快捷按钮。
