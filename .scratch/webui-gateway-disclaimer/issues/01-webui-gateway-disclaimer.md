# 01-webui-gateway-disclaimer

Status: resolved
Type: task
Spec: ../spec.md

## 任务

在 webui 状态行下方添加常驻口径说明（静态文案）：

- 统计仅含 pi 会话中的对话请求（assistant+usage）；
- pi 压缩/摘要等内部请求与 opencode 等其他客户端请求不计入；
- 与 pi-switch 网关数字存在结构性差异，属预期行为。

## 约束

- 不改统计逻辑/数据源/筛选行为；
- 文案中文，样式复用现有 dim 小字；
- 08-overview-ui.test.ts 加骨架断言；
- agent_browser 验收。

## Comments

- 2026-08-07：用户决策「ui上添加展示，结构性接受这些差异」（不依赖网关）。

## 实现记录（2026-08-07）

- src/webui.html：状态行下方新增 `#scope-note`（CSS 12px dim 小字），常驻口径说明；
- test/08-overview-ui.test.ts：S13 新增 4 条骨架断言（元素存在 + 口径/内部请求/结构性差异文案）；
- npm test 116 全绿；agent_browser 渲染验收（block 可见、dim 灰、状态行下方）；
- CONTEXT.md/README 补充对账差异源记录（内部请求 + 其他客户端）。
