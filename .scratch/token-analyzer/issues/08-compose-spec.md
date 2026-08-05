# 08 — 综合 spec

Type: task
Status: resolved
Blocked by: 01, 02, 03, 04, 05, 06, 07, 11

## Question

将全部已决决策综合为可交接的 **`.scratch/token-analyzer/spec.md`**（经 `/to-spec` 技能）。

## 背景

这是 map 的终点：当 01–07 全部解析后，路线已清晰，唯一剩下的是把决策落成 spec。spec 是 Destination 的载体——供后续 session 按 spec 实现工具（本 effort 不写实现代码）。

## Resolution

当 01–07 全部 `resolved` 且无剩余阻塞时，运行 `/to-spec` 综合 spec；将完成情况写入本文件 `## Answer` 段落，`Status: resolved`，并在 map `Decisions so far` 追加一行。

## Answer

**已交付：** `.scratch/token-analyzer/spec.md`（7.7KB）——经 `/to-spec` 技能综合全部已决决策（01/02/03/04/05/06/07/09/10/11）产出，标注 `ready-for-agent`。

包含：Problem Statement / Solution（数据源与合法性判定、统计口径决策锚表、指标与维度、运行形态）/ User Stories（20 条）/ Implementation Decisions（数据读取层、聚合模型、模型归属、cwd 规范化、时间归属、实时监控、花费、输出）/ Testing Decisions（单 seam：JSONL fixture → CLI 输出；13 个口径边界用例）/ Out of Scope / Further Notes（与 pi 自身统计的差异、数据量参考）。

map 终点达成：11 个 ticket 全部 resolved，route 清晰，spec 可交接供后续 session 实现（本 effort 不写实现代码）。
