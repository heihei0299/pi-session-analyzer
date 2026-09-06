# 06: 费用回算（reported 优先，否则 model_pricing）

**What to build:** `costForRecord(record)` 按 `session_usage_pi.rs:insert_pi_record` 的 `reported ?? CostCalculator` 三分支：`cost.total` 有值用 `reported`，否则查 `model_pricing` 回算 `CostCalculator(app_type, usage, pricing)`，缺失时为 `0`，`total_cost_usd TEXT` 存 Decimal 字符串。

**Blocked by:** 03: 四载体解析与门控

**Status:** resolved

- [x] `cost.reported().is_some()` 则用 `cost.total`，否则 `model_pricing` 回算 `CostCalculator`（`input/output/cacheRead/cacheWrite` 分量），缺失时为 `0`
- [x] `model_pricing` 表 CRUD（`model_id PK, display_name, input/output/cache_read/cache_creation_cost_per_million TEXT`），`CostCalculator` 抽 `src/cost/calculator.ts` 与 `internal/cost/calculator.go` 共享
- [x] `total_cost_usd TEXT` 存 Decimal 字符串，`cost_multiplier` 支持（默认 `1.0`）
- [x] `input_token_semantics = FRESH` 写入 `proxy_request_logs`
- [x] 三分支单测：reported/回算/0

## 实施总结
- 提交：`15a3cb7` — `feat(pi-storage): SQLite 基座与四载体解析直切 cc-switch (01-06)`
- 实现的 seams：见上方验收清单
- 验收标准：全部 `- [x]`
- 测试结果：对应 30-35 测试全绿（37 项），typecheck 通过，go vet 通过
- 文档对齐：CONTEXT.md 与 ADR-0003 已对齐（后续 07-08 另提交）
