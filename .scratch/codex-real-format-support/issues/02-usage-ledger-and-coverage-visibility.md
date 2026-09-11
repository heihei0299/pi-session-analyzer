# 02: Codex usage 口径对齐与覆盖率可见性

**What to build:** 用户看到的 Codex token 数字与 Codex 自报一致——`input` 是非缓存输入、`cacheRate` 反映真实缓存命中、Codex 窗口的 `totalTokens` 等于上游 `usage.total_tokens`；同时当会话里存在「有 `token_count` 快照、没有 durable usage record」的物理 rollout 时，用户能明确知道这部分没有被计入，而不会把它误读成「Codex 没怎么用」。

**Blocked by:** 01: 真实命名可发现（Codex rollout 发现契约）

**Status:** resolved

- [x] 账本 `input` = `input_tokens - cached_input_tokens`（饱和减、不为负），`cacheRead` = `cached_input_tokens`；`cacheWrite` 与 `reasoning` 继续独立成列且不参与 `totalTokens`。
- [x] Codex 窗口（totals / sessions / groups）的 `totalTokens` 等于同一份 fixture 中上游 `usage.total_tokens` 之和；`cacheRate` 用同一份 fixture 推导正确；Pi 行为不受影响，且 Pi/Codex/All 的同名列 `input` 语义统一为非缓存输入。
- [x] `cached_input_tokens > input_tokens`（口径异常或第三方 provider 语义不同）时产生诊断告警，数值保持非负，不静默修正。
- [x] 无 durable usage record 但有 `token_count` 快照的物理 rollout：不计入任何窗口、不产生 usage 行，但产生 per-file 诊断并声明未计入的快照条数。
- [x] 诊断在四个出口一致可见：CLI stderr、CLI JSON 的 meta、HTTP `/api/meta`、WebUI 诊断条。
- [x] 上游真实存在但不产生 usage 的事件类型不再逐条告警；真正未知的类型仍逐条告警。
- [x] 与真实语义矛盾的旧 fixture 与断言就地迁移清理：带 `Z` 的重复 fixture、`total_tokens` 与字段相加不一致的记录、以及按「input 含缓存」假设写下的数字断言。
- [x] 幂等、半行、表示切换、坏行不阻塞等既有回归在本 ticket 后仍为绿；`all` 源合计不双算。
- [x] 领域术语表的 Codex 计入口径词条，以及跨源 input / cacheRead 语义归一的 ADR 随实现落库（ADR-0002 的 `totalTokens` 公式不变）。

## Implementation summary

- usage 映射改为非缓存口径：`input = input_tokens - cached_input_tokens`（饱和减），`cacheRead = cached_input_tokens`，`totalTokens = input + cacheRead + output` 因此等于上游 `usage.total_tokens`；`cacheWrite` / `reasoning` 仍独立成列。
- `cached > input` 视为 provider 口径异常：按 0 计入并产生带行号的诊断，不静默修正、不写负数。
- 覆盖率可见性：`event_msg` 的 `token_count` 快照单独计数；当物理 rollout 一条 durable usage record 都没有、却有快照时，产生「有 N 条 token_count 快照但无 durable usage record，未计入统计」的 per-file 诊断。累计快照仍然绝不入账。
- 已知非账本事件集合补入真实存在的 `world_state`（本机真实数据 129 条噪声全部来自它）；真正未知的类型仍逐条告警。
- 基线 fixture 迁移到真实语义：三条 usage record 都声明上游 `total_tokens`（14 / 5 / 15），新增一个「只有 2 条 `token_count` 快照 + `world_state`」的物理 rollout，用于同时锁定「快照不入账」「覆盖率诊断」「已知事件不再告警」。
- 断言迁移：`parser_test` / `sync_test` 里按「input 含缓存」写下的数字断言改为非缓存口径；`parser_test` 里自相矛盾的 `total_tokens:19` 改为上游真实语义的 14。
- 文档：CONTEXT 的 Codex 数据域补「Codex 计入口径」「Codex 覆盖率诊断」；新增 ADR-0004 记录跨源 input / cacheRead 语义归一（ADR-0002 公式不变）。
- 覆盖率诊断随文件 revision 持久化（`session_log_sync.diagnostics_summary`，旧库自动补列）：游标命中而跳过重扫时重放该 revision 的 per-file 诊断，避免「未计入」变成一次性提示；`meta.uncountedSnapshots` 结构化暴露未计入快照条数，UI/对账无需解析告警文案。

## Comments

- 2026-09-09 一轮 code-review（Standards / Spec 两轴）后的修正：
  - **Spec 轴发现真实缺陷（已修）**：覆盖率诊断只在解析时产生，而游标命中会跳过重扫，因此第二次查询（以及 WebUI 每次刷新）就看不到「未计入」。这正是本 ticket AC「诊断在四个出口一致可见」要防的事。修法是把 per-file 诊断摘要随游标落库并在跳过时重放，并加了两条回归：sync seam 的 `TestSyncRolloutsReplaysFileDiagnosticsWhenCursorMatches`（连跑两次同步都必须报 2 条未计入）与 query seam 的 `TestQueryCodexCoverageDiagnosticsSurviveRepeatedQueries`（两次查询口径与诊断一致）。
  - **文档不准确（已修）**：CONTEXT 与 ADR-0004 原先无条件断言 `totalTokens` 等于上游自报值；在 `cached > input` 的饱和分支下该等式不成立。已补「唯一例外」与异常时的实际取值（`cached + output`）。
  - **断言覆盖不足（已修）**：原实现只在 totals 上断言新口径。现在 sessions 窗口逐行合计、groups 窗口合计、以及 `all` 源（pi 5 + codex 34 = 39，requests 4）都断言了同一口径，`all` 不双算由此锁定。
  - **告警文案断言重复（已修）**：四个文件里各自 grep 告警文案改为断言结构化 `Diagnostics.UncountedSnapshots` / `meta.uncountedSnapshots`，文案断言只保留在 parser 与 query 各一处。
  - 采纳的其他修正：`snapshots` → `tokenCountSnapshots`、口径异常标记从裸返回值移到 `UsageEvent.CachedOverInput`（签名回到两值）、`event_msg` 分支注释语言统一、CONTEXT 的 `## Codex 数据域` 前补空行、仍带 `Z` 的临时测试路径统一迁移（仅保留 discovery 表里两条**故意**的历史形态容忍用例）。
  - 未采纳：无（两轴给出的项全部处理）。
- 2026-09-09 验证（本机真实 Codex home，全新账本库）：
  - `totalTokens` **145,748,798**，与逐条累加 Codex 自报 `usage.total_tokens` 完全一致（修正前 285,975,614，虚高 96.2%）；
  - `input` 4,937,882（非缓存）、`cacheRead` 140,226,816、`output` 584,100、`cacheWrite` 0、`reasoning` 271,816；
  - `cacheRate` **0.9660**（修正前 0.4911）；成本 `unpriced`；1109 条 usage event 全部入账；
  - 覆盖率缺口：**25 个快照-only 物理 rollout、合计 738 条未计入的 `token_count` 快照**，`meta.uncountedSnapshots = 738`；连续两次查询数字与诊断完全一致（游标命中后仍可见）。
  - 诊断由 129 条「未知 Codex event type × world_state」变为 25 条「有 N 条 token_count 快照…未计入统计」。
- 覆盖率口径更正：本机 74 个物理 rollout 中，31 个有 durable usage record、**25 个只有快照**（即 25 条诊断，逐一对应）、其余 18 个既无 usage 也无快照（无数据可计，正确保持静默）。此前记录的「43 个静默无数据」应拆分为 25 + 18。
- 四个出口的覆盖方式：CLI stderr 与 CLI JSON 读同一份 `meta.warnings`（`jsonOutputData` 对所有窗口都附带 meta，已有用例）；HTTP `/api/meta` 由新增 server 用例断言 warnings、sources 与 `uncountedSnapshots`；WebUI 诊断条是既有渲染器（`meta.warnings` → `#diagnostics`），本 ticket 未新增 UI 拼装逻辑，数据出口由 server 用例锁定。
- 历史账本迁移：按 `response_id` 去重、重扫只跳过，因此旧口径写入的 codex 行不会被改写。实际不构成用户负担——发现契约（01）与本修正同批交付，在此之前真实 Codex home 一个 rollout 都发现不了，不存在用旧口径写入的真实 codex 行；已记入 ADR-0004 的约束一节。
- 验证命令：`go test ./...`（全绿）、`go vet ./...`（clean）、`go build ./cmd/token-analyzer`（通过）、`gofmt -l`（改动包全 clean；`internal/pi` 为既有漂移）。
