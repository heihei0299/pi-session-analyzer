# 04: Codex / All 切到同一个 ledger-native Query Engine

**What to build:** Codex 与 All 用户查看 totals、sessions、groups、period、meta 时直接查询 normalized ledger，不再经过 ledger → SessionFileData → memory aggregate 回绕；Pi/Codex/All 使用同一统计引擎与统一 token/cost 语义。

**Blocked by:** 01: 冻结 Go-only 迁移 canonical contract.

**Status:** ready-for-agent

- [ ] Codex refresh 保持 durable usage、response identity、plain/zstd、revert/archive、diagnostics 语义。
- [ ] Codex totals/sessions/groups/period/meta 直接通过 ledger-backed Query Engine。
- [ ] All 是 normalized records 的组合 query，不存在单独内存 merge 统计实现。
- [ ] input/cacheRead/totalTokens 与 ADR-0004 一致。
- [ ] snapshot-only rollout 只产生 diagnostics，不进入 usage。
- [ ] Codex costStatus=unpriced；All 保留已知 Pi 成本并标注含 unpriced 源。
- [ ] Codex/All 的 requests/detail/rename 继续明确 unsupported。
- [ ] canonical contract 下 Pi/Codex/All 均通过统一 query acceptance。
