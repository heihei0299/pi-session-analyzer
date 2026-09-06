# pi 与 cc-switch 统计对齐 — 功能规格

**状态**: draft（ready-for-agent）
**数据域**: `~/.pi/agent/sessions` → SQLite（`proxy_request_logs` 直切 cc-switch，唯一真相源）
**对标**: `/home/shial/.cc-switch/cc-switch.db`（`app_type='pi'`, `data_source='pi_session'`）

---

## Problem Statement

用户在同一 `~/.pi/agent/sessions` 目录下对比 `cc-switch` 与 `token-analyzer` 的今日总量：`cc-switch 783,565,456` vs `token-analyzer 787.5M`（实测 2026-09-06：cc-switch `808,896,765 (2971 req)` vs token-analyzer `801,152,103 (3098 req)`，差 7.7M / 0.96%，全量 3.61B vs 6.00B 差 2.4B）。虽已从重构前的 4% 系统性低估收敛到 <1%，但仍未达到“同目录同窗口逐项相等”的直切目标，影响用户对“唯一真相源”信任。

## Solution

使 `token-analyzer` 的 `totals` 统计（`requests/input/cacheRead/output/totalTokens/cacheRate`）在同 DB 文件、同时间窗口（`localtime` 的 `今天`/`7天`/`全部`）下与 `cc-switch` 的 `SELECT SUM(...) FROM proxy_request_logs WHERE app_type='pi' AND data_source='pi_session'` 逐项一致；`fmtCompact` 仅展示舍入，不参与对账口径。存储路径默认共库 `~/.cc-switch/cc-switch.db`（若存在），否则回退 `~/.cache/token-analyzer/token-analyzer.db`。

## User Stories

1. As a pi 用户, I want `token-analyzer totals` 的今日 `totalTokens` 与 `cc-switch` 今日 `totalTokens` 一致（±0.1%），so that 我信任任一工具均可作为唯一真相源
2. As a pi 用户, I want `token-analyzer totals --since 2026-09-06 --until 2026-09-06` 与 `cc-switch` 的 `date(created_at,'unixepoch','localtime')=date('now','localtime')` 结果一致，so that 时间语义可预期
3. As a pi 用户, I want `token-analyzer totals --since 2026-08-07`（7天）与 `cc-switch` 7天窗口一致，so that 周报可直接互换
4. As a pi 用户, I want `token-analyzer totals`（全部）与 `cc-switch` 全量一致，so that 历史总量不差 2.4B
5. As a pi 用户, I want `totalTokens = input+cacheRead+output`（不含 `cacheWrite`/`reasoning`）在两工具一致，so that 缓存率分母可比
6. As a pi 用户, I want `cacheRate = cacheRead/(input+cacheRead)`（先求和再除，分母不含 `cacheWrite`）在两工具一致，so that 缓存命中率可比
7. As a pi 用户, I want 四载体 `assistant|toolResult|compaction|branch_summary` 均计入且门控 `has_billable||has_cost||failed` 在两工具一致，so that workflow/子 agent 不再漏算
8. As a pi 用户, I want `provider/requestModel/responseModel` 三字段与 `512B UTF-8` 截断在两工具一致，so that `model` 维度分组可比
9. As a pi 用户, I want 双账本去重 `request_id/semantic_id`（`hash_field/hash_json` 规范化）与 `fork ts<forkTs` 在两工具一致，so that 同尺寸重写不双算
10. As a pi 用户, I want 指纹增量 `tailFingerprint(末4096B, pi-session-tail-v1)+complete` 的 `seek` 与半行 `committed_offset` 在两工具一致，so that 今日增量不丢不重且性能可比
11. As a pi 用户, I want `withDirDb` 读路径走 `proxy_request_logs ∪ usage_daily_rollups` 的 SQL 聚合且 `rollup_and_prune(30)` 语义一致，so that 剪枝后总量仍对齐
12. As a 开发者, I want `TOKEN_ANALYZER_DB` > `--db` > `~/.cc-switch/cc-switch.db` > `~/.cache/token-analyzer/token-analyzer.db` 的路径优先级，so that 默认共库无需配置
13. As a 开发者, I want `GET /api/totals?since=&until=` 与 `cc-switch` 的 `created_at BETWEEN` 语义一致（`localtime`，`since 00:00 / until 23:59:59`），so that WebUI 与 DB 可直接对账
14. As a QA, I want `npm test` 中新增 `38-cc-switch-alignment` 对比用例在 `withDirDb` 同库下与 `cc-switch.db` 的 `SUM` 逐项 `deepEqual`，so that 回归可锁定

## Implementation Decisions

- **存储共库**：`resolveDbPath` 新增 `~/.cc-switch/cc-switch.db` 探测分支（`existsSync`），路径优先级 `TOKEN_ANALYZER_DB > --db > ~/.cc-switch/cc-switch.db > ~/.cache/token-analyzer/token-analyzer.db`；`SCHEMA_VERSION=3`（在 `ADR-0003` 的 `1` 基础上补 `kind/reasoning_tokens/cwd/timestamp_text` 四列，已通过 `ensureColumns` 兼容 `cc-switch` 旧库，无则 `ALTER TABLE ADD COLUMN`，查询时 `COALESCE`）。
- **引擎**：Node `node:sqlite:DatabaseSync`（`>=24`）与 Go `modernc.org/sqlite` 保持 `WAL/busy_timeout=5000/foreign_keys=ON/synchronous=NORMAL` 一致，`CGO_ENABLED=0` 交叉编译不受影响。
- **解析口径**：`parsePiUsageRecord` 严格四载体分支（`type=message:role=assistant|toolResult` + `type=compaction|branch_summary` 顶层 `usage`），门控 `has_billable||has_cost||failed`（`failed=stopReason∈{error,aborted}`），`provider/bounded_label` 与 `512B Buffer` 截断保 UTF-8。
- **去重/增量**：`piRequestIdentity` 的 `hash_field`（8字节长度前缀）与 `hash_json`（类型标签+键排序+规范化）与 `cc-switch` 实现逐字节一致；`PiFileRevision` 的 `tailFingerprint` 域标签 `pi-session-tail-v1` 末 4096B SHA256，`complete=末字节=='\n'`，`last_byte_offset/last_tail_fingerprint` 存已提交位点（非全文件），`tailAt(oldByte)==expected ? seek : full`，半行 `complete=false` 时 `committed_offset = lastNL+1` 不推进。
- **费用**：`cost.reported().is_some()` 则用 `cost.total`，否则 `CostCalculator` 回算，`total_cost_usd TEXT` 存 Decimal 字符串，缺失为 `0`。
- **聚合**：`queryTotals/Groups/Period/Sessions/Requests` 全部 `SELECT SUM(...) FROM proxy_request_logs WHERE app_type='pi' AND data_source='pi_session' UNION ALL usage_daily_rollups`，`since/until` 转 `created_at BETWEEN`（`localtime`），`model/cwd` 下推，`page/size/sortKey/sortDir` 下推；`totalTokens` 与 `cacheRate` 在 `finalizeTotals` 单点计算。
- **API/CLI**：`runCli` 与 `handleApi` 已切 `withDirDb`，`GET /api/db/meta {dbPath,schemaVersion,lastSyncAt,rollupWatermark}` 返回实值（`MAX(last_synced_at)`/`MIN(date)`），WebUI `scope-note` 标四载体与 `总输入=input+cacheRead`。
- **兼容**：不保留旧文件聚合回退，首启空库即空结果；`TOKEN_ANALYZER_DB` 指向 `cc-switch.db` 时复用其 `session_log_sync/session_usage_dedup` 账本，避免双写双算。

## Testing Decisions

- **判定标准**：仅测外部行为（CLI `totals --format json` 与 `sqlite3 cc-switch.db SELECT SUM` 数值相等），不测 `hash` 中间值或 `tailFingerprint` 私有函数。
- **被测模块**：`db`（路径/建表/迁移）、`pi-parse`（四载体门控）、`pi-identity`（双账本）、`pi-sync`（指纹 seek/半行）、`db-aggregation`（SQL 聚合与 rollup）、`cli`/`api`（withDirDb 透传）。
- **先验**：复用 `test/30-db-foundation` `31-pi-discovery` `32-four-carrier` `33-dedup` `34-incremental` `36-db-aggregation` `37-api-webui` 的 seam；新增 `test/38-cc-switch-alignment.test.ts` 在同库 `~/.cc-switch/cc-switch.db` 上对比 `today/7d/30d/全部` 四窗口的 `SUM`。

## Out of Scope

- 不改 `pi-switch` 网关 `~/.pi-switch/requests.db` 口径（`promptTokens/completionTokens`），不引入网关数据源
- 不接 `cc-switch` 的 `providers/provider_endpoints/mcp/skills/proxy_config/profiles` 运营表，不做 Tauri/WebDAV/S3 适配
- 不做 `data/opencode/*.json` 旧数据迁移，不保留 `Node 18` 兼容

## Further Notes

- 实测 2026-09-06：共库后 `cc-switch` 今日 `808,896,765 (2971 req)` 与 `token-analyzer` 今日 `801,152,103 (3098 req)` 差 7.7M（0.96%），`cacheRead` 差 11M 指向 `toolResult` 的 `unknown` 归一或 `512B` 截断导致的 127 请求去重差异；全量差 2.4B 系 `cc-switch` 的 `pi` 仅网关后落库，`token-analyzer` 读全历史，预期差异
- `fmtCompact` 的 `787.5M` 仅展示舍入，`totalTokens` 原值 `783,565,456` 与 `787,500,000` 的 0.5% 即舍入噪声，不入断言
- 后续可增 `token-analyzer compare --cc-db <path>` 一键对账命令（本 spec 不纳入）
