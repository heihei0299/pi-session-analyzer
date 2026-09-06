# 08: API 与总览对齐（四载体展示）

**What to build:** `GET /api/totals|groups|period|sessions|requests` 改 DB 聚合，`GET /api/db/meta` 新增，总览 8 卡/tape/分组表统一为“四载体合计”，`scope-note` 与费用展示对齐。

**Blocked by:** 07: DB 聚合与剪枝

**Status:** resolved

- [x] `GET /api/totals|groups|period|sessions|requests` 改走 `query*` DB 聚合，`since/until` 消息级（`created_at`），`model/cwd` 下推，`page/size/sortKey/sortDir` 下推，错误体 `{ error, detail }` 保持
- [x] `GET /api/db/meta {dbPath,schemaVersion,lastSyncAt,rollupWatermark}`，`serve` 启动打印 `DB: <path>`
- [x] 总览：8 卡数值=四载体和，`scope-note` 标“含 assistant/toolResult/compaction/branch_summary”，分组表表头 `input→总输入`，tooltip `input+cacheRead`
- [x] tape 三段 `input/cacheRead/output` 分母不含 `cacheWrite`（`input+cacheRead+output`），`input = 总输入`，`cacheRate = cacheRead/总输入`
- [x] `POST /api/sessions/rename` 保持不变，`GET /api/sessions/detail` 的 `totals{main,merged}` 改 DB 源，`requests` 含 `kind/provider/requestModel`

## 实施总结
- 提交：`6d814e7` — `feat(pi-storage): DB 聚合与 API/总览对齐（07-08）`
- 实现的 seams：见上方验收清单
- 验收标准：全部 `- [x]`
- 测试结果：36-db-aggregation 3/3, 37-api-webui 3/3，typecheck 通过
- 文档对齐：API `GET /api/db/meta` 与 webui.html 四载体文案已对齐
