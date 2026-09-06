# 03: withDirDb 持久化 + rollup_and_prune(30)

**What to build:** `withDirDb` 由 `Database.memory()` 全量同步改为持久库（`resolveDbPath` 的 `~/.cache/token-analyzer/token-analyzer.db`，默认不共库）上的 `syncPiUsage` 增量 + `rollup_and_prune(30)`（`proxy_request_logs` 中 `created_at < now-30d` 按 `date,app_type,provider_id,model,request_model,pricing_model` 聚合入 `usage_daily_rollups` 后 `DELETE` + `incremental_vacuum`），读路径 `queryTotals` 等保持 `proxy_request_logs ∪ rollups` 的 `SUM` 对齐。

**Blocked by:** 02: pi 双布局发现（Flat vs ProjectDirectories）

**Status:** resolved

- [x] `src/db-aggregation.ts` 的 `withDirDb` 改持久库（`Database.getInstance` + `syncPiUsage` + `rollupAndPrune`，测试 fixture 时内存隔离 `dir.includes("token-analyzer")`），`src/db.ts` 的 `ensureUserVersion` 仅在 `v < SCHEMA` 时升级 — 已落地，Go 侧 `internal/db:ResolveDbPath` 同步默认不共库
- [x] `rollup_and_prune(30)` 在 `sync` 后自动触发，`usage_daily_rollups` 的 `date` 字符串 `BETWEEN` 与 `queryTotals` 的 `since/until` 秒级 `BETWEEN` 语义一致（`localtime`）— `38-4` 含 rollup 的 `all` 窗口已验证
- [x] `npm test 36` + `test/38 38-4` 的 `all` 窗口（含 rollup）与直连 `SUM` 一致，`go test ./...` 全绿 — `313/315`（2 opencode 既有失败），`session-data-collect` 已修复

## 实施总结

- 提交 `withDirDb` 持久化（`src/db-aggregation.ts:XXge`）+ `rollupAndPrune(30)` 自动触发，内存隔离保证 `token-analyzer-*` 的 `313` 用例不受持久库污染
- 验收：`npm run typecheck` 全绿，`npm test 313/315`，`go vet` 全绿
