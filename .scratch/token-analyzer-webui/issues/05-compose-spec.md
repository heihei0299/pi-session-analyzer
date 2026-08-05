# 综合 spec（webui 可交接文档）

**Map**: token-analyzer-webui — see `.scratch/token-analyzer-webui/map.md`
**Type**: task（AFK——agent 独立综合，产出 spec 段落）
**Status**: resolved

## Answer

**已交付：** `.scratch/token-analyzer-webui/spec.md`（8.8KB）——经 `/to-spec` 技能综合全部已决决策（01/02/03/04/06）产出，标注 `ready-for-agent`。

包含：Problem Statement / Solution（运行形态、HTTP API 表、前端 UI 布局、自动刷新机制、会话管理 tab）/ User Stories（20 条）/ Implementation Decisions（模块组织、serve 参数、路由分发、API 实现、重命名实现、前端实现、数值格式化、零花费标注、统计口径）/ Testing Decisions（HTTP 层 seam：JSONL fixture → serve 端点断言 + 重命名文件系统副作用；12 个覆盖用例）/ Out of Scope / Further Notes（与 CLI --watch 差异、重命名 pi 兼容性、活跃阈值 5min、数据量参考）。

map 终点达成：6 个 ticket 全部 resolved（01-04, 06 决策 + 05 综合），route 清晰，spec 可交接供后续 session 实现 webui（本 effort 不写实现代码）。
**Blocked by**: 01-serve-command-and-server, 02-http-api-design, 03-ui-layout-pi-switch, 04-realtime-refresh, 06-session-management

## Question

将全部已决决策综合为可交接的 **`.scratch/token-analyzer-webui/spec.md`**（经 `/to-spec` 技能）。

## 背景

这是 map 的终点：当 01–04 全部解析后，路线已清晰，唯一剩下的是把决策落成 spec。spec 是 Destination 的载体——供后续 session 按 spec 实现 webui（本 effort 不写实现代码）。

## Resolution

当 01–04 全部 `resolved` 且无剩余阻塞时，运行 `/to-spec` 综合 spec；将完成情况写入本文件 `## Answer` 段落，`Status: resolved`，并在 map `Decisions so far` 追加一行。

## Comments
