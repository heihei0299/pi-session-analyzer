# 01: Go normalized ledger 与 Codex rollout 导入

**What to build:** 用户可以在保持现有 Pi 默认行为不变的前提下，选择 Codex 数据源并查看 Codex 的 totals 与 sessions。Codex 的持久化 rollout 会被识别、解析并写入现有 normalized ledger；结果可从 Go CLI、HTTP API 和总览/会话页面观察到，cost 明确显示为 `unpriced`。

**Blocked by:** None (can start immediately)

**Status:** resolved

## Acceptance criteria

- [x] 现有 Pi CLI、HTTP API 和 WebUI 在既有 synthetic fixture 上保持原有结果。
- [x] `--source codex` 能读取 canonical plain rollout，并发现 `sessions` 与 `archived_sessions` 下的记录。
- [x] `.jsonl.zst` rollout 能被读取；plain/compressed sibling 不会在基础导入中产生两份结果。
- [x] `session_meta`、`TokenUsageRecord.usage`、cwd、model/provider、thread/session identity 和 parent/fork metadata 正确映射。
- [x] 每个 `source + response_id` 只产生一条 usage 记录；累计 snapshot 不被重复计入。
- [x] Codex totals 与 sessions 返回 token、response count 和 `unpriced` cost；无可靠 usage 的 response 不产生 usage row。
- [x] Codex model attribution 遵循 turn model → session metadata → `unknown` 的回退顺序。
- [x] 添加 focused Go tests，覆盖最小 plain/zstd rollout、usage 映射、identity 去重和 Pi 回归。

## Implementation summary

- Added `internal/codex` discovery, plain/zstd parser, normalized ledger sync, response identity dedup, diagnostics, and metadata mapping.
- Reused `SessionData` as the query boundary through `QueryFiles`; added Go CLI/API source selection and Codex/all requests rejection.
- Added focused parser, zstd, dedup, half-line, sibling-switch, query, server, render, CSV, and Pi regression coverage.
- WebUI source selector remains Issue 02 scope; `serve --source codex` exposes Codex totals/sessions through the existing page.
