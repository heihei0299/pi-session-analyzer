# 02: Codex usage 口径对齐与覆盖率可见性

**What to build:** 用户看到的 Codex token 数字与 Codex 自报一致——`input` 是非缓存输入、`cacheRate` 反映真实缓存命中、Codex 窗口的 `totalTokens` 等于上游 `usage.total_tokens`；同时当会话里存在「有 `token_count` 快照、没有 durable usage record」的物理 rollout 时，用户能明确知道这部分没有被计入，而不会把它误读成「Codex 没怎么用」。

**Blocked by:** 01: 真实命名可发现（Codex rollout 发现契约）

**Status:** ready-for-agent

- [ ] 账本 `input` = `input_tokens - cached_input_tokens`（饱和减、不为负），`cacheRead` = `cached_input_tokens`；`cacheWrite` 与 `reasoning` 继续独立成列且不参与 `totalTokens`。
- [ ] Codex 窗口（totals / sessions / groups）的 `totalTokens` 等于同一份 fixture 中上游 `usage.total_tokens` 之和；`cacheRate` 用同一份 fixture 推导正确；Pi 行为不受影响，且 Pi/Codex/All 的同名列 `input` 语义统一为非缓存输入。
- [ ] `cached_input_tokens > input_tokens`（口径异常或第三方 provider 语义不同）时产生诊断告警，数值保持非负，不静默修正。
- [ ] 无 durable usage record 但有 `token_count` 快照的物理 rollout：不计入任何窗口、不产生 usage 行，但产生 per-file 诊断并声明未计入的快照条数。
- [ ] 诊断在四个出口一致可见：CLI stderr、CLI JSON 的 meta、HTTP `/api/meta`、WebUI 诊断条。
- [ ] 上游真实存在但不产生 usage 的事件类型不再逐条告警；真正未知的类型仍逐条告警。
- [ ] 与真实语义矛盾的旧 fixture 与断言就地迁移清理：带 `Z` 的重复 fixture、`total_tokens` 与字段相加不一致的记录、以及按「input 含缓存」假设写下的数字断言。
- [ ] 幂等、半行、表示切换、坏行不阻塞等既有回归在本 ticket 后仍为绿；`all` 源合计不双算。
- [ ] 领域术语表的 Codex 计入口径词条，以及跨源 input / cacheRead 语义归一的 ADR 随实现落库（ADR-0002 的 `totalTokens` 公式不变）。
