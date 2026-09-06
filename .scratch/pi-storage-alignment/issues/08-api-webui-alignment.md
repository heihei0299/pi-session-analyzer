# 08: API 与总览对齐（四载体展示）

**What to build:** `GET /api/totals|groups|period|sessions|requests` 改 DB 聚合，`GET /api/db/meta` 新增，总览 8 卡/tape/分组表统一为“四载体合计”，`scope-note` 与费用展示对齐。

**Blocked by:** 07: DB 聚合与剪枝

**Status:** ready-for-agent

- [ ] `GET /api/totals|groups|period|sessions|requests` 改走 `query*` DB 聚合，`since/until` 消息级（`created_at`），`model/cwd` 下推，`page/size/sortKey/sortDir` 下推，错误体 `{ error, detail }` 保持
- [ ] `GET /api/db/meta {dbPath,schemaVersion,lastSyncAt,rollupWatermark}`，`serve` 启动打印 `DB: <path>`
- [ ] 总览：8 卡数值=四载体和，`scope-note` 标“含 assistant/toolResult/compaction/branch_summary”，分组表表头 `input→总输入`，tooltip `input+cacheRead`
- [ ] tape 三段 `input/cacheRead/output` 分母不含 `cacheWrite`（`input+cacheRead+output`），`input = 总输入`，`cacheRate = cacheRead/总输入`
- [ ] `POST /api/sessions/rename` 保持不变，`GET /api/sessions/detail` 的 `totals{main,merged}` 改 DB 源，`requests` 含 `kind/provider/requestModel`
