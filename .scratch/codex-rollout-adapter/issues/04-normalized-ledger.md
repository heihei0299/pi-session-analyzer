# 04: Codex usage 接入 normalized ledger 的模型

**Type:** grilling
**Status:** resolved
**Blocked by:** 01, 02, 03

## Question

在 rollout 格式、usage snapshot 语义和 session identity 研究完成后，决定 Codex 如何进入现有 SQLite ledger：复用哪些字段，如何表达 `source=codex`、session/cwd/model/provider、累计 snapshot 与可计量 token，request identity 是否仍需要，以及 cost unavailable 如何落库。

必须同时决定 source-separated 查询与显式 `all` 合计的边界，避免 Pi 与 Codex 语义不同却被静默混算。


## Answer

用户确认采用以下 normalized ledger 决策：

- **账本单位**：一条 Codex response usage record；`source=codex`，以 `source + response_id` 作为逻辑 identity。物理 rollout path 只作来源/同步信息，stable thread/session identity 作为查询维度；不隐式合并 parent/child/fork。
- **usage 来源**：优先且仅使用可靠的 `TokenUsageRecord.usage`；不把 `TokenCount` 累计快照混入账本，也不为没有可靠 record 的旧/异常数据推算 usage。
- **cost**：复用现有 schema，Codex 行写入 `total_cost_usd = "0"` 与 `pricing_model = "unpriced"`；这是 unavailable sentinel，不代表真实花费为零。
- **model/provider**：优先 response/turn 级归属，缺失时回退到 `SessionMeta.model_provider`，再缺失使用 `unknown`；不得把整个 Codex source 粗暴归并为一个 model。
- **请求数**：继续复用 `Totals.Requests`，含义是导入的 Codex response usage record 数量；与 Pi 的 assistant usage event 数量一样，仅在 `all` 视图中按各 source 的已导入账本行求和。

由此，下一步 [Codex rollout 增量、重写与去重策略](05-incremental-dedup-policy.md) 负责把这些 identity 与 usage 优先级落成文件同步规则；model 具体来源和 Go zstd 实现另见新 research tickets。
