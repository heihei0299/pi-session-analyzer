# 03: Pi / Codex / All 全部切到统一 Go ledger + Query Engine

**What to build:** token-analyzer 的 Pi、Codex 与 All 全部经过 source Refresh → normalized SQLite ledger → 单一 Go Query Engine。用户从 CLI/API/WebUI 读取 totals、sessions、requests、groups、period、detail、meta 时，不再存在 Pi 文件扫描统计、Codex SessionFileData 回绕或独立 All merge 统计实现。

**Blocked by:** 02: 冻结 Go-only 迁移 canonical contract.

**Status:** resolved

- [x] Pi refresh 覆盖 assistant、toolResult、compaction、branch_summary 与 billable/cost/failed 门控。
- [x] Pi 的 fork、requestId/semanticId 双账本、append、partial line、truncate、rewrite、指纹增量满足 canonical contract。
- [x] Pi totals/sessions/requests/groups/period/detail/meta 全部从 ledger-backed Query Engine 派生。
- [x] Pi 的 SessionTimeRange / MessageTimeRange 双语义、detail、子代理、rename 用户行为保持不变。
- [x] Codex refresh 保持 durable usage、response identity、plain/zstd sibling、revert/archive 与 diagnostics 现有语义。
- [x] Codex totals/sessions/groups/period/meta 直接从 normalized ledger 查询，不再经过 ledger → SessionFileData → memory aggregate。
- [x] Codex input/cacheRead/totalTokens 与 ADR-0004 一致；snapshot-only rollout 只产生 diagnostics，不进入 usage。
- [x] All 只是 normalized records 的组合 query，不存在单独的 Pi+Codex 内存 merge 统计实现。
- [x] Codex 单源 costStatus 继续为 unpriced；All 保留已知 Pi 成本并标注含 unpriced 源。
- [x] Codex/All 的 requests、detail、rename 继续明确 unsupported，不返回伪 Pi 结果。
- [x] Query Engine 不理解 Pi/Codex 原始文件格式，不执行 discovery/parse/refresh，不写 DB。
- [x] CLI 与 HTTP 对相同 Query Request 映射到同一 domain result。
- [x] 完成后旧 Pi SessionData 文件扫描/内存 aggregation 与 Codex SessionFileData 回绕都不再是生产统计路径。
- [x] Pi、Codex、All 全部通过 02 的 canonical golden acceptance。

## Answer

- 已完成统一 `Refresh → normalized SQLite ledger → Go Query Engine` 数据流；Pi ledger 原子事务、跨 Refresh replacement、raw + rollup 查询、严格 discovery 与 Codex path scope 已落地。
- 关键提交：`929f4ea`（R1/R2）、`379430e`（R3），R4/R5 收尾随本次提交完成。
- 已执行 root 包级回归覆盖 canonical Pi/Codex、transaction rollback、replacement、rollup、discovery、path containment、watch 与 rename；R4/R5 收尾遵循用户要求不再启动本机编译/测试。
