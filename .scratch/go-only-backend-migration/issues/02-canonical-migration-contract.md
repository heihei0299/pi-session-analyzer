# 02: 冻结 Go-only 迁移 canonical contract

**What to build:** 在生产路径切换前，用 synthetic fixtures 与 golden expected 固化 Pi/Codex 当前已接受行为，使后续每个 Go migration slice 都能证明行为没有漂移。

**Blocked by:** 01: 将 OpenCode 抽离为 standalone opencode-analyzer 项目.

**Status:** resolved

- [x] Pi contract 覆盖四载体、failed/aborted、fork/nested fork、task、requestId/semanticId 去重、append、partial line、truncate、rewrite、mixed model、pricing。
- [x] Codex contract 覆盖 durable usage、cached input、cached > input、snapshot-only、diagnostics replay、plain/zstd sibling、revert、archive、非 canonical 输入。
- [x] golden expected 对 totals、sessions、支持的 requests、groups、period、meta/diagnostics 做字段级断言，而非只比较 row count。
- [x] cost 使用明确容差，排序有稳定 tie-breaker。
- [x] TypeScript 只作为迁移期 oracle，不新增生产能力。
- [x] fixture 全部为合成数据，不含真实 session、rollout、DB 或凭据。
- [x] OpenCode 不进入 canonical usage contract；此时 OpenCode 已位于独立项目边界。

## Answer

- Pi：`testdata/canonical/pi` synthetic fixture + `testdata/canonical/expected/pi-api.json` + `test/42-canonical-pi-contract.test.ts` 对完整 API golden 做字段级 contract。
- Codex：`internal/query/canonical_contract_test.go` + `internal/query/testdata/canonical-codex-query.json` 对 totals/sessions/groups/period/meta 做 golden，含 diagnostics markers、requests unsupported、ledger-only totals。
- 增量幂等：`TestSyncIncrementalLifecycle` 覆盖 append/partial/truncate/rewrite，Sync error 显式断言。
- Pi golden 含 `aborted` 全零计入；pricing 回算由既有 `test/35-cost.test.ts` 覆盖，不重复进 canonical golden。
- Codex golden 开关已删除：`canonical-codex-query.json` 为固定 expected，缺失即失败。
- 验证：`GOMAXPROCS=2 go test -p 1 ./...` 73 passed；`pnpm node --test --test-concurrency=1` 217 passed；`tsc --noEmit` 通过。
