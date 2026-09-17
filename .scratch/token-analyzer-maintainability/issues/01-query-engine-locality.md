# 01: 降低 Query Engine 的变更半径

**What to build:** 让维护者可以在不重新理解整个统计引擎的情况下修改某一种 ledger 读取、rollup 或视图行为，同时保证用户从 CLI、HTTP、WebUI 和导出看到的统计结果完全不变。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] Query Engine 的私有实现按 ledger 读取、rollup、Pi/Codex/All 视图、行构建和分页等职责局部化。
- [x] 公共 Query interface、QueryResult、领域统计口径和 source capability 不发生行为变化。
- [x] canonical Pi/Codex/All contract 的结果与拆分前一致。
- [x] Query 继续只读已提交 ledger，不执行 discovery、parse、Refresh、cursor 更新或其他写入。
- [x] 不建立第二套聚合逻辑，不引入 ORM、通用 factory、依赖注入框架或新的 runtime 依赖。
- [x] 变更完成后，重复查询、raw/rollup 视图限制和既有回归测试仍然成立。

---

This ticket is a behavior-preserving private refactor. The OpenCode Analyzer migration-out cleanup is already handled separately in the current working tree and is not part of this ticket.

## Implementation summary

- Split the private Query Engine into ledger core/read/rollup, row builders, Pi, Codex, and All implementation files without changing the public query seam.
- Corrected the Codex contract fixture to valid JSON and isolated query tests from the developer's `TOKEN_ANALYZER_DB` environment override.
- Behaviors: canonical source/query outputs preserved; Query remains read-only and ledger-only; raw/rollup view restrictions and repeated-query behavior remain covered.
- Tests: `GOMAXPROCS=2 go test -p 1 ./internal/query`; `GOMAXPROCS=2 go vet ./internal/query`.
- Commit: deferred to the single request-level feature commit by repository policy.
