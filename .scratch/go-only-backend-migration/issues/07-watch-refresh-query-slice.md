# 07: Watch 改为变化检测 → Refresh → Query

**What to build:** 使用实时 Watch 的用户在文件发生变化后看到的 totals 与普通 Go 查询完全一致；Watch 只负责发现“数据变了”，触发同一 Refresh，再读取同一 Query Engine，不再维护独立 token/cost 累加器。

**Blocked by:** 06: 将 Refresh 生命周期与 Query 彻底分离.

**Status:** ready-for-agent

- [ ] Watch 检测到 Pi 数据变化后调用统一 refresh，而不是直接解析 usage 并累加 Totals。
- [ ] refresh 完成后 Watch 显示的 totals 与立即执行普通 totals query 完全一致。
- [ ] append、partial line、truncate、rewrite、fork 等规则继续由 Pi source adapter 负责，Watch 不复制这些规则。
- [ ] 多个短时间变化可以安全合并，不产生并发 refresh 风暴或重复入账。
- [ ] Watch 不再拥有独立 pricing/cacheRate/totalTokens 业务计算。
- [ ] Watch 的现有 CLI 使用体验保持不变，除非某个旧行为与 canonical contract 明确冲突。
