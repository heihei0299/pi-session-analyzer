# 02 — HTTP API 端点集

**What to build:** 六个只读 JSON 端点全部可调用：`/api/totals`、`/api/sessions`、`/api/requests`、`/api/groups?by=`、`/api/period?period=`、`/api/meta`。响应字段与 CLI 结构化输出一致；筛选参数（model/cwd/since/until）直接映射 CLI 语义；错误按统一 JSON 错误体。spec 依据：`.scratch/token-analyzer-webui/spec.md` 的「HTTP API」表。

**Blocked by:** 01 — serve 子命令与最小 HTTP 服务器

**Status:** resolved

- [x] `/api/totals` 返回 Totals 字段，与 CLI totals 输出数字一致（同 fixture）
- [x] `/api/sessions` 每行含 sessionId/timestamp/cwd/model + 指标；`/api/requests` 每行含 sessionId/timestamp/model + 指标
- [x] `/api/groups?by=model|cwd|model,cwd` 分组行与 CLI `--by` 输出一致
- [x] `/api/period?period=day|week|month` 周期行与 CLI `--period` 输出一致
- [x] `/api/meta` 返回 `{ dir, sessionCount, dataRange: { since, until } }`
- [x] model/cwd/since/until 筛选参数过滤结果与 CLI 一致；非法 since/until 或未知 by/period 值 → 400
- [x] 数据目录无合法会话 → 500（detail 附原因）；未知 API 路径 → 404 统一错误体

## 实施总结
- 提交：`<hash>` — feat: HTTP API 端点集（totals/sessions/requests/groups/period/meta + 筛选与统一错误体）
- 实现的 seams：S6 /api/totals 与 CLI json 一致 / S7 sessions+requests 行结构一致 / S8 groups?by=model|cwd|model,cwd 一致 / S9 period?period=day|week|month 一致 / S10 meta（dir/sessionCount/dataRange min/max）/ S11 model/cwd/since/until 筛选一致 + 非法 since/until、未知 by/period → 400 / S12 空目录 500+detail、未知 API 404、非 GET 方法 404
- 验收标准：7 条全部 `- [x]`（见上）
- 测试结果：7/7 全绿（test/07-api.test.ts）；完整套件 61/61
- typecheck：通过（npm run typecheck）
- 文档对齐：README 的 API/前端描述在 ticket 06 收尾一次性补齐（本 ticket 无文档失配）
- 遗留 / 后续建议：POST /api/sessions/rename 当前 404，由 ticket 06 实现；/api/sessions 的 fileName/displayName/cwdNorm 扩展字段同属 ticket 06
