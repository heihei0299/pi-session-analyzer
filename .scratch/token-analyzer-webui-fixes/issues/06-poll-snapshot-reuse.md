# 06 — 轮询复用快照

**What to build:** 自动刷新轮询在总览页检测到数据变化后，直接用本轮已拉取的快照数据渲染（卡片、分组表），不再整刷导致 totals/groups 重复请求；每周期请求收敛到最小集合（totals + groups + meta）。明细页轮询保持「只对比当前页 + total」的既有语义不变。

**Blocked by:** None — can start immediately

**Status:** resolved

- [x] 总览页一个轮询周期内无重复的 totals/groups 请求
- [x] 数据变化时卡片/分组表/状态行正常更新，「已更新 HH:MM:SS」正常显示
- [x] 无变化时不触发渲染
- [x] 明细页（会话/请求）轮询仍只请求当前页 + total
- [x] 会话管理页轮询语义不变（全量会话对比）
