# 08 — 状态行双值会话数

**What to build:** 应用时间筛选后，状态行显示「会话数: N（全量 M）」——N 为筛选后会话数（与会话明细分页响应的 total 同源），M 为全量会话数（元数据 sessionCount）；未筛选时保持单值 M。筛选口径清晰，用户不再误读筛选结果规模。

**Blocked by:** None — can start immediately

**Status:** ready-for-agent

- [ ] 有筛选时状态行显示「会话数: N（全量 M）」
- [ ] N 与 `/api/sessions` 分页响应的 total 一致（消息级/会话级口径按既有语义）
- [ ] M 与元数据 sessionCount 一致
- [ ] 未筛选时仍显示单值 M
