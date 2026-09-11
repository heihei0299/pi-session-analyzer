# 07: Codex response model 归属研究

**Type:** research
**Status:** resolved
**Blocked by:** None
**Research artifact:** `../research/04-codex-model-attribution.md`

## Question

基于 Codex 官方源码确认 response/turn 级 model、model provider 和 model reroute 信息实际持久化在哪些 rollout item 或 metadata 中，以及 model 变化是否能可靠映射到每条 `TokenUsageRecord`。

研究结果用于落实 normalized ledger 的“response/turn 优先，SessionMeta 回退，最后 unknown”规则；若 response 级归属不存在，必须明确降级边界，不得猜测。


## Answer

官方源码确认：`TokenUsageRecord` 不含 model/provider；turn model 位于 `TurnContextItem`，session provider 位于 `SessionMeta.model_provider`；`ModelReroute` 没有 response/turn identity。

采用“response/turn 明确关联 → session metadata → `unknown`”的归属顺序。reroute 只作诊断信息，不强行把目标 model 归属到 usage record；model 不可知时仍计 token。研究详情见 [`Codex response model 归属研究`](../research/04-codex-model-attribution.md)。
