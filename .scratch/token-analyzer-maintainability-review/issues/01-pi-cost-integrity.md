# 01: 保证 Pi cost 精确写入 normalized ledger

**What to build:** 让 Pi Refresh 计算或读取的 cost 在 normalized ledger、Query、CLI 和 HTTP 输出中保持相同数值，不因金额表示方式而静默少算。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] 整数金额和小数金额写入 ledger 后都保持原始数值语义，包括以零结尾的整数。
- [x] `Refresh → normalized ledger → Query` canonical path 的 totals 与修复前的合法 cost 口径一致。
- [x] 不引入新的数值格式化依赖或第二套 cost 计算逻辑。
- [x] 回归测试覆盖整数 cost、普通小数 cost、重复 Refresh 和 Query 输出。
- [x] 失败的数值编码不会静默写入错误金额；实现使用无错误返回的标准库编码器，无静默失败分支。

## Comments

- Source spec: `.scratch/token-analyzer-maintainability-review/spec.md`
- Triage: ready-for-agent

---

## Completion note

- 修改摘要：将 Pi cost 的 normalized ledger 编码改为 `strconv.FormatFloat`，避免整数金额去零导致少算；新增 canonical cost fixture/golden，覆盖整数、小数和重复 Refresh → Query。
- 验证：`go test ./internal/query -run '^TestPiCostPreservesIntegerAmountsAcrossRefresh$'`；`go test ./internal/pi ./internal/query`，均通过。
- Review：完整 Standards/Spec 双轴 Review 已通过；ADR-0005 fixture finding 已增量复核关闭。
- Commit：`2139ca8 fix(pi): preserve normalized cost precision`。
- 未解决边界问题：无。
- Evidence sync（ticket 12）：以上 checklist 与已记录的聚焦测试和 Review 证据一致。
