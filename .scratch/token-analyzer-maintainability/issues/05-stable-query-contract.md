# 05: 锁定 Query 的 rollup、排序与分页契约

**What to build:** 让同一份 ledger 在 CLI、HTTP、WebUI 和导出中始终得到一致、可重复的结果；历史 rollup 不会漏算或双算，分页和自动刷新不会因不确定排序而漂移。

**Blocked by:** 01: 降低 Query Engine 的变更半径；02: 建立可重放的 ledger schema migration

**Status:** resolved

- [x] totals、period 和可安全按 model 聚合的 groups 正确合并 raw ledger 与 daily rollup。
- [x] sessions、requests、cwd groups 和 detail 不从缺少 identity 的 rollup 伪造 session/cwd/request 信息。
- [x] partial-day MessageTimeRange 继续只使用安全的完整 rollup，并暴露 `coverageStatus=partial` 和 warning。
- [x] Pi、Codex、All 的统计、cost 状态和 source capability 与领域术语表及 canonical golden 一致。
- [x] 默认排序和用户指定排序都有显式、稳定的 tie-breaker；相同排序 key 的行不会互相“优于”对方。
- [x] 服务端分页在重复查询、刷新、导出和排序切换时保持稳定的 total、page 和 row 顺序。
- [x] HTTP/WebUI/CLI/JSON/CSV 对同一筛选条件的结果保持同一统计口径。
- [x] 测试覆盖 raw+rollup、raw-only view、partial coverage、默认排序、相等 key、分页和跨源合并。

## Implementation summary

- Added one query validation path for view capabilities and stabilized default sessions/requests ordering as `timestamp desc` before pagination.
- Kept explicit sort keys and directions on the existing public rows, added deterministic group-row ordering, and preserved raw-only identity views plus raw/rollup aggregate rules.
- Added a public-seam regression for default ordering, equal timestamps, and repeated paginated reads.
- Tests: `TOKEN_ANALYZER_DB= GOMAXPROCS=2 go test -p 1 ./internal/query ./internal/server ./cmd/token-analyzer ./internal/pi ./internal/codex ./internal/db`; `go vet` on the same packages (pass).
- Commit: deferred to the single request-level feature commit by repository policy.
