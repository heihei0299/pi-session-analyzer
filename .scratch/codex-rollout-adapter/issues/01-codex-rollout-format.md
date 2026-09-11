# 01: Codex rollout 文件格式与压缩布局

**Type:** research
**Status:** resolved
**Blocked by:** None
**Research artifact:** `../research/01-codex-rollout-format.md`

## Question

基于 Codex 官方文档与官方源码，确认持久化 rollout 的发现路径、文件命名、普通与压缩扩展名、核心 event 类型、session metadata 字段，以及不同 Codex 版本之间会影响读取器的格式差异。

研究必须只使用 primary sources，并明确区分稳定契约、当前实现细节和无法保证的行为。输出应足以决定 Go 读取器是否需要额外的压缩解码能力，以及遇到未知 event/版本时的处理策略。

## Answer

截至 2026-09-08，基于 `openai/codex@2cbbf0c9b542a36a1c3284b5e804917635b6f666` 的官方源码确认：rollout 是带顶层 `timestamp`（可选 `ordinal`）和 tagged `type` 的 JSONL；逻辑目录包括 `sessions` 与 `archived_sessions`；canonical 文件名为 `rollout-<timestamp>-<thread-id>.jsonl`，revert 可追加独立 `rollout-id`；`.jsonl.zst` 是同一 rollout 的压缩 sibling，plain 与 compressed 同时存在时 plain 优先。

`SessionMeta` 提供 session/thread、cwd、source、model provider、父子和 fork 元数据。适配器必须递归读取两个目录，把 plain/compressed 视作同一 physical rollout，并在发现阶段去重；Go v1 必须具备 zstd 解码能力。未知文件应跳过，缺少 envelope timestamp 或 metadata 的记录/文件应可诊断地跳过，而不是静默计入。

研究详情见 [`Codex rollout 文件格式与压缩布局`](../research/01-codex-rollout-format.md)。
