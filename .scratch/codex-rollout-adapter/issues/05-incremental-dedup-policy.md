# 05: Codex rollout 增量、重写与去重策略

**Type:** grilling
**Status:** resolved
**Blocked by:** 01, 02, 04

## Question

决定 Go 读取器如何处理 rollout 追加、文件替换、截断、压缩归档、重复 usage snapshot、累计值回退/重置和未知 event：采用什么文件 revision、账本 identity、重扫条件和错误/跳过提示，才能不漏算也不双算。

范围只覆盖 totals + sessions 所需的可靠导入；不为 v1 伪造逐请求数据。


## Answer

用户确认采用以下 Codex rollout 增量与去重策略：

- **revision 变化**：物理文件大小、mtime、tail fingerprint 或可读性状态不匹配时全量重扫；旧账本不删除，依靠 `source + response_id` 幂等。
- **plain/compressed 切换**：物理 path 变化即按新 path 全量读取；plain 与 `.jsonl.zst` 仍视为同一 logical rollout，发现阶段 plain 优先并去重。
- **不完整/坏数据**：只提交完整换行记录；坏行和未知 event 跳过并记录 diagnostic，其他有效记录继续处理；不猜测 token。
- **identity 冲突**：同一 response ID 优先保留第一条完整记录；首条无有效 usage 时接受后续完整记录；两条完整记录冲突时保留首条并记录 conflict。
- **事务边界**：每个物理文件的 usage insert、dedup 结果和同步 cursor 在同一事务中提交；cursor 不推进到未提交的半行。
- **时间归属**：使用 rollout envelope timestamp；无效时回退 session metadata timestamp；仍无效时保留记录但不参与时间筛选。
- **关系边界**：保存 physical rollout、stable thread/session、parent/fork metadata，但 v1 不隐式聚合 parent/child/fork；跨物理文件只按 response identity 去重。

研究与决策详情见 [`Codex rollout 增量、重写与去重策略`](../issues/05-incremental-dedup-policy.md)。下一步转入 source/query 契约；Go 实现 seam 与 fixture 验收另建 ticket。
