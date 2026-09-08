# 03: Codex 适配发布验收

**What to build:** Codex 数据源适配达到可发布状态：维护一组不含真实会话和 secrets 的 synthetic fixture，验证 Go CLI/API/WebUI 的完整行为、Pi 回归和跨平台 release 构建；后续实现者可以用固定命令判断这项适配是否交付完成。

**Blocked by:** 02: 多 source 查询体验与可靠性闭环

**Status:** resolved

## Acceptance criteria

- [x] synthetic fixture 覆盖 plain/zstd、archive、usage 缺失、重复/冲突 response、半行/坏行、fork/revert、unknown model 和 reroute。
- [x] `go test -v ./...` 通过，包含 Codex focused tests、API/UI 集成测试和既有 Pi regression tests。
- [x] `go build ./cmd/token-analyzer` 通过。
- [x] Linux amd64/arm64、macOS amd64/arm64、Windows amd64 release cross-build 通过。
- [x] Go dependency、`go.mod`/`go.sum`、diagnostics 文案和 CLI/API 帮助文本与 Codex spec 一致。
- [x] 确认没有读取或提交真实 Codex/Pi session、local database、token、`.env` 或其他 secret。

## Implementation summary

- Added synthetic Codex home fixtures for sessions/archive metadata and usage cases; zstd/half-line cases remain generated in focused Go tests.
- Updated README and CLI help-aligned usage docs for `--source`, `--codex-dir`, normalized ledger, zstd, diagnostics, and `unpriced` cost.
- Verified `go test -v ./...`, `go build ./cmd/token-analyzer`, `make all`, and Linux/macOS/Windows amd64/arm64 release cross-builds.
- No real session files, local databases, tokens, `.env`, or secrets were added.
