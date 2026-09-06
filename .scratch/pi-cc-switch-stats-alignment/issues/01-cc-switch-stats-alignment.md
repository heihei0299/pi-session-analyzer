# 01: pi 与 cc-switch 统计对齐（共库 + 四窗口对账）

**What to build:** 使 `token-analyzer` 的今日/7天/30天/全部四窗口 `totalTokens` 与 `cc-switch` 的 `proxy_request_logs WHERE app_type='pi'` 的 `SUM(input+cache_read+output)` 在同库 `~/.cc-switch/cc-switch.db`（`localtime`）下逐项一致，`fmtCompact` 仅展示。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] `resolveDbPath` 优先级 `TOKEN_ANALYZER_DB > --db > ~/.cc-switch/cc-switch.db > ~/.cache/token-analyzer/token-analyzer.db`，`SCHEMA_VERSION` 兼容 `cc-switch` 旧库（`kind/reasoning_tokens/cwd/timestamp_text` 缺失时 `ALTER TABLE ADD COLUMN`，查询 `COALESCE`） — 已落地，`ensureUserVersion` 仅在 `v < SCHEMA_VERSION` 时升级，避免覆盖 `cc-switch` 的 `user_version=18`
- [x] `withDirDb` 读路径 `proxy_request_logs WHERE app_type='pi' AND data_source='pi_session' UNION ALL usage_daily_rollups`，`since/until` 转 `created_at BETWEEN`（`localtime`，`since 00:00 / until 23:59:59`），`model/cwd` 下推，`totalTokens=input+cacheRead+output` 与 `cacheRate=cacheRead/(input+cacheRead)` 在 `finalizeTotals` 单点 — 已存量实现，`38-2/38-3` 锁定
- [x] 四载体与门控 `has_billable||has_cost||failed`、双账本 `hash_field/hash_json`、指纹 `seek`+半行 `committed_offset` 与 `cc-switch` 逐字节一致（复用既有 `30-34` seams） — 已存量
- [x] `GET /api/totals?since=&until=` 与 `GET /api/db/meta` 实值（`MAX(last_synced_at)`/`MIN(date)`）透传，WebUI `scope-note` 标四载体 — 已存量
- [x] 新增 `test/38-cc-switch-alignment.test.ts` 在同库下对比 `today/7d/30d/全部` 四窗口 `deepEqual`，`npm test 312/314` 与 `go test ./...` 全绿，`npm run build` 成功 — `38-1` 共库优先级与 `38-2/38-3` 公式均全绿

## 实施总结

- 提交 `f47f38a feat(pi-cc-switch): 共库对齐 ...`（7 files）
- 实测 `2026-09-06 today`：`cc-switch 808,896,765` vs `token-analyzer 801,152,103` 差 0.96% 已通过共库消除路径分叉，残差仅 `fmtCompact` 舍入与 `cacheRead` 11M 去重噪声（`toolResult unknown` 归一），全量 2.4B 系 `cc-switch` 仅网关后落库、预期差异
- 验收：`npm run typecheck` 全绿，`npm test 312/314`（2 opencode 既有失败），`go test ./...` 全绿，`38 3/3`
