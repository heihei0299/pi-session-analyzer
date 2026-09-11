# 02: Pi totals 全链路切到 Go normalized ledger

**What to build:** Pi 用户执行 totals（CLI 或 WebUI/API）时，Go 后端先按现有四载体、fork、双账本和指纹增量规则刷新 normalized ledger，再由单一 Query Engine 从 ledger 返回 totals；相同数据重复刷新保持幂等，用户看到的数字与 canonical contract 一致。

**Blocked by:** 01: 冻结 Go-only 迁移的 canonical 行为契约.

**Status:** ready-for-agent

- [ ] Pi refresh 覆盖 assistant、toolResult、compaction、branch_summary 与 billable/cost/failed 门控。
- [ ] fork copied history、requestId/semanticId 去重、append、partial line、truncate、rewrite 在 Go refresh 后满足 canonical expected。
- [ ] Pi totals 只从 normalized ledger 读取，不再通过会话文件内存聚合生产 totals。
- [ ] CLI totals 与 HTTP totals 对同一 snapshot 得到同一 domain totals。
- [ ] 重复 refresh 不增加已存在 usage；半行不推进已提交游标。
- [ ] source/model/cwd 与当前 totals 过滤语义不回归。
- [ ] 此 slice 完成后，旧 Pi 读路径仍可暂时保留为迁移 oracle，但不再是 Go totals 的生产路径。
