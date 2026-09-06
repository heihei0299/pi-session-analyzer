# 01: pi 与 cc-switch 统计对齐（共库 + 四窗口对账）

**What to build:** 使 `token-analyzer` 的今日/7天/30天/全部四窗口 `totalTokens` 与 `cc-switch` 的 `proxy_request_logs WHERE app_type='pi'` 的 `SUM(input+cache_read+output)` 在同库 `~/.cc-switch/cc-switch.db`（`localtime`）下逐项一致，`fmtCompact` 仅展示。

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] `resolveDbPath` 优先级 `TOKEN_ANALYZER_DB > --db > ~/.cc-switch/cc-switch.db > ~/.cache/token-analyzer/token-analyzer.db`，`SCHEMA_VERSION=3` 兼容 `cc-switch` 旧库（`kind/reasoning_tokens/cwd/timestamp_text` 缺失时 `ALTER TABLE ADD COLUMN`，查询 `COALESCE`）
- [ ] `withDirDb` 读路径 `proxy_request_logs WHERE app_type='pi' AND data_source='pi_session' UNION ALL usage_daily_rollups`，`since/until` 转 `created_at BETWEEN`（`localtime`，`since 00:00 / until 23:59:59`），`model/cwd` 下推，`totalTokens=input+cacheRead+output` 与 `cacheRate=cacheRead/(input+cacheRead)` 在 `finalizeTotals` 单点
- [ ] 四载体与门控 `has_billable||has_cost||failed`、双账本 `hash_field/hash_json`、指纹 `seek`+半行 `committed_offset` 与 `cc-switch` 逐字节一致（复用既有 `30-34` seams）
- [ ] `GET /api/totals?since=&until=` 与 `GET /api/db/meta` 实值（`MAX(last_synced_at)`/`MIN(date)`）透传，WebUI `scope-note` 标四载体
- [ ] 新增 `test/38-cc-switch-alignment.test.ts` 在同库下对比 `today/7d/30d/全部` 四窗口 `deepEqual`，`npm test 311+` 与 `go test ./...` 全绿，`npm run build` 成功
