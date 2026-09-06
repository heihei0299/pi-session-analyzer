# 03: 四载体解析与门控（assistant/toolResult/compaction/branch_summary）

**What to build:** `parsePiUsageRecord(entry)` 按 `session_usage_pi.rs:parse_usage_record` 实现四载体识别与 `has_billable||has_cost||failed` 门控，抽取 `input/output/cacheRead/cacheWrite/cost/provider/requestModel/model(statusCode/errorMessage)`，截断 512B 保 UTF-8。

**Blocked by:** 01: SQLite 持久化基座

**Status:** ready-for-agent

- [ ] `type=message:role=assistant|toolResult` + `type=compaction|branch_summary` 四载体识别，`usage` 字段 `input/output/cacheRead/cacheWrite` 规范化（`u32`，`min(u32::MAX)`）
- [ ] 门控 `has_billable = input||output||cacheRead||cacheWrite >0`，`has_cost = cost.reported().is_some()`，`failed = stopReason∈{error,aborted}`，任一成立入库，否则丢弃（`0-token` 无失败丢弃，`0-token` 有 failed 保留）
- [ ] 归属：`provider = bounded_label(message.provider, "_pi_session")`，`requestModel = message.model`，`model = message.responseModel ?? requestModel`（512B 截断），`pricing_model = model`，非 assistant 固化 `unknown/_pi_session`
- [ ] 时间：`created_at = entry.timestamp ?? message.timestamp ?? header.timestamp ?? file_mtime` 夹逼 SQLite 范围
- [ ] Node 与 Go 双实现一致，`reasoning` 不单计（隐于 output）
