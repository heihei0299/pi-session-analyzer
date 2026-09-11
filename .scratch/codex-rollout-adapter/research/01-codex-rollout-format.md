# Research: Codex rollout 文件格式与压缩布局

**研究日期**：2026-09-08
**上游源码快照**：`openai/codex@2cbbf0c9b542a36a1c3284b5e804917635b6f666`

## Findings

1. **逻辑记录是带时间戳的 JSONL envelope**
   - `RolloutLine` 要求顶层 JSON object，必须有 `timestamp`，可选 `ordinal`，其余字段按 `type` tagged union 解码为 `RolloutItem`。
   - 主要 item 类型包括 `session_meta`、`response_item`、`token_usage_record`、`event_msg`、`compacted`、`turn_context`、`inter_agent_communication`、`retained_context` 等。
   - 缺少 `timestamp`、顶层不是 object 或 item 无法解码时，canonical decoder 会报错；因此读取器不能把任意 JSONL 当作有效 rollout。

2. **目录与文件名有稳定的当前契约**
   - rollout store 暴露 `sessions` 与 `archived_sessions` 两个子目录常量；官方列表器会递归遍历这些目录中的合法 rollout 文件。
   - 普通文件名：`rollout-<UTC timestamp>-<thread-id>.jsonl`。
   - reverted thread 的文件名：`rollout-<UTC timestamp>-<thread-id>_<rollout-id>.jsonl`。文件名 timestamp 精确到秒；普通文件中 `thread-id` 同时作为 rollout id，revert 文件则有独立 rollout id。
   - 文件名解析只接受 canonical `.jsonl` 名称；非匹配文件应跳过。

3. **压缩是同一 rollout 的物理表示，不是另一种逻辑格式**
   - `.jsonl.zst` 是 `.jsonl` 的压缩 sibling；官方 `RolloutFile` 将物理路径与 canonical plain filename 分开。
   - 如果 plain 与 compressed sibling 同时存在，官方发现逻辑优先 plain，并跳过 compressed sibling，避免一次 rollout 被枚举两次。
   - 官方 reader 能透明读取 plain 与 zstd；压缩 rollout 在追加前会 materialize 回 plain，说明压缩文件可能经历 plain↔compressed 表示切换。
   - 当前官方 compression worker 扫描 `archived_sessions` 与 `sessions`，对满足条件的冷 rollout 做 zstd 压缩；读取器必须处理归档目录和表示切换。

4. **session metadata 是首行 item 的 payload，不应只依赖文件名**
   - `SessionMeta` 提供 `session_id`、`id`、`timestamp`、`cwd`、`originator`、`cli_version`、`source`、`model_provider` 等字段。
   - `session_id` 表示 root thread id；`id` 表示当前 thread id。revert/fork 语义可能使文件名中的 rollout id 与 thread id 不同。
   - `parent_thread_id`、`forked_from_id`、`history_base` 等关系字段属于 metadata，只有读取首个 `session_meta` 才能可靠识别。

## 对本项目的结论

- Go 适配器必须把 `.jsonl` 和 `.jsonl.zst` 视为同一逻辑 rollout，并在发现阶段实现 sibling 去重；不能简单把两个扩展名分别计数。
- 需要支持 `sessions` 与 `archived_sessions`，并以 metadata 的 thread/session identity 作为会话归属，文件名只用于发现、排序和 physical identity 辅助。
- zstd 解码是 v1 的真实依赖；不能用“遇到压缩文件就跳过”作为默认策略，否则历史统计会静默漏算。
- `RolloutLine` 顶层 timestamp/ordinal 与 item type 的错误处理仍需在设计 ticket 中决定：建议逐行容错但对缺 metadata 的文件显式跳过并报告。
- 这是对当前上游源码的兼容，不等于永久格式承诺；spec 应记录 `cli_version`，并对未知 item type 保留可诊断的 skip/error 统计。

## Primary sources

- [`rollout/src/lib.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/rollout/src/lib.rs)
- [`rollout/src/list.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/rollout/src/list.rs)
- [`rollout/src/compression.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/rollout/src/compression.rs)
- [`rollout/src/rollout_file_name.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/rollout/src/rollout_file_name.rs)
- [`protocol/src/protocol.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/protocol/src/protocol.rs)
- [`core/tests/suite/rollout_compression.rs`](https://github.com/openai/codex/blob/2cbbf0c9b542a36a1c3284b5e804917635b6f666/codex-rs/core/tests/suite/rollout_compression.rs)
