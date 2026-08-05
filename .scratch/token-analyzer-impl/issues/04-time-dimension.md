# 04 — 时间维度：日/周/月汇总 + 时间范围筛选

**What to build:** 给统计增加时间维度——**按日/周/月汇总**与**时间范围筛选**（`--since` / `--until`）。日/周/月汇总与时间筛选以**会话时间戳**（会话 header 的 `timestamp`）为基准归属，保持口径简单。

端到端行为：
- 按日汇总：每天一行（按会话时间戳归属到日）
- 按周/月汇总：同理归属到周/月
- `--since <时间>` / `--until <时间>` 筛选会话时间戳范围，只统计范围内的会话
- 各窗口（总/会话/单请求）与各维度（模型/cwd）均支持时间汇总与筛选
- 时间范围筛选可与 `--model` / `--cwd` 组合使用

**Blocked by:** 01 — 最小闭环：总消耗量统计

**Status:** resolved

## Acceptance criteria

- [x] 日/周/月汇总正确：每期一行，数字为该期内会话之和，与总窗口对得上
- [x] `--since` / `--until` 边界正确（含端点语义明确）
- [x] 与 `--model` / `--cwd` 组合筛选生效
- [x] fixture 覆盖：跨日/跨月会话归属、时间边界含/不含

## 实施总结
- 提交：`e83ad39` — feat: 时间维度——日/周/月汇总 + 时间范围筛选
- 实现的 seams：S1 --since/--until 筛选（闭区间含端点，对全部窗口生效）/ S2 --period day / S3 --period week（ISO 周，周一起始）/ S4 --period month / S5 时间×维度组合（--period + --since/--until + --model/--cwd/--by）
- 验收标准：4 条全部 `- [x]`（见上）
- 测试结果：38/38 全绿（`npm test`，含 S1a-S1f 六个独立场景）
- typecheck：通过（`npm run typecheck`，tsc --noEmit strict）
- 真实数据核对：CLI --period month 与独立 python 实现逐指标完全一致（2 个月份）；--since 2026-08-01 + --until 2026-07-31 与无筛选全量完全一致（闭区间边界正确）
- Code Review 修复：时区基准统一（parseUtcTimestamp 无时区后缀补 Z，filterFiles/periodKey 同基准）/ --period 与 --by 冲突显式报错且校验在 IO 前 / S1 测试拆 6 个独立场景（一个逻辑断言）/ S2 弱正则断言改精确 parseTable
- 遗留 / 后续建议：--period 仅作用于 totals 窗口（阶段②确认范围）；sessions/requests 窗口仅受 since/until 筛选无周期汇总；serialize period/group 同构可后续收敛
