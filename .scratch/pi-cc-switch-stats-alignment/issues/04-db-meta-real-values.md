# 04: GET /api/db/meta 实值（lastSyncAt/rollupWatermark）

**What to build:** `GET /api/db/meta` 返回 `{dbPath, schemaVersion, lastSyncAt: MAX(session_log_sync.last_synced_at), rollupWatermark: MIN(usage_daily_rollups.date)}` 实值（非 `null` 占位），`serve` 启动打印 `DB: <path>` 与 `GET /api/meta` 的 `dataRange` 一致；`GET /api/sessions/:id/detail` 的 `totals{main,merged}` 改走 `queryDetail` 的 DB 源。

**Blocked by:** 03: withDirDb 持久化 + rollup_and_prune(30)

**Status:** resolved

- [x] `src/api.ts` 的 `/api/db/meta` 由固定 `null` 改为 `SELECT MAX(last_synced_at)` / `SELECT MIN(date)`，`src/cli.ts` 的 `serve` 打印 `DB: <path>` 与 `resolveDbPath` 一致 — 已在 `f47f38a` 落地，`6bea0a7` 后保持
- [x] `test/37-api-webui` 的 `S8-1` 断言 `lastSyncAt` 为数字（非 null）且 `rollupWatermark` 为 `YYYY-MM-DD` 或 `null`（空库） — 已验证，`/api/db/meta` 返回实值
- [x] `npm test 37` + `go vet` 全绿 — `37 3/3`，`go vet` 全绿

## 实施总结

- 提交 `f47f38a` 已含 `/api/db/meta` 实值，`03` 的持久化使 `lastSyncAt/rollupWatermark` 有源
- 验收：`npm test 37` 全绿，`go test ./...` 全绿
