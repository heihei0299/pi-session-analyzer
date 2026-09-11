# 09: Go Codex 适配器 seam 与验收矩阵

**Type:** grilling
**Status:** resolved
**Blocked by:** 06, 08

## Question

在 source/query 契约和 zstd 依赖策略确定后，决定 Go-only 实现的最小模块边界、现有 `internal/pi` seam 的复用方式、Codex parser/sync/ledger 的职责划分，以及 fixture、failure case、cross-build 和现有 Pi 回归的验收矩阵。

目标是让后续实现只新增必要的 Codex adapter，不复制 `SessionData`、聚合或数据库查询逻辑，也不要求 TypeScript parity。


## Answer

用户确认采用以下 Go-only 实现边界：

- 新增独立 `internal/codex` adapter，复用现有 `internal/db`、`internal/domain`、`internal/timerange`、序列化和 server 查询逻辑；不先抽象通用 `internal/source`，不把 Codex 塞进 `internal/pi`。
- Codex 复用现有 `proxy_request_logs`、`session_usage_dedup`、`session_log_sync`，通过 `data_source=codex` 表达来源，不新建 Codex 专用账本。
- 允许对 Go backend 和共享 `src/webui.html` 做最小修改；不实现 TypeScript parser，也不要求 TypeScript parity。
- 使用 synthetic、脱敏 fixture，覆盖 plain/zstd、有效/缺失 usage、重复/冲突 response、半行/坏行、fork/revert、unknown model 和 reroute。
- 验收包括 focused Go tests、`go test -v ./...`、`go build`，以及 CI/发布前的 Linux/macOS/Windows release cross-build；现有 TypeScript 测试只作为未回归检查。

路线已清晰，destination spec 已生成于 [`Codex rollout 数据源适配 spec`](../spec.md)。
