# Research: Codex session 与 child thread 身份关系

**研究日期**：2026-09-08
**上游源码快照**：`openai/codex@2cbbf0c9b542a36a1c3284b5e804917635b6f666`

## Findings

1. **session 与 thread 是两个相关但不同的 identity**
   - `SessionMeta.session_id` 是 root thread 的 ID。
   - `SessionMeta.id` 是当前 thread ID；源码注释说明 revert 后 thread ID 仍可保持稳定，而物理 rollout 文件可以拥有不同的 rollout ID。
   - 普通文件名使用同一个 UUID 同时表示 thread/rollout；revert 文件使用 `thread-id_rollout-id` 形式区分稳定 thread 与物理 rollout。

2. **metadata 提供了 parent/fork/sub-agent 关系**
   - `forked_from_id` 和 `forked_from_ordinal_exclusive` 描述 fork 边界。
   - `parent_thread_id` 描述直接控制/创建父 thread。
   - `history_base` 描述从另一分页 rollout 继承的 exclusive history prefix；`subagent_history_start_ordinal` 描述 child 自有投影的起始 ordinal。
   - `thread_source` 可为 `user`、`subagent`、`guardian_review`、feature 等；`SessionSource` 还区分 `Cli`、`VSCode`、`Exec`、`Mcp`、`SubAgent` 等。

3. **child-agent metadata 不是只有 parent id**
   - `SessionSource::SubAgent(SubAgentSource::ThreadSpawn { ... })` 可携带 `parent_thread_id`、depth、agent path、nickname、agent role。
   - `SessionMeta` 也有 `agent_nickname`、`agent_role`、`agent_path`，因此 UI 可区分 child-agent，但这些字段不应被当成 token identity。

4. **同一 thread 可能对应多个物理 rollout**
   - canonical filename 对 reverted thread 明确允许独立 rollout id；官方 list/metadata 代码同时处理 thread id 与 rollout id。
   - 因此以文件路径作为唯一会话 ID 会把同一逻辑 thread 的不同 rollout 错拆；只以 thread id 合并又可能把不同物理历史/重放记录错误叠加。
   - `history_base`、fork ordinal 和 `TokenUsageRecord.response_id` 是区分继承前缀与新增消耗的关键候选字段。

## 对本项目的结论

- v1 的 normalized ledger 至少要保存：物理 rollout identity、稳定 thread/session identity、parent/fork metadata、cwd、source、model provider、cli version。
- 不能直接沿用 Pi 的“一个文件 = 一个独立会话”规则；Codex 的 revert/fork/paginated history 需要在下一张 grilling ticket 中决定是按 physical rollout 统计，还是按 stable thread 去重。
- child-agent 是否并入父 thread 不能仅凭 `parent_thread_id` 自动决定；应先区分“继承历史”与“新增 response usage”，并避免把 child 的 response 重复算入 parent。
- 在没有完成这项设计前，最安全的默认是：每个可识别的 physical rollout 独立导入，保存 parent/fork 关系但不做隐式聚合；`all` 只合并已确认不重复的 ledger records。

## Primary sources

- [`protocol/src/protocol.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/protocol/src/protocol.rs)
- [`rollout/src/rollout_file_name.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/rollout/src/rollout_file_name.rs)
- [`rollout/src/metadata.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/rollout/src/metadata.rs)
- [`rollout/src/list.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/rollout/src/list.rs)
- [`core/src/session/rollout_reconstruction.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/core/src/session/rollout_reconstruction.rs)
- [`core/tests/suite/rollout_compression.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/core/tests/suite/rollout_compression.rs)
