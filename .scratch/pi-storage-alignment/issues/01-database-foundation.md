# 01: SQLite 持久化基座（直切，无兼容）

**What to build:** 引入 `token-analyzer.db` 为唯一真相源（`node:sqlite:DatabaseSync` + `modernc.org/sqlite`），一次性建齐 5 表与索引（与 `cc-switch/database/schema.rs` 列定义 1:1），`SCHEMA_VERSION=1`，`PRAGMA journal_mode=WAL, foreign_keys=ON, auto_vacuum=INCREMENTAL, busy_timeout=5000`，路径优先级 `TOKEN_ANALYZER_DB > --db > ~/.cache/token-analyzer/token-analyzer.db`（不可写则 `data/token-analyzer.db`），幂等 `create_tables` 与 `user_version` 校验，启动 `rollup_and_prune(30)` 入口占位。

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] `Database.getInstance(dbPath)` 单例，`DatabaseSync`（Node）/ `modernc.org/sqlite`（Go）共享同一文件，WAL/busy_timeout 生效
- [ ] `create_tables` 建 `proxy_request_logs`（含 `pricing_model/input_token_semantics`，6 索引）、`session_log_sync`（含 `last_byte_offset/last_tail_fingerprint`）、`session_usage_dedup`+`idx_semantic`、`usage_daily_rollups`、`model_pricing`，与 `schema.rs` 列定义逐字一致
- [ ] `user_version=1` 校验与 `SAVEPOINT` 保护，`apply_schema_migrations` 幂等二次调用不报错
- [ ] 路径解析：`TOKEN_ANALYZER_DB` 环境变量优先，`--db` 次之，`~/.cache` 不可写回退 `data/`，`engines.node` 抬至 `>=24`
- [ ] `go.mod` 新增 `modernc.org/sqlite`，`make release` 交叉编译不受影响（`CGO_ENABLED=0`）
- [ ] 启动 `rollup_and_prune(30)` 入口（空实现占位，T7 实现完整逻辑）
