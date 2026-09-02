# 04: OpenCode HTTP API 服务端端点

**What to build:** 在现有 Web 服务器（`src/server.ts` 与 `src/api.ts`）中增加 `/api/opencode/*` 接口路由，为前端 WebUI 与第三方客户端提供 OpenCode 数据查询与同步服务。包括月度成本查询、历史明细分页过滤、本地会话对账计算以及后台异步同步触发。

**Blocked by:** 02: OpenCode 本地数据分层持久化与增量同步仓

**Status:** resolved

- [x] 在 `api.ts` 实现 `GET /api/opencode/costs`，支持参数 `year`, `month`，返回指定月份的按模型细分成本
- [x] 在 `api.ts` 实现 `GET /api/opencode/history`，支持参数 `page`, `size`, `model`, `session`，返回分页的使用历史明细
- [x] 在 `api.ts` 实现 `GET /api/opencode/audit`，对比指定月份内本地 `SessionData` 聚合数据（Tokens、预估费用）与 OpenCode 官方月度账单，返回差异比率与对账指标
- [x] 在 `api.ts` 实现 `POST /api/opencode/sync`，触发后台增量数据同步，返回同步结果（新增条数、最新同步时间）或错误状态
- [x] 实现统一的 JSON 错误响应与参数校验（400/404/500）
- [x] 编写针对 `/api/opencode/*` 各端点的集成测试

## 实施总结
- 提交：`83c375d` — `feat(opencode-sync): OpenCode HTTP API (#04)`
- 实现的 seams：
  - T1 `GET /api/opencode/costs?year=YYYY&month=M` → `{year, month, costs}` 查询本地 `costs.json` 按 year-month key，若缺失 404（选一并文档一致），参数校验 year 4 位、month 1-12 非法 400 — `src/api.ts:parseYearMonth` + `getOpencodeStorage().getCosts`
  - T2 `GET /api/opencode/history?page=N&size=M&model=X&session=Y` → `{rows, total, page, size}` 从 `history.json` 读，支持分页（成对 1-200 非法 400）、按 model/sessionID 精确过滤、按 timeCreated 逆序（已排序）— `src/api.ts:parseOpencodePaging` + `storage.getHistory`
  - T3 `GET /api/opencode/audit?year=YYYY&month=M` → `{localTotals, opencodeTotals, diff, diffRate, comparison}` 对比当月本地 `SessionData` MessageTimeRange 聚合与 OpenCode history 成本/Token 总和，计算差额与比例，标注结构性差异说明 — `src/api.ts` + `src/opencode/audit.ts:buildAudit/computeOpencodeTotals`
  - T4 `POST /api/opencode/sync` → `{added, pages, elapsedMs, lastSyncedTime}` 触发增量同步，读凭证（env/.env 复用 #03 `credentials.ts:loadCredentials`，支持 body 临时覆盖），调用 `OpenCodeClient+Storage.sync`，错误 500 友好含认证失效提示，单锁防并发 — `src/api.ts` ponytail 串行同步
  - T5 统一错误与参数校验：400 非法参数 / 404 无数据 / 409 冲突 / 500 同步失败，全部 `{error, detail}` 与现有 `handleApi` ApiError 风格一致
- 验收标准：
  - [x] 在 `api.ts` 实现 `GET /api/opencode/costs` — `handleApi` 分支，校验+存储查询，缺失 404，测试 T1 正常/缺失/非法/404
  - [x] 在 `api.ts` 实现 `GET /api/opencode/history` — 分页成对校验 1-200、model/session 精确过滤，测试 T2 分页/过滤/校验
  - [x] 在 `api.ts` 实现 `GET /api/opencode/audit` — MessageTimeRange 按月聚合本地 + Date.parse 过滤 opencode + `buildAudit` 求和对比，测试 T3 计算正确/空数据/校验
  - [x] 在 `api.ts` 实现 `POST /api/opencode/sync` — `loadCredentials` + `OpenCodeClient` + `storage.sync` + `opencodeSyncLock`，测试 T4 成功/body 覆盖/认证缺失 500/网络异常 500/自动发现/并发 409
  - [x] 实现统一 JSON 错误响应与参数校验 — ApiError 400/404/409/500 统一 `{error, detail}`，测试 T5
  - [x] 编写针对 `/api/opencode/*` 各端点的集成测试 — `test/04-opencode-api.test.ts` 18 项全绿
- 测试结果：相关测试 18 项全绿（`TZ=Asia/Shanghai node --test test/04-opencode-api.test.ts` pass 18 fail 0）；与 `07-api.test.ts` + `01/02` 合跑 84 项全绿；全量 243 项全绿；typecheck 通过
- typecheck：通过（`npm run typecheck` 无输出）
- 文档对齐：`src/opencode/audit.ts` 拆分对账逻辑以利测试，`localCost` 口径为 `usage.cost.total` 求和（未定价则 0），`opencode cost` 为官方 `cost` 求和，`totalTokens` 按 ADR-0002 `input+cacheRead+output`，`comparison` 固定文案说明结构性差异（未计入 compaction/非 pi 请求）；`costs` 缺失选 404（文档一致），`history` 空返回 200 `{rows:[], total:0}`
- 遗留 / 后续建议：`sync` 单进程内存锁（`ponytail: 串行同步`），多进程/多实例需文件锁；`costs` 404 与 `history` 200 空的差异已文档化；`audit` 仅月度求和，未做按日拆分（待 05 WebUI 扩展）
