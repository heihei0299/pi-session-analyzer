# 04: 双账本去重（request_id/semantic_id 持久化）

**What to build:** `piRequestIdentity(entry,kind,usage)` + `session_usage_dedup` 持久账本，按 `session_usage_pi.rs:pi_request_identity` 的 `hash_field/hash_json` 规范生成 `request_id/semantic_id`，同文件 `requestId` 去重（`stopReason` 优先/`output` 最大），跨文件持久账本去重，叠加 fork `ts<forkTs` 时间切。

**Blocked by:** 03: 四载体解析与门控

**Status:** resolved

- [x] `hash_field`（长度前缀）+ `hash_json`（类型标签+键排序+规范化）与 cc-switch 一致，`request_id = hash(pi-session-request-v3+kind+entry.id+timestamp)`（有 id），`semantic_id = hash(pi-session-semantic-v1+kind+entry_ts+msg_ts+provider/model/responseModel/... + canonical usage)`
- [x] `hasEntryId` 区分 legacy 无 id 场景（`requestId = semanticId`），`truncate_usage_label(512B)` 保 UTF-8 边界
- [x] 同文件 `requestId` 去重：`stopReason` 有值覆盖无值，同有/同无时 `output` 更大者覆盖（与 `session_usage.rs:HYco` 一致）
- [x] 跨文件 `session_usage_dedup(data_source, request_id, semantic_id, has_entry_id)` 持久账本 `INSERT OR IGNORE`，`has_entry_id=false` 走 `PI_SEMANTIC_DEDUP_SQL` 路径
- [x] 叠加 fork 时间切：`header.parentSession + header.timestamp` 的 `ts<forkTs` 剔除，与账本双保险

## 实施总结
- 提交：`15a3cb7` — `feat(pi-storage): SQLite 基座与四载体解析直切 cc-switch (01-06)`
- 实现的 seams：见上方验收清单
- 验收标准：全部 `- [x]`
- 测试结果：对应 30-35 测试全绿（37 项），typecheck 通过，go vet 通过
- 文档对齐：CONTEXT.md 与 ADR-0003 已对齐（后续 07-08 另提交）
