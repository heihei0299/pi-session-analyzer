# Research: Codex response model 归属

**研究日期**：2026-09-08
**上游源码快照**：`openai/codex@2cbbf0c9b542a36a1c3284b5e804917635b6f666`

## Findings

1. `TokenUsageRecord` 保存 `thread_id`、`turn_id`、`session_id`、`root_turn_id`、`response_id` 和三层 usage，但没有 model/provider 字段。
2. `TurnContextItem` 保存该 turn 的请求 model，可以通过 `turn_id` 做 response/turn 级关联；它不直接提供 provider。
3. `SessionMeta.model_provider` 是 rollout/session 级 provider。`ThreadSettingsApplied.thread_settings` 能记录设置变化中的 model 与 `model_provider_id`，但需要按 rollout 顺序重放才可作为时间上下文。
4. `EventMsg::ModelReroute` 保存 `from_model`、`to_model` 和原因，但没有 response ID 或 turn ID；仅凭它不能把 reroute 后的 model 可靠归属到某个 `TokenUsageRecord`。
5. `RawResponseCompleted` 有 response ID 与 usage，但没有 model/provider，因此不能替代 turn context 做 model attribution。

## 对本项目的结论

- 落库时保持“response/turn 优先，session metadata 回退，最后 `unknown`”规则，但只有存在明确 `turn_id` 关联时才使用 turn model。
- provider v1 以 session metadata 为主；设置变化只能在能按 ordinal/timestamp 明确重放时使用，否则保留 session provider，不猜测 response provider。
- Model reroute 只作为诊断/metadata，不把 `to_model` 强行写到具体 usage record。
- `TokenUsageRecord` 的 response identity 与 model attribution 分离：即使 model 不可知，也必须照常计入 token usage。

## Primary sources

- [`protocol/src/protocol.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/protocol/src/protocol.rs)
- [`history/src/rollout_payload.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/history/src/rollout_payload.rs)
- [`core/src/session/turn.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/core/src/session/turn.rs)
- [`app-server/src/request_processors/token_usage_replay.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/app-server/src/request_processors/token_usage_replay.rs)
