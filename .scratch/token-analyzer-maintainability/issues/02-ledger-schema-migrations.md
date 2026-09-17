# 02: 建立可重放的 ledger schema migration

**What to build:** 让新安装和已有数据库都能安全到达同一个 normalized SQLite ledger schema；数据库升级失败时，用户不会得到一个无法判断或无法继续使用的半迁移数据库。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] 新数据库使用最新 schema 创建，受支持的旧版本数据库可以按顺序升级。
- [x] 每个 schema migration 都有明确版本、执行顺序和事务边界。
- [x] migration 只有在全部 DDL、索引和约束成功后才推进 user version。
- [x] migration 失败会回滚，重复打开或重复执行不会重复添加列、索引或数据。
- [x] 迁移后的数据库可以正常执行 Refresh、Query、rollup/prune 和只读查询。
- [x] 数据库元信息能够暴露最终 schema version，且与实际 schema 一致。
- [x] 测试覆盖 fresh database、受支持旧版本、重复升级和失败回滚。
- [x] 不改变既有 raw ledger、daily rollup、ownership predicate 和查询能力契约。

## Implementation summary

- Replaced out-of-transaction table/column repair with an explicit v2 migration and transactional latest-schema reconciliation.
- `user_version` advances only after the migration DDL and indexes succeed; failed upgrades roll back and can be retried.
- Preserved v1 rows and the existing normalized ledger schema, including `source_sessions`, Pi session metadata, rollups, and the physical request index.
- Behaviors: fresh creation, supported v1 upgrade, repeated open, failure rollback, retry, and read/query compatibility are covered.
- Tests: `GOMAXPROCS=2 go test -p 1 ./internal/db ./internal/query`; `go vet ./internal/db ./internal/query` (pass).
- Commit: deferred to the single request-level feature commit by repository policy.
