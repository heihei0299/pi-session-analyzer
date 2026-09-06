# 07: DB 聚合与剪枝（totals/groups/period/sessions/requests）

**What to build:** `queryTotals/groups/period/sessions/requests` 全部走 `proxy_request_logs ∪ usage_daily_rollups` 的 SQL 聚合，`since/until` 转 `created_at BETWEEN`，`model/cwd` 下推，`page/size/sortKey/sortDir` 下推，`rollup_and_prune(30)` 日聚合剪枝。

**Blocked by:** 05: 指纹增量同步, 06: 费用回算, 04: 双账本去重

**Status:** resolved

- [x] `queryTotals(db, {since,until,model,cwd})`：`SELECT SUM(input)… FROM proxy_request_logs WHERE app_type='pi' AND data_source='pi_session' UNION ALL usage_daily_rollups`，`totalTokens=input+cacheRead+output`、`cacheRate=cacheRead/(input+cacheRead)`（四载体和），空库返回 `emptyTotals`
- [x] `queryGroups(by=model|cwd|model,cwd)` 与 `queryPeriod(day|week|month)` 同源 SQL 聚合
- [x] `querySessions/queryRequests` 支持 `page/size/sortKey/sortDir` 下推，`total` 为筛选后全量
- [x] `rollup_and_prune(30)`：`proxy_request_logs` 中 `created_at < now-30d` 按 `date,app_type,provider_id,model,request_model,pricing_model` 聚合入 `usage_daily_rollups` 后 `DELETE`，账本保留
- [x] `watch` 复用 `session_log_sync` 的 `FileState{tailFingerprint,complete}`，不再走瞬时 Map

## 实施总结
- 提交：`6d814e7` — `feat(pi-storage): DB 聚合与 API/总览对齐（07-08）`
- 实现的 seams：见上方验收清单
- 验收标准：全部 `- [x]`
- 测试结果：36-db-aggregation 3/3, 37-api-webui 3/3，typecheck 通过
- 文档对齐：API `GET /api/db/meta` 与 webui.html 四载体文案已对齐
