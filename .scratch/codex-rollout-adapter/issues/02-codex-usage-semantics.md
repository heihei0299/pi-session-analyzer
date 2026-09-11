# 02: Codex usage snapshot 计数语义

**Type:** research
**Status:** resolved
**Blocked by:** None
**Research artifact:** `../research/02-codex-usage-semantics.md`

## Question

基于 Codex 官方源码，确认 usage event 的字段和语义：哪些字段是会话累计值、哪些字段是最近一次/当前上下文值，是否存在 reset、重复事件、无 token 增长事件、错误终止和跨 rollout 边界。

研究输出应给出 totals 与 sessions 所需的可靠计数算法前提，并明确哪些数据不能安全推导为 Codex `requests` 窗口。不要直接实现算法；先记录事实、风险和待用户决策的分叉。

## Answer

截至 2026-09-08，官方源码将 usage 分成 `TokenUsage`、`TokenUsageInfo` 和 `TokenUsageRecord` 三层。`TokenUsageInfo.total_token_usage` 是累计值，`last_token_usage` 是最近一次增量；不能逐条累加累计字段。`TokenUsageRecord` 绑定 `response_id`、`turn_id`、`thread_id` 和 `session_id`，并同时保存单 response usage、turn 累计和 thread 累计。

官方测试确认：有 usage 的 response 产生 durable `TokenUsageRecord`；没有 usage 的 completed response 不产生 record；同一 turn 可有多个 response，turn 累计在 response 间增长，换 turn 后重置，而 thread 累计继续增长。v1 totals 应以每个 response 的 `TokenUsageRecord.usage` 一次计入，累计字段只用于校验/展示；TokenCount fallback 与 durable records 不能混加。

因此 Codex `requests` v1 保持 out of scope；缺 usage 不推算为零请求，cost/budget units 也不转成美元。

研究详情见 [`Codex usage snapshot 计数语义`](../research/02-codex-usage-semantics.md)。
