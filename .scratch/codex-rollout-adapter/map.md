# Codex rollout 数据源适配 — Map

**Map id**: `codex-rollout-adapter` — see `docs/agents/issue-tracker.md` for tracker conventions.

## Destination

产出一份可交接的 Codex 数据源适配 spec（`.scratch/codex-rollout-adapter/spec.md`）：规定 `token-analyzer` 如何读取持久化 Codex CLI rollout，将可靠的 token usage 接入现有 SQLite、CLI、API 与展示层，同时保持 Pi 统计兼容。

终点不是本 effort 的实现代码，而是：输入格式、计数口径、会话归属、增量/去重、source 选择和 Go 实现验收边界都已明确，后续实现无需再做产品决策。

## Notes

- **Domain**: token-analyzer 多数据源 token usage；现有 Pi 统计、SQLite ledger、CLI/API/WebUI 查询面。
- **Canonical terms**: `Codex rollout` 指 Codex CLI 的持久化运行记录；当前 `CONTEXT.md` 的“会话文件”仍专指 Pi JSONL，暂不扩义。
- **已定基线**：
  - v1 只读取持久化 Codex rollout，不接 `codex exec --json` / `--ephemeral` stdout；
  - `.jsonl` 与 `.jsonl.zst` 都属于 v1 输入；
  - Pi / Codex 默认分源展示，`all` 才显式合计；
  - v1 只保证 `totals` + `sessions`，不提供 Codex `requests` 窗口；
  - CLI/API 使用显式 `--source pi|codex|all` 与 `--codex-dir`，默认行为保持 Pi 兼容；
  - cost 不估算，Codex cost 为 `unavailable/unpriced`；
  - 本 effort 只实现 Go，TypeScript 不跟进。
- **Skills**: `research`（Codex 上游格式与语义）、`grilling` / `domain-modeling`（用户决策与术语）、`codebase-design`（normalized ledger seam）、`ponytail`（最小实现）。
- **Standing preference**: 中文沟通；不改代码，先形成 spec；保持 Pi 既有口径和默认 CLI 兼容。
- **Charting date**: 2026-09-08。

## Decisions so far

<!-- Charting baseline is recorded in Notes. Closed ticket decisions are indexed here. -->

- [Codex rollout 文件格式与压缩布局](issues/01-codex-rollout-format.md)：rollout 递归覆盖 `sessions` / `archived_sessions`；`.jsonl` 与 `.jsonl.zst` 是同一物理记录的两种表示，plain 优先，Go v1 必须支持 zstd。
- [Codex usage snapshot 计数语义](issues/02-codex-usage-semantics.md)：v1 以每个 `TokenUsageRecord.usage` 一次计入，累计 usage 只作校验，缺 usage 不推算；requests 保持 out of scope。
- [Codex session 与 child thread 身份关系](issues/03-codex-session-identity.md)：保存 physical rollout 与 stable thread/session 两套 identity 及 parent/fork 元数据，v1 不做隐式 parent/child 聚合。
- [Codex usage 接入 normalized ledger 的模型](issues/04-normalized-ledger.md)：每个 `response_id` 只计一次；只接收可靠 `TokenUsageRecord.usage`；cost 使用 `unpriced` sentinel；model/provider 保留最佳可得归属。
- [Codex response model 归属研究](issues/07-codex-model-attribution.md)：usage record 不含 model/provider；turn model 只在明确关联时使用，provider 默认按 session metadata，reroute 不强行归属。
- [Go zstd reader 依赖与兼容策略研究](issues/08-go-zstd-compatibility.md)：标准库、CGO wrapper 和外部命令均不合适；采用 Go 1.23 兼容版本线的纯 Go streaming decoder，不引入 seekable format。
- [Codex rollout 增量、重写与去重策略](issues/05-incremental-dedup-policy.md)：revision 变化全量重扫，response identity 幂等；plain/zstd 切换不改源文件；坏行可诊断跳过；usage 与 cursor 同事务提交。
- [Codex source selector 与查询表面契约](issues/06-source-query-surface.md)：默认 Pi，显式 `pi|codex|all`；独立 Codex 目录；复用 WebUI；空/坏数据可诊断；Codex requests 明确拒绝。
- [Go Codex 适配器 seam 与验收矩阵](issues/09-go-implementation-acceptance.md)：独立 `internal/codex`、复用现有 ledger/query、Go-only 加最小 WebUI、synthetic fixture 和跨平台验收。

## Not yet specified

<!-- Route is clear; destination spec is delivered at `spec.md`. -->

## Out of scope

- Codex `exec --json` 管道和 `--ephemeral` 无文件运行记录；
- Codex `requests` 逐请求窗口；
- ChatGPT/Codex 订阅实际扣费、OpenAI 账单 API 或 cost 估算；
- TypeScript 适配与双运行时 parity；
- Codex 与 Pi 的 token 优化建议、模型选择建议或提示词改造。
