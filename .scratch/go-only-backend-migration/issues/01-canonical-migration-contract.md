# 01: 冻结 Go-only 迁移 canonical contract

**What to build:** 在生产路径切换前，用 synthetic fixtures 与 golden expected 固化 Pi/Codex 当前已接受行为，使后续每个 Go migration slice 都能证明行为没有漂移。

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

- [ ] Pi contract 覆盖四载体、failed/aborted、fork/nested fork、task、requestId/semanticId 去重、append、partial line、truncate、rewrite、mixed model、pricing。
- [ ] Codex contract 覆盖 durable usage、cached input、cached > input、snapshot-only、diagnostics replay、plain/zstd sibling、revert、archive、非 canonical 输入。
- [ ] golden expected 对 totals、sessions、支持的 requests、groups、period、meta/diagnostics 做字段级断言，而非只比较 row count。
- [ ] cost 使用明确容差，排序有稳定 tie-breaker。
- [ ] TypeScript 只作为迁移期 oracle，不新增生产能力。
- [ ] fixture 全部为合成数据，不含真实 session、rollout、DB 或凭据。
- [ ] OpenCode 不进入 canonical usage contract。
