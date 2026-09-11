# 05: 收口 Refresh / Query / Watch 生命周期

**What to build:** 用户读取数据时只查询最近一次已提交 snapshot；source discovery/sync 由明确 Refresh 生命周期负责；Watch 只检测变化并触发同一 Refresh，再读取同一 Query，因此实时结果与普通查询完全一致。

**Blocked by:** 03: Pi 全窗口切到 Go ledger + Query Engine; 04: Codex / All 切到同一个 ledger-native Query Engine.

**Status:** ready-for-agent

- [ ] Query 不执行 discovery、parse、sync、游标推进或 DB 写入。
- [ ] server 启动可完成初次 Pi/Codex refresh。
- [ ] 后续 refresh 走统一 orchestration，并发 refresh 被合并/串行化。
- [ ] refresh 失败保留上一成功 snapshot，并通过 diagnostics/meta 暴露。
- [ ] 连续 GET 不改变 ledger 或同步游标。
- [ ] Watch 改为 change → refresh → query，不再直接累加 usage/cost/totals。
- [ ] append/partial/truncate/rewrite/fork 规则只存在于 source adapter，不在 Watch 重复实现。
- [ ] Watch totals 与同一时刻普通 query totals 完全一致。
