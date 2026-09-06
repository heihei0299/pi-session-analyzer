# 06: 费用回算（reported 优先，否则 model_pricing）

**What to build:** `costForRecord(record)` 按 `session_usage_pi.rs:insert_pi_record` 的 `reported ?? CostCalculator` 三分支：`cost.total` 有值用 `reported`，否则查 `model_pricing` 回算 `CostCalculator(app_type, usage, pricing)`，缺失时为 `0`，`total_cost_usd TEXT` 存 Decimal 字符串。

**Blocked by:** 03: 四载体解析与门控

**Status:** ready-for-agent

- [ ] `cost.reported().is_some()` 则用 `cost.total`，否则 `model_pricing` 回算 `CostCalculator`（`input/output/cacheRead/cacheWrite` 分量），缺失时为 `0`
- [ ] `model_pricing` 表 CRUD（`model_id PK, display_name, input/output/cache_read/cache_creation_cost_per_million TEXT`），`CostCalculator` 抽 `src/cost/calculator.ts` 与 `internal/cost/calculator.go` 共享
- [ ] `total_cost_usd TEXT` 存 Decimal 字符串，`cost_multiplier` 支持（默认 `1.0`）
- [ ] `input_token_semantics = FRESH` 写入 `proxy_request_logs`
- [ ] 三分支单测：reported/回算/0
