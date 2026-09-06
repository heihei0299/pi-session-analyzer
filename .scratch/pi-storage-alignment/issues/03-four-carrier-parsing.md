# 03: 四载体解析与门控（assistant/toolResult/compaction/branch_summary）

**What to build:** `parsePiUsageRecord(entry)` 按 `session_usage_pi.rs:parse_usage_record` 实现四载体识别与 `has_billable||has_cost||failed` 门控，抽取 `input/output/cacheRead/cacheWrite/cost/provider/requestModel/model(statusCode/errorMessage)`，截断 512B 保 UTF-8。

**Blocked by:** 01: SQLite 持久化基座

**Status:** resolved

- [x] `type=message:role=assistant|toolResult` + `type=compaction|branch_summary` 四载体识别，`usage` 字段 `input/output/cacheRead/cacheWrite` 规范化（`u32`，`min(u32::MAX)`）
- [x] 门控 `has_billable = input||output||cacheRead||cacheWrite >0`，`has_cost = cost.reported().is_some()`，`failed = stopReason∈{error,aborted}`，任一成立入库，否则丢弃（`0-token` 无失败丢弃，`0-token` 有 failed 保留）
- [x] 归属：`provider = bounded_label(message.provider, "_pi_session")`，`requestModel = message.model`，`model = message.responseModel ?? requestModel`（512B 截断），`pricing_model = model`，非 assistant 固化 `unknown/_pi_session`
- [x] 时间：`created_at = entry.timestamp ?? message.timestamp ?? header.timestamp ?? file_mtime` 夹逼 SQLite 范围
- [x] Node 与 Go 双实现一致，`reasoning` 不单计（隐于 output）

## 实施总结
- 提交：`15a3cb7` — `feat(pi-storage): SQLite 基座与四载体解析直切 cc-switch (01-06)`
- 实现的 seams：见上方验收清单
- 验收标准：全部 `- [x]`
- 测试结果：对应 30-35 测试全绿（37 项），typecheck 通过，go vet 通过
- 文档对齐：CONTEXT.md 与 ADR-0003 已对齐（后续 07-08 另提交）
