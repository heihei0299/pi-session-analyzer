# pi 统计与存储对齐 cc-switch — 功能规格（直切）

**状态**: draft（本 spec 直切 cc-switch，不保留旧口径/旧存储兼容；实现按此规格全新落地）
**数据域**: pi 会话 JSONL → SQLite `token-analyzer.db`（唯一真相源）

---

## 1 背景与问题

token-analyzer 现状为无库文件扫描：`session-data.ts` 仅 `assistant` 单载体、`readSessionFilesCached` 瞬时 Map、`totalTokens = input+cacheRead+output`；与 cc-switch（`session_usage_pi.rs` + `schema.rs`）的四载体落库 + 持久账本 + 指纹增量相比，workflow/子 agent 系统性低估约 4%，且文件重写/同尺寸替换/半行残段场景会双算。本 spec 按 cc-switch 源码 1:1 直切，不做兼容分支。

## 2 目标

- SQLite 为唯一真相源：`proxy_request_logs` 四载体落库 + `session_log_sync` revision 指纹 + `session_usage_dedup` 双键账本 + `usage_daily_rollups` 30 天剪枝，`sync_pi_usage` 为唯一写入路径。
- 统计口径对齐 cc-switch：`assistant|toolResult|compaction|branch_summary` 四载体，门控 `has_billable||has_cost||failed`，`provider/requestModel/responseModel` 三字段，`totalTokens/cacheRate` 公式不变但输入为四载体和。
- 发现对齐：`PI_CODING_AGENT_SESSION_DIR` 环境变量 > `pi native defaults.session_dir` > `~/.pi/agent/sessions`，`Flat` vs `ProjectDirectories` 双布局。
- 读路径全部走 `proxy_request_logs ∪ usage_daily_rollups` 的 SQL 聚合，`watch` 复用 `session_log_sync`。

## 3 非目标

- 不保留 `data/opencode/*.json`、旧文件聚合回退、Node 18 兼容、`go.mod` 零依赖声明。
- 不复刻 `providers/provider_endpoints/mcp/skills/proxy_config/profiles` 等运营表，不接 Tauri/WebDAV/S3。
- 不改 `pi-switch` 网关口径，不改 OpenCode 远端抓取逻辑。

## 4 术语（增量）

- **四载体**：`assistant`（`type=message, role=assistant`）、`toolResult`（`role=toolResult`）、`compaction`（`type=compaction`）、`branch_summary`（`type=branch_summary`），各带 `usage`。
- **双账本**：`session_usage_dedup(data_source, request_id, semantic_id, has_entry_id)`，`request_id = hash(pi-session-request-v3+kind+entry.id+timestamp)`，`semantic_id = hash(pi-session-semantic-v1+kind+entry_ts+msg_ts+provider/model/responseModel/responseId/api/toolCallId/toolName/stopReason/errorMessage/content+canonical usage)`。
- **revision 指纹**：`PiFileRevision{modified_nanos, file_size, tail_fingerprint(末4096B SHA256), complete}` 编码于 `session_log_sync.last_synced_at`，`last_line_offset` 为行游标。
- **FRESH 语义**：`input_token_semantics = FRESH`，`cacheRead` 不从 `input` 扣除。

## 5 存储

### 5.1 位置与引擎
- 路径优先级 `TOKEN_ANALYZER_DB` > `--db <path>` > `~/.cache/token-analyzer/token-analyzer.db`（`~/.cache` 不可写则 `data/token-analyzer.db`），`PRAGMA journal_mode=WAL, busy_timeout=5000, foreign_keys=ON, auto_vacuum=INCREMENTAL, synchronous=NORMAL`。
- Node `node:sqlite:DatabaseSync`（`engines.node>=24`），Go `modernc.org/sqlite` 纯 Go（`go 1.22`，`CGO_ENABLED=0` 交叉编译不受影响）。

### 5.2 Schema（SCHEMA_VERSION=1，新建库）
- `proxy_request_logs(request_id TEXT PRIMARY KEY, provider_id TEXT NOT NULL, app_type TEXT NOT NULL, model TEXT NOT NULL, request_model TEXT, pricing_model TEXT, input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0, cache_read_tokens INTEGER NOT NULL DEFAULT 0, cache_creation_tokens INTEGER NOT NULL DEFAULT 0, input_token_semantics INTEGER NOT NULL DEFAULT 0, input_cost_usd TEXT NOT NULL DEFAULT '0', output_cost_usd TEXT NOT NULL DEFAULT '0', cache_read_cost_usd TEXT NOT NULL DEFAULT '0', cache_creation_cost_usd TEXT NOT NULL DEFAULT '0', total_cost_usd TEXT NOT NULL DEFAULT '0', latency_ms INTEGER NOT NULL, first_token_ms INTEGER, duration_ms INTEGER, status_code INTEGER NOT NULL, error_message TEXT, session_id TEXT, provider_type TEXT, is_streaming INTEGER NOT NULL DEFAULT 0, cost_multiplier TEXT NOT NULL DEFAULT '1.0', created_at INTEGER NOT NULL, data_source TEXT NOT NULL)` + 6 索引（`provider/created_at/model/session/status` + usage 复合）。
- `session_log_sync(file_path TEXT PRIMARY KEY, last_modified INTEGER NOT NULL, last_line_offset INTEGER NOT NULL DEFAULT 0, last_synced_at INTEGER NOT NULL, last_byte_offset INTEGER, last_tail_fingerprint INTEGER)`。
- `session_usage_dedup(data_source TEXT NOT NULL, request_id TEXT NOT NULL, semantic_id TEXT NOT NULL, has_entry_id INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(data_source, request_id))` + `idx_semantic(data_source, semantic_id, has_entry_id)`。
- `usage_daily_rollups(date TEXT NOT NULL, app_type TEXT NOT NULL, provider_id TEXT NOT NULL, model TEXT NOT NULL, request_model TEXT NOT NULL DEFAULT '', pricing_model TEXT NOT NULL DEFAULT '', request_count INTEGER NOT NULL DEFAULT 0, success_count INTEGER NOT NULL DEFAULT 0, input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0, cache_read_tokens INTEGER NOT NULL DEFAULT 0, cache_creation_tokens INTEGER NOT NULL DEFAULT 0, input_token_semantics INTEGER NOT NULL DEFAULT 0, total_cost_usd TEXT NOT NULL DEFAULT '0', avg_latency_ms INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(date, app_type, provider_id, model, request_model, pricing_model))`。
- `model_pricing(model_id TEXT PRIMARY KEY, display_name TEXT NOT NULL, input_cost_per_million TEXT NOT NULL, output_cost_per_million TEXT NOT NULL, cache_read_cost_per_million TEXT NOT NULL DEFAULT '0', cache_creation_cost_per_million TEXT NOT NULL DEFAULT '0')`。
- `create_tables` 幂等，`apply_schema_migrations` 仅 `SAVEPOINT + user_version` 校验；启动 `rollup_and_prune(30)` + `incremental_vacuum`。

## 6 发现

- `resolvePiSessionRoot()` 按 `PI_CODING_AGENT_SESSION_DIR`（绝对路径才可枚举）> `pi native defaults` > `~/.pi/agent/sessions`，`Flat`（根下 `*.jsonl`）vs `ProjectDirectories`（`sessions/<project>/*.jsonl`）双布局；相对路径直接报错 `PI_SESSION_DIR_REQUIRES_PROJECT_CONTEXT`（`400`）。
- `collectPiJsonlFiles` 按布局枚举，不再递归兜底。

## 7 解析与去重

- **载体识别**：`type=message:role=assistant|toolResult` + `type=compaction|branch_summary` 顶层 `usage`；`input/output/cacheRead/cacheWrite` 取 `usage.*`，`cost` 取 `cost.*`，`reasoning` 不单计。
- **门控**：`has_billable = input||output||cacheRead||cacheWrite >0`，`has_cost = cost.reported().is_some()`，`failed = stopReason∈{error,aborted}`，任一成立即入库，否则丢弃。
- **归属**：`provider = bounded_label(message.provider, "_pi_session")`，`requestModel = message.model`，`model = message.responseModel ?? requestModel`（截断 512B 保 UTF-8），`pricing_model = model`，非 assistant 固化 `unknown/_pi_session`。
- **时间**：`created_at = entry.timestamp ?? message.timestamp ?? header.timestamp ?? file_mtime`，夹逼 SQLite 范围。
- **去重**：`hash_field`（长度前缀）+ `hash_json`（类型标签+键排序+规范化）按 cc-switch 实现，`request_id/semantic_id` 生成一致；同文件 `requestId` 去重（`stopReason` 优先/`output` 最大），跨文件 `session_usage_dedup` 持久去重，叠加 fork `ts<forkTs` 时间切（`header.parentSession + header.timestamp`）。

## 8 增量

- `pi_file_revision()` 读末 4096B SHA256（`pi-session-tail-v1` 域标签）+ `complete = 末字节=='\n'`；`revision` 编码于 `last_synced_at`，`last_line_offset` 为行游标。
- `sync_pi_usage`：`load_sync_cursors()` → 追加校验 `tail(oldEOF)==expected` 则 `seek(oldSize)` 续读，否则全量重扫（账本防双算）；`read_until('\n')` 半行不推进 `committed_offset`，`complete=false` 下轮补全后重验；`Tx` 内原子 `dedup查+写 + proxy_request_logs INSERT OR IGNORE + session_log_sync UPDATE`。

## 9 费用

- `cost.reported().is_some()` 则用 `cost.total`，否则查 `model_pricing` 回算 `CostCalculator(app_type, usage, pricing)`；缺失时为 `0`，`total_cost_usd TEXT` 存 Decimal 字符串。

## 10 聚合与展示

- `Totals.requests = COUNT(*) WHERE data_source='pi_session'`（四载体），`totalTokens = input+cacheRead+output`，`cacheRate = cacheRead/(input+cacheRead)`（四载体和），`input_token_semantics=FRESH`。
- 读路径 `queryTotals/groups/period/sessions/requests` 全部 `SELECT SUM(...) FROM proxy_request_logs WHERE app_type='pi' UNION ALL usage_daily_rollups`，`since/until` 转 `created_at BETWEEN`，`model/cwd` 下推，`page/size/sortKey/sortDir` 下推。
- 总览：8 卡/tape 三段（`input/cacheRead/output`，分母不含 `cacheWrite`）/分组表统一为“四载体合计”，分组表表头 `input→总输入`，`scope-note` 标“含 assistant/toolResult/compaction/branch_summary”。

## 11 CLI / HTTP

- CLI：`token-analyzer sync [--full] [--db <path>]`（`--full` 忽略 `session_log_sync` 全量重扫）、`--prune [--days 30]`，`serve [--port --host --db --dir]` 启动打印 `DB: <path>`；`--help` 口径说明更新。
- HTTP：`GET /api/totals|groups|period|sessions|requests` 改 DB 聚合，`GET /api/db/meta {dbPath,schemaVersion,lastSyncAt,rollupWatermark}`；`POST /api/sessions/rename` 保持不变（文件名前缀改，统计口径不受影响）。

## 12 测试

- 四载体与门控、双去重（同 id/无 id/键重排）、增量四场景（同尺寸/截断/半行/修复）、费用三分支、DB 聚合等价/空库、分组/分页 SQL 下推、总览 tape 一致。
- 全量：`npm test`（`TZ=Asia/Shanghai`）+ `go test ./...` 全绿；手工 `TOKEN_ANALYZER_DB=/tmp/ta.db npm run build && node dist/cli.js sync && node dist/cli.js totals --format json` 与 `SELECT SUM` 一致。

## 13 废弃与兼容

- 废弃 `readSessionFilesCached` 瞬时 Map、`data/opencode/*.json`、`collectJsonlFiles` 递归兜底、无文件回退分支；首启空库即空结果，不做旧数据迁移。

