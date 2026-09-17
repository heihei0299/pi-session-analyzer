# 04: 让 source adapter 错误与诊断可观察

**What to build:** 当 Pi 或 Codex 源数据存在坏记录、文件竞态、定价读取问题或 ledger 写入失败时，用户可以从查询结果和运行状态判断发生了什么；有效数据仍可导入，失败同步可以安全重试。

**Blocked by:** 01: 降低 Query Engine 的变更半径；02: 建立可重放的 ledger schema migration

**Status:** resolved

- [x] 单条格式错误可以跳过，但会累积明确的诊断，不会与“没有 usage”混淆。
- [x] 文件级读取、revision 或解析失败不会伪装成完整成功，也不会推进该文件的 cursor。
- [x] Pi/Codex 的 dedup、usage、session metadata、diagnostics 和 cursor 仍在真实事务中原子提交。
- [x] 任一 ledger mutation 或 commit 失败都会 rollback，并保留可重试的旧 snapshot。
- [x] 定价表读取失败、模型未配置价格和 Codex `unpriced` 状态能够区分。
- [x] 诊断能够经 QueryMeta、CLI 输出和 HTTP/WebUI 状态观察，并在 cursor 命中时正确重放。
- [x] Refresh 失败后重试不会丢失有效记录、推进错误 cursor 或产生重复账。
- [x] 测试覆盖 malformed record、文件读取失败、mutation 失败、cursor 失败、重试和幂等。

## Implementation summary

- Added source-tagged Pi diagnostics with persisted per-file summaries, cursor-hit replay, malformed-record warnings, pricing-table failure propagation, and missing-model price warnings.
- Made Pi/Codex revision, parse, mutation, and commit failures observable; failed files do not advance cursors and can be retried.
- Made Codex diagnostics load/persist errors return to Query/Refresh, and clear stale plain/zstd cursor summaries after a successful representation switch.
- Kept usage, deduplication, session metadata, diagnostics, and cursor writes in the existing per-file transactions; existing rollback tests remain green.
- Tests: `TOKEN_ANALYZER_DB= GOMAXPROCS=2 go test -p 1 ./internal/pi ./internal/codex ./internal/query ./internal/server ./cmd/token-analyzer ./internal/refresh`; `go vet` on the affected packages (pass).
- Commit: deferred to the single request-level feature commit by repository policy.
