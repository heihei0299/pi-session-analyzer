# 01: 详情数据基座 — parentSessionId 归属与详情聚合

**What to build:** 让后端能回答“某主会话含哪些子代理、合并后合计是多少、合并后的请求时间线长什么样”。端到端：给定一个含 1 主 + N 子代理的会话目录，`GET /api/sessions/:id/detail` 返回主会话 enriched、子代理列表、合并/主两套 Totals 与混排 requests（附 source 标记），不存在时 404。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] `SessionFileData` 新增可选 `parentSessionId?: string`（解析 header `parentSession` 非空字符串；缺失为 undefined，注释“子代理 (isTask)”）
- [x] `SessionData.detailFromFiles`/`queryDetail` 实现：`parent = find(sessionId)` → 404；`children = filter(parentSessionId===parent.sessionId)`；`mainTotals = totalsFromFiles([parent])`、`mergedTotals = totalsFromFiles([parent,...children])`（求和后 `finalizeTotals` 重算 `cacheRate/totalTokens`）；`requests` 混排 `timestamp asc` 附加 `source/sourceSessionId`
- [x] `GET /api/sessions/:id/detail`（REST `:id`，若冲突退化 `?sessionId=`）返回 `{ session, children, totals:{main,merged,childrenCount}, requests, meta:{hasChildren} }`，复用 `sessionToObject/requestToObject/totalsToObject` 并透传 `displayName/cwdNorm/fileName/source`
- [x] 错误：`sessionId` 缺失/非法 400，不存在 404，读异常 500；与现有 `rename` 的 `ACTIVE_MS` 无关
- [x] 单测：归属（`parentSessionId`）、孤儿不归入、合并 totals（`totalTokens = input+cacheRead+output`、`cacheRate` 重算）、请求混排与 source、404
## 实施总结
- 提交：`a912def0f8f7173a94216699bf7179dbd31d5b80` — `feat(session-detail-subagent): 详情数据基座 (#01)`
- 实现的 seams：
  - Seam-1 parentSessionId 解析（header.parentSession 非空字符串→提取，缺失/空串/非字符串→undefined；isTask 注释更新为“子代理 (isTask)”）— `src/session-data.ts:SessionFileData` & `analyzeFile`
  - Seam-2 归属判定 & 孤儿过滤（children=filter(parentSessionId===parent.sessionId)，孤儿不归入）— `detailFromFiles`
  - Seam-3 合并 totals（main/merged 求和后 finalizeTotals 重算 totalTokens=input+cacheRead+output 与 cacheRate=cacheRead/(input+cacheRead)）— `detailFromFiles`
  - Seam-4 请求混排与 source 标记（[...main,...child].sort(timestamp asc) 附 source/sourceSessionId/displayName）— `detailFromFiles`
  - Seam-5 404 行为（不存在 id 抛 404 status，queryDetail 同）— `detailFromFiles/queryDetail`
  - Seam-6 GET /api/sessions/:id/detail 端点（REST :id 与 /detail?sessionId= 双路由，400/404/500，复用 serialize）— `src/api.ts`
- 验收标准：
  - [x] SessionFileData 新增可选 parentSessionId?: string（解析 header parentSession 非空字符串；缺失为 undefined，注释“子代理 (isTask)”）— `src/session-data.ts:40-45` / Seam-1
  - [x] SessionData.detailFromFiles/queryDetail 实现：parent=find→404；children=filter；main/merged 求和后 finalize；requests 混排 timestamp asc 附加 source/sourceSessionId — `src/session-data.ts:detailFromFiles` / Seam-2~4
  - [x] GET /api/sessions/:id/detail 返回 {session,children,totals:{main,merged,childrenCount},requests,meta:{hasChildren}} 复用 serialize — `src/api.ts:serializeDetail` / Seam-6
  - [x] 错误：sessionId 缺失/非法 400，不存在 404，读异常 500 — `src/api.ts:handleApi` / Seam-6 400/404分支
  - [x] 单测：归属、孤儿、合并 totals、请求混排、404 — `test/25-detail-data-foundation.test.ts` Seam-1~6
- 测试结果：全绿 — `test/25-detail-data-foundation.test.ts` 6/6，相关回归 `07-api/23-fork-dedup/14-session-name` 16/16
- typecheck：通过 (`npm run typecheck` 0 errors)
- 文档对齐：无需更新（#01 为数据基座，无用户可见文案/CLI 变更；CONTEXT/ADR 预留待 #02 后统一增补，README 暂无需同步）
- 遗留 / 后续建议：
  - watch.ts 的 fork 去重仍按旧“任意非空 parentSession 即 fork”判定，对子代理会话（parentSession 为纯 id）本应不触发去重；已在 session-data.ts 修正为“仅含 / 或 \\ 的路径才视为 fork”，watch.ts 保持旧逻辑但因真实子代理消息时间戳恒 ≥ header，故无实际影响，后续可同步修正
  - 前端抽屉与文案重命名由 #02-#04 承载，本 issue 不改 webui.html（已遵守边界）

