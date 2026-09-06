# ADR-0003 — 存储与口径对齐 cc-switch（SQLite 直切，无兼容）

- **状态**: Accepted（2026-09-03，直切）
- **影响组件**: `src/db.ts` / `src/pi-discovery.ts` / `src/pi-parse.ts` / `src/pi-identity.ts` / `src/pi-sync.ts` / `src/cost/calculator.ts` / `src/db-aggregation.ts` / `src/api.ts` / `src/webui.html` / `internal/db` / `internal/pi` / `CONTEXT.md`

## 背景

token-analyzer 原为无库文件扫描：仅 `assistant` 单载体、瞬时 `Map` 缓存、与 cc-switch（`session_usage_pi.rs` + `schema.rs`）的四载体落库 + 持久账本 + 指纹增量相比，workflow/子 agent 系统性低估约 4%，且文件重写/同尺寸替换/半行残段会双算。本次按用户要求“存储也与 cc-switch 对齐”且“不用考虑过往兼容”直切。

## 决策

- **存储**：`token-analyzer.db` 5表与 `cc-switch/schema.rs` 列定义 1:1（`proxy_request_logs/session_log_sync/session_usage_dedup/usage_daily_rollups/model_pricing`），`SCHEMA_VERSION=1`，`WAL/foreign_keys/auto_vacuum INCREMENTAL`，路径 `TOKEN_ANALYZER_DB > --db > ~/.cache/token-analyzer/token-analyzer.db`，`sync_pi_usage` 为唯一写入路径，读路径 `proxy_request_logs ∪ rollups` 的 SQL 聚合。
- **口径**：`assistant|toolResult|compaction|branch_summary` 四载体，门控 `has_billable||has_cost||failed`，`provider/requestModel/responseModel` 三字段，`totalTokens/cacheRate` 公式不变但输入为四载体和。
- **发现**：`PI_CODING_AGENT_SESSION_DIR` 环境变量 > `pi native defaults` > `~/.pi/agent/sessions`，`Flat` vs `ProjectDirectories` 双布局。
- **去重/增量**：`request_id/semantic_id` 双账本（`hash_field/hash_json` 规范化），同文件 `output` 最大覆盖，`PiFileRevision{modifiedMs,fileSize,tailFingerprint(末4096B),complete}` 的 `tail` 指纹校验。
- **引擎**：Node `node:sqlite:DatabaseSync`（`engines.node>=24`），Go `modernc.org/sqlite` 纯 Go（`CGO_ENABLED=0` 交叉编译不受影响）。

## 后果

**正面**
- 与 cc-switch 口径与存储完全一致，workflow/子 agent 不再低估，重写/半行场景不双算。

**负面/约束**
- `engines.node` 抬至 `>=24`（breaking），`go.mod` 新增 `modernc.org/sqlite` 唯一外部依赖，`data/opencode` JSON 与旧文件聚合回退直接废弃，首启空库即空结果。

## 备选方案（未采纳）

- **保留兼容分支**：文件回退 + 旧口径双轨，增加复杂度且与用户“不用考虑过往兼容”相悖。
- **better-sqlite3 / mattn/go-sqlite3**：前者需原生编译（`node-gyp`），后者需 `CGO`，均破坏现有零原生/纯 Go 交叉编译约束。
