# 05: Codex 与 All 改为 ledger-native 查询

**What to build:** Codex 和 All 用户查看 totals、sessions、groups、period、meta 时，Go 后端直接从 normalized ledger 查询，不再把 Codex 账本重新包装成会话文件内存模型；Pi/Codex/All 使用同一个 Query Engine 和统一 token/cost 语义。

**Blocked by:** 01: 冻结 Go-only 迁移的 canonical 行为契约; 02: Pi totals 全链路切到 Go normalized ledger.

**Status:** ready-for-agent

- [ ] Codex refresh 保持 durable usage、response identity、plain/zstd 去重、revert、archive 与 diagnostics 现有语义。
- [ ] Codex totals/sessions/groups/period/meta 直接由 normalized ledger/query 产生，不经过 SessionData 风格的文件模型再聚合。
- [ ] All 是 normalized records 的组合查询，不存在单独的 Pi+Codex 内存 merge 统计实现。
- [ ] Codex input/cacheRead/totalTokens 与 ADR-0004 一致；cached > input 仍产生诊断且不出现负 input。
- [ ] snapshot-only rollout 不进入 usage totals，但其 uncountedSnapshots/warnings 每次查询仍可见。
- [ ] Codex 单源 costStatus 继续为 unpriced；All 保留已知 Pi 美元成本并标注含 unpriced 源。
- [ ] requests、详情与重命名对 Codex/All 继续明确 unsupported，不能返回伪 Pi 结果或误写 Pi 会话。
- [ ] Pi、Codex、All 在相同 Query Engine 下通过 canonical golden acceptance。
