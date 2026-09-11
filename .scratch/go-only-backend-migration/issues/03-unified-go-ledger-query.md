# 03: Pi / Codex / All 全部切到统一 Go ledger + Query Engine

**What to build:** token-analyzer 的 Pi、Codex 与 All 全部经过 source Refresh → normalized SQLite ledger → 单一 Go Query Engine。用户从 CLI/API/WebUI 读取 totals、sessions、requests、groups、period、detail、meta 时，不再存在 Pi 文件扫描统计、Codex SessionFileData 回绕或独立 All merge 统计实现。

**Blocked by:** 02: 冻结 Go-only 迁移 canonical contract.

**Status:** ready-for-agent

- [ ] Pi refresh 覆盖 assistant、toolResult、compaction、branch_summary 与 billable/cost/failed 门控。
- [ ] Pi 的 fork、requestId/semanticId 双账本、append、partial line、truncate、rewrite、指纹增量满足 canonical contract。
- [ ] Pi totals/sessions/requests/groups/period/detail/meta 全部从 ledger-backed Query Engine 派生。
- [ ] Pi 的 SessionTimeRange / MessageTimeRange 双语义、detail、子代理、rename 用户行为保持不变。
- [ ] Codex refresh 保持 durable usage、response identity、plain/zstd sibling、revert/archive 与 diagnostics 现有语义。
- [ ] Codex totals/sessions/groups/period/meta 直接从 normalized ledger 查询，不再经过 ledger → SessionFileData → memory aggregate。
- [ ] Codex input/cacheRead/totalTokens 与 ADR-0004 一致；snapshot-only rollout 只产生 diagnostics，不进入 usage。
- [ ] All 只是 normalized records 的组合 query，不存在单独的 Pi+Codex 内存 merge 统计实现。
- [ ] Codex 单源 costStatus 继续为 unpriced；All 保留已知 Pi 成本并标注含 unpriced 源。
- [ ] Codex/All 的 requests、detail、rename 继续明确 unsupported，不返回伪 Pi 结果。
- [ ] Query Engine 不理解 Pi/Codex 原始文件格式，不执行 discovery/parse/refresh，不写 DB。
- [ ] CLI 与 HTTP 对相同 Query Request 映射到同一 domain result。
- [ ] 完成后旧 Pi SessionData 文件扫描/内存 aggregation 与 Codex SessionFileData 回绕都不再是生产统计路径。
- [ ] Pi、Codex、All 全部通过 02 的 canonical golden acceptance。
