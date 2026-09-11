# Research: Codex usage snapshot 计数语义

**研究日期**：2026-09-08
**上游源码快照**：`openai/codex@2cbbf0c9b542a36a1c3284b5e804917635b6f666`

## Findings

1. **usage 有三层粒度，不能把所有字段混为一谈**
   - `TokenUsage` 字段包括 `input_tokens`、`cached_input_tokens`、`cache_write_input_tokens`、`output_tokens`、`reasoning_output_tokens`、`total_tokens`，另有 provider budget units。
   - `TokenUsageInfo` 同时包含 `total_token_usage` 与 `last_token_usage`；`TokenCountEvent` 的注释明确它是当前 session 的 usage update，缺失 usage 时 `info` 可以为 `None`。
   - `TokenUsageRecord` 则绑定 `thread_id`、`turn_id`、`session_id`、`root_turn_id`、`response_id`，并携带单个 response 的 `usage`、该 turn 的累计 `turn_token_usage`、该 thread 的累计 `thread_token_usage`。

2. **`total_token_usage` 是累计值，`last_token_usage` 是增量/最近响应值**
   - `TokenUsageInfo::append_last_usage` 将新的 `last` 加入 `total_token_usage`，并用该 `last` 覆盖旧的 `last_token_usage`。
   - 因此从 `TokenCountEvent.info.total_token_usage` 统计 totals 时，必须按快照增量或最终快照处理；不能逐条相加累计字段。
   - 若消费 `TokenUsageRecord.usage`，它代表一个已观察到的 upstream response 的 usage；`turn_token_usage` 与 `thread_token_usage` 只能作为累计校验/展示，不能再次加入账本。

3. **官方测试确认 response-level record 的行为**
   - `token_usage_rollout.rs` 构造三个有 usage 的 response 和一个没有 usage 的 response。
   - rollout 中只产生三个 `TokenUsageRecord`；同一 turn 的两个 response 的 `turn_token_usage` 变为 120→200，新 turn 的值重置为 30，而 `thread_token_usage` 继续为 230。
   - 每条 record 的 `response_id` 可作为 response-level identity；相同 turn 不代表相同 response。

4. **无 usage 的响应不应被推算为 0 token 请求**
   - 官方测试明确没有 usage 的 completed response 不会产生 `TokenUsageRecord`。
   - 这支持 v1 只统计有可靠 usage 的记录，并把缺失 usage 作为 unavailable/skip 诊断，而不是根据输出文本或 turn 数量估算。

5. **TokenCount 与 TokenUsageRecord 的关系需要在适配器中固定一种主账本**
   - `EventMsg::TokenCount` 面向 session/UI 更新，包含 session totals 与最近一次 usage。
   - `RolloutItem::TokenUsageRecord` 面向 durable response usage，包含 response/turn/thread 三种粒度。
   - 两者同时存在时不能相加，否则会双算。当前源码的 response-level durable record 更适合作为 v1 ledger 输入；TokenCount 可用于兼容旧 rollout 或校验，但这需要实现 ticket 明确。

## 对本项目的结论

- totals 的主计数应使用每个 `TokenUsageRecord.usage` 一次，按 `response_id` 做 identity；`turn_token_usage` / `thread_token_usage` 不进入加总。
- 对缺少 `TokenUsageRecord` 的旧/异常 rollout，不能自动用 TokenCount 的累计字段与 response records 混加；应设计明确的 fallback 和互斥优先级。
- 同一 response 的重复 `TokenUsageRecord` 必须幂等；文件重写或压缩切换不能导致 record 二次导入。
- `requests` v1 保持 out of scope 是合理的：虽然新格式存在 response-level record，但旧版本和异常响应的覆盖不一定一致，且 request 的用户可见边界仍需单独定义。
- cost 不从这些字段推导；`codex_rollout_budget_units` 是 provider budget 单位，不是美元。

## Primary sources

- [`protocol/src/protocol.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/protocol/src/protocol.rs)
- [`history/src/rollout_payload.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/history/src/rollout_payload.rs)
- [`core/src/session/turn.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/core/src/session/turn.rs)
- [`core/tests/suite/token_usage_rollout.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/core/tests/suite/token_usage_rollout.rs)
- [`app-server/src/request_processors/token_usage_replay.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/app-server/src/request_processors/token_usage_replay.rs)
