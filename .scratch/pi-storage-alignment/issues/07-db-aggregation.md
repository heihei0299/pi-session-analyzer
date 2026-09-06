# 07: DB 聚合与剪枝（totals/groups/period/sessions/requests）

**What to build:** `queryTotals/groups/period/sessions/requests` 全部走 `proxy_request_logs ∪ usage_daily_rollups` 的 SQL 聚合，`since/until` 转 `created_at BETWEEN`，`model/cwd` 下推，`page/size/sortKey/sortDir` 下推，`rollup_and_prune(30)` 日聚合剪枝。

**Blocked by:** 05: 指纹增量同步, 06: 费用回算, 04: 双账本去重

**Status:** ready-for-agent

- [ ] `queryTotals(db, {since,until,model,cwd})`：`SELECT SUM(input)… FROM proxy_request_logs WHERE app_type='pi' AND data_source='pi_session' UNION ALL usage_daily_rollups`，`totalTokens=input+cacheRead+output`、`cacheRate=cacheRead/(input+cacheRead)`（四载体和），空库返回 `emptyTotals`
- [ ] `queryGroups(by=model|cwd|model,cwd)` 与 `queryPeriod(day|week|month)` 同源 SQL 聚合
- [ ] `querySessions/queryRequests` 支持 `page/size/sortKey/sortDir` 下推，`total` 为筛选后全量
- [ ] `rollup_and_prune(30)`：`proxy_request_logs` 中 `created_at < now-30d` 按 `date,app_type,provider_id,model,request_model,pricing_model` 聚合入 `usage_daily_rollups` 后 `DELETE`，账本保留
- [ ] `watch` 复用 `session_log_sync` 的 `FileState{tailFingerprint,complete}`，不再走瞬时 Map
