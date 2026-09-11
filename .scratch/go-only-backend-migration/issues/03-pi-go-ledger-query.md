# 03: Pi 全窗口切到 Go ledger + Query Engine

**What to build:** Pi 用户通过 CLI/API/WebUI 使用 totals、sessions、requests、groups、period、detail、meta 时，Go 后端统一执行 Pi Refresh → normalized ledger → Query Engine，旧 SessionData 文件扫描聚合不再承担生产统计。

**Blocked by:** 01: 冻结 Go-only 迁移 canonical contract.

**Status:** ready-for-agent

- [ ] Pi refresh 覆盖四载体与 billable/cost/failed 门控。
- [ ] fork、双账本、append/partial/truncate/rewrite 与指纹增量满足 canonical contract。
- [ ] totals/sessions/requests/groups/period/detail/meta 全部从 ledger-backed Query Engine 派生。
- [ ] SessionTimeRange 与 MessageTimeRange 的既有双语义保持不变。
- [ ] Pi detail、子代理与 rename 的用户行为保持不变。
- [ ] Query 不扫描 JSONL、不执行 refresh、不写 DB。
- [ ] CLI/API 对相同 query 映射到同一 domain result。
- [ ] 完成后旧 Pi 文件扫描/内存 aggregate 不再是生产查询路径。
