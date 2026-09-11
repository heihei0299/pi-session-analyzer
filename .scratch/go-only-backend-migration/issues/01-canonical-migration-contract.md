# 01: 冻结 Go-only 迁移的 canonical 行为契约

**What to build:** 在任何生产路径切换前，把 Pi、Codex 与现有用户可见窗口的已接受行为固化成可重复执行的 synthetic fixtures 和 golden expected results，使后续每个 Go 迁移 slice 都能证明“行为没漂移”，而不是依赖两个后端互相证明正确。

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

- [ ] canonical fixture 覆盖 Pi 四载体、failed/aborted、fork/nested fork、子代理、双账本去重、append、partial line、truncate、rewrite、mixed model 与 pricing。
- [ ] canonical fixture 覆盖 Codex durable usage、cached input、cached > input、snapshot-only、diagnostics replay、plain/zstd sibling、revert、archive 与非 canonical 输入。
- [ ] golden expected 对 totals、sessions、支持的 requests、groups、period、meta/diagnostics 进行字段级断言，而不是只比较行数。
- [ ] 迁移期 Go 与 TypeScript 对 Pi 行为均可对照同一 golden expected；TypeScript 只作为 oracle，不新增生产能力。
- [ ] cost 浮点比较使用明确容差，排序结果有稳定顺序，避免偶然通过。
- [ ] fixture 全部为合成数据，不包含真实 session、rollout、数据库、cookie、token 或可识别的真实 id。
- [ ] 此 ticket 不改变任何生产行为；后续 ticket 若需要改变 golden expected，必须同时说明对应领域决策变化。
