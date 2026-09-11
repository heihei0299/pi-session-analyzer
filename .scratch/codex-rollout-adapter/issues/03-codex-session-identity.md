# 03: Codex session 与 child thread 身份关系

**Type:** research
**Status:** resolved
**Blocked by:** None
**Research artifact:** `../research/03-codex-session-identity.md`

## Question

基于 Codex 官方源码，确认 rollout 中 session/thread 的稳定身份字段、cwd、model provider、origin/source、父子 thread 或 child-agent 关系，以及同一逻辑会话跨文件/跨重试时的识别方式。

研究输出应足以决定 v1 是“每个 rollout 独立一行”，还是需要像 Pi 子代理会话一样做 parent 聚合；若官方没有稳定关系字段，要明确建议保持独立而不是猜测。

## Answer

截至 2026-09-08，官方 `SessionMeta` 区分 root `session_id`、稳定的当前 `id`（thread）和物理 rollout identity。revert/fork 可让同一 thread 对应多个物理 rollout；`forked_from_id`、fork ordinal、`parent_thread_id`、`history_base`、`subagent_history_start_ordinal` 描述继承和父子关系。`thread_source`/`SessionSource` 还能标识 subagent、CLI、Exec 等来源。

不能把文件路径直接当作逻辑 session ID，也不能只按 thread ID 静默合并所有物理 rollout。v1 应保存 physical rollout identity、stable thread/session identity 和关系元数据；在 normalized ledger 规则确定前，最安全的默认是 physical rollout 独立导入、保存关系但不做隐式 parent/child 聚合，并依靠 response identity 避免重复。

研究详情见 [`Codex session 与 child thread 身份关系`](../research/03-codex-session-identity.md)。
