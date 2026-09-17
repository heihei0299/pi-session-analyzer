# CONTEXT — token-analyzer 领域术语表

> 单上下文仓库。词汇表供实现/诊断/审查命名时使用；命名请用本文术语，勿漂移到同义词。
> 关联决策见 `docs/adr/`。

## 数据域

- **会话文件**：按解析出的布局枚举 `*.jsonl`：`Flat` 只看根目录，`ProjectDirectories` 只看根目录下的项目目录（两层）；首行 `type == "session"` 才是合法会话（`type: message`/`custom` 单条导出视为残留，跳过），生产路径不递归兜底。
- **header**：会话文件首行 JSON entry，权威字段 = `id`（会话 ID）、`timestamp`（会话创建时间）、`cwd`（项目归属）、`parentSession`（fork 标记，见下）。
- **项目归属（cwd）**：以 header `cwd` 为权威（完整绝对路径）；目录名是 cwd 的有损编码（`--` 包裹、`/`→`-`），不可反解，仅展示/辅助分组。聚合按规范化 cwd（resolve 去尾斜杠/符号链接）。
- **计入口径（四载体，直切 cc-switch）**：`assistant`（`type=message, role=assistant`）、`toolResult`（`role=toolResult`）、`compaction`（`type=compaction`）、`branch_summary`（`type=branch_summary`）四者，门控 `has_billable||has_cost||failed`（`has_billable=input||output||cacheRead||cacheWrite>0`，`has_cost=cost.total>0`，`failed=stopReason∈{error,aborted}`），任一成立即计入；其余 `user` 等一律不计入。
- **事实中心（normalized SQLite ledger）**：Pi/Codex usage 唯一事实中心；所有统计窗口只从 ledger 派生，不再有文件扫描统计旁路。
- **查询引擎（Go Query Engine）**：totals/sessions/requests/groups/period/detail/meta 的唯一生产统计 seam（`internal/query`）；只读已提交 ledger 快照，不执行 discovery/parse/sync/游标更新，不写 DB，不理解上游原始格式。
- **刷新（Refresh）**：source → ledger 的唯一写入路径（`internal/refresh` 编排 + `internal/pi` / `internal/codex` source adapter）；承担 source-specific discovery、parse、identity、fork 去重（ADR-0001）、双账本去重、指纹增量、diagnostics；幂等，失败保留上一成功快照并经 meta/diagnostics 暴露。
- **会话数据类型（sessiondata）**：仅保留共享查询 DTO（Filter/View/QueryResult/QueryMeta/详情类型）与 cwd 归一、显示名、排序、周期 helper；不承担文件扫描统计、usage parser 或缓存。会话重命名由 Pi source adapter 的 header-only locator 按 sessionId 定位文件（非统计路径）。
- **子代理会话**：`isTask` 会话（路径含 `/tasks/`，header 含 `parentSession`，由 `parentSessionId` 指向主会话），其消耗在详情视图合并到主会话，主列表保持独立；API 字段 `isTask` 保持原名，仅 UI 文案为“子代理”。
- **存储（SQLite 直切）**：`token-analyzer.db`（`proxy_request_logs/session_log_sync/session_usage_dedup/usage_daily_rollups/model_pricing` + `pi_sessions/source_sessions/source_root_bindings`，与 `cc-switch/schema.rs` 1:1 扩展），`SCHEMA_VERSION=3`，`WAL/foreign_keys/auto_vacuum INCREMENTAL`，路径 `TOKEN_ANALYZER_DB > --db > ~/.cache/token-analyzer/token-analyzer.db`（默认不共库，显式 `TOKEN_ANALYZER_DB=~/.cc-switch/cc-switch.db` 或 `--db` 才共库）。写路径仅 Refresh（Pi `SyncPiUsage` + Codex `SyncRollouts`，按文件事务提交）；读路径仅 Query Engine 的 ledger SQL（`proxy_request_logs ∪ rollups`），经 `OpenReadOnly + query_only` 快照读取。日 rollup 可用于 totals/period 与 model（或无维度）groups；rollup 与其贡献的 raw 行按 prune 合约相加（截断日可存在同日但互不重叠的两类行）。因缺少 session/cwd/request identity，cwd groups、sessions、requests、detail 只使用 raw ledger。
- **Rollup maintenance**：每次成功 Refresh 后按 30 天保留窗口，以本地自然日将过期 raw usage 原子聚合到 `usage_daily_rollups`，再删除 raw 并执行 `incremental_vacuum`；维护只触碰 token-analyzer 自有行（`app_type='pi' AND data_source='pi_session'` 或 `app_type='codex' AND data_source='codex'`），共享 DB 不代表拥有其他 `proxy_request_logs` 行；失败回滚，不留下半状态。partial-day MessageTimeRange 只合并完整中间日，边界日历史 raw 已 prune 时通过 `coverageStatus=partial` 与 warning 明示不可精确恢复。
- **双账本去重**：`session_usage_dedup(data_source, request_id, semantic_id, has_entry_id)`，`request_id = hash(pi-session-request-v3+kind+entry.id+timestamp)`，`semantic_id = hash(pi-session-semantic-v1+kind+entry_ts+msg_ts+provider/model/responseModel/... + complete usage payload)`，同文件 `requestId` 去重（`stopReason` 优先/`output` 最大），跨文件持久账本，叠加 fork `ts<forkTs`。
- **指纹增量**：`PiFileRevision{modifiedMs, fileSize, tailFingerprint(末4096B SHA256, pi-session-tail-v1), complete}` 编码于 `session_log_sync.last_synced_at`，`tail(oldEOF)==expected` 则 `seek`，否则全量重扫（账本防双算），`last_line_offset` 行游标保证半行不推进；`sync_semantics_version` 识别旧 Pi cursor 并触发一次安全重扫自愈。Watch 使用 source-specific revision：Pi 复用 adapter revision，Codex 先用 path/size/mtime/轻量 tail 检测，候选变化后才完整读取/解压。
- **会话发现（双布局，直切 cc-switch providers/pi.rs）**：`PI_CODING_AGENT_SESSION_DIR`（绝对路径才可枚举，相对路径 `400 PI_SESSION_DIR_REQUIRES_PROJECT_CONTEXT`）> `pi native defaults`（`getPiNativeSessionDir()` 读 pi 配置的 session_dir，若无则空）> `~/.pi/agent/sessions`（`--dir` 默认值）；`Flat`（根下 `*.jsonl`）vs `ProjectDirectories`（`sessions/<project>/*.jsonl` 两层）按 `layout` 枚举，不再递归兜底；`--dir` 显式时仅影响第三优先级默认根，`PI_CODING_AGENT_SESSION_DIR` 与 `~/.pi-switch` 等其他项目目录隔离。

## Codex 数据域

- **Codex rollout**：Codex CLI 的持久化运行记录；它不是 Pi 的“会话文件”，同一逻辑 thread 可能有多个物理 rollout。
- **物理 rollout**：磁盘上的一份 Codex rollout 表示；plain JSONL 与 zstd 压缩表示属于同一物理记录，不应重复计数。
- **Codex usage event**：一个有可靠 usage 的 Codex response 记录；累计 snapshot 只是状态，不是额外的 usage event。
- **Codex response identity**：由 Codex source 与 response ID 确定的一次 usage 记录，用于跨重扫、fork/revert 和压缩切换保持幂等。
- **Codex cost 状态**：本 effort 只确认 token usage；`unpriced` 表示没有可用美元花费，不等于花费为零。
- **All 模式成本状态**：All 窗口可能同时含可定价 Pi 与 `unpriced` Codex；窗口 `cost` 保留可定价源的美元合计，`costStatus=unpriced` 表示合计含未定价源，展示必须标注「含 unpriced 源 / 部分可用」，不得把已知金额显示成 unpriced 或真实 `$0`。Codex 单源仍为 `unpriced`；真实零花费由空 `costStatus` + `cost=0` 区分。
- **Codex 计入口径**：上游 `usage.input_tokens` 是**含缓存**的 prompt 总量，因此账本 `input` 一律记非缓存输入（`input_tokens - cached_input_tokens`，饱和减、不为负），`cacheRead = cached_input_tokens`；据此 `totalTokens = input + cacheRead + output`（ADR-0002）等于上游自报的 `usage.total_tokens`（唯一例外是 `cached > input` 的口径异常记录，此时按 0 饱和计入并告警，数值会大于上游）。`cacheWrite` / `reasoning` 独立成列且不参与 `totalTokens`（ADR-0004）。
- **Codex 覆盖率诊断**：物理 rollout 只有 `token_count` 快照、没有 durable usage record 时不计入任何窗口，但必须产生 per-file 诊断（含未计入快照条数，结构化字段 `meta.uncountedSnapshots`）；诊断随文件 revision 存续、每次查询都会重放，游标命中跳过重扫也不例外。累计 snapshot 永远不是 Codex usage event，不参与 totals。
- **Codex 源能力**：`meta.sources` 是后端能力唯一声明源。Go-only 后端恒声明 `["pi","codex"]`（无 npm/TS 后端）；WebUI 只按该声明渲染源选择器。requests、会话详情、重命名仅覆盖 Pi：Codex/All 下这些入口必须禁用并给出原因，服务端返回明确 `unsupported`（400），不得返回 404 或写入 Pi 目录。

## 统计窗口

- **totals（总窗口）**：全量计入口径消息的汇总（requests / input / output / cacheRead / cacheWrite / reasoning / totalTokens / cost / cacheRate）。
- **sessions（会话级窗口）**：每会话一行（按 header 归属）。
- **requests（单请求级窗口）**：每条计入口径消息一行（按消息 timestamp 归属）。
- **cacheRate**：先求和分子分母再除（cacheRead / (input + cacheRead)，分母不含 cacheWrite，ADR-0002）；分母为 0 记 0。
- **totalTokens**：总输入 + 输出 = input + cacheRead + output（不含 cacheWrite；对齐网关 total = promptTokens + completionTokens，ADR-0002）。

## 时间归属

- **时间范围（TimeRange）**：branded 双语义 — `SessionTimeRange`（`kind: "session"`，按会话 header `timestamp` 闭区间，CLI `--since/--until` 与会话管理用）vs `MessageTimeRange`（`kind: "message"`，按消息 `timestamp` 逐条闭区间，webui 全端点 totals/groups/period/requests/sessions 用）。`since` 纯日期按本地 00:00，`until` 按本地 23:59:59.999；无效时间戳消息保守保留。
- **消息级归属**（`MessageTimeRange`）：webui 全端点（totals/groups/period/requests/sessions）的 since/until 语义，跨天会话中落在范围内的请求/消耗按消息 timestamp 计入当天，使总览与明细求和一致（2026-09-01 修复：sessions 亦改为消息级，修复前总览 79M vs 会话明细 33M 对不上）。用户决策（ticket 22/23，2026-09-01 增补 sessions 消息级）。
- **会话级归属**（`SessionTimeRange`）：仅 CLI `--since/--until` 按会话 header timestamp 过滤，跨天会话整段归 header 日（口径 A 保留）。
- 两个层级在跨天场景数字有**预期差异**（webui vs CLI），spec 已记录；TimeRange 的 `kind` 在类型层面杜绝传错（Query Engine 内单引擎分派）。
## 时间语义与网关可比窗口

- **时间参数时区**：webui/CLI 的 since/until 按**本地时区**解释日期（CST）；网关日志 ts 为 UTC。对账时以本地时区解释网关 ts（用户决策 2026-08-06）。
- **网关可比窗口**：2026-08-01T00:00:00Z 起（pi-switch 网关数据起点）；webui 默认窗口 =「自 8/1」预设（since=2026-08-01T00:00:00Z，UTC 精确），可切「全部」看含 8/1 前数据的完整历史（用户决策 2026-08-06）。

## Fork 会话（ticket 25 已实现）

- **fork 会话**：header 含 `parentSession`（父会话文件绝对路径）的会话，由 pi 的 fork 功能创建，复制父会话历史消息（保留原 timestamp 与 usage）。
- **复制历史**：fork 会话中 `message.timestamp < header.timestamp`（fork 创建时间）的消息——其 usage 已在父会话统计过，**fork 本身未消耗这些 token**。
- **forkTs**：fork 创建时间 = fork 会话 header.timestamp，去重切分边界。
- **fork 去重**：Pi source adapter 数据层剔除复制历史，fork 后新增消息（ts ≥ forkTs）保留；CLI/webui 一致（数据读取层）。嵌套 fork 链按各层自身 forkTs 自动正确。

## 字段语义

- **input（非缓存输入）**：usage.input，本次请求未命中缓存的新输入。
- **cacheRead（缓存命中输入）**：usage.cacheRead，独立维度；单请求总输入 = input + cacheRead（与网关 prompt 一致）。
- **总输入（webui 展示语义，ticket 24）**：input + cacheRead；webui 卡片/分组/明细的「输入」列显示总输入，与 pi-switch 网关 Input 对齐；CLI 与导出 JSON/CSV 保持原始字段。
- **output / reasoning**：usage.output（输出）、usage.reasoning（推理，output 子集，不重复累加）。

## 外部对比基准（非数据源）

- **pi-switch 网关**：转发请求日志（`~/.pi-switch/requests.log`），统计口径 total = 总输入 + output（`promptTokens + completionTokens`）。是**对比基准**，不是 token-analyzer 的数据源——token-analyzer 数据只来自 session 目录（用户约束）。两者覆盖范围结构性不同（pi 直连请求只在 session 目录、其他客户端请求只在网关）；fork 去重生效后 8/1 起累计差 0.6%（8/2、8/4 分毫不差），8/1 当天网关刚启用（仅 4 条记录）为最大单日差异源。
- **对账验证（2026-08-07）**：逐条匹配（时间戳+token 数）确认两侧对同一请求定价**完全一致**（679 条 0 差异），差异全部来自覆盖结构：① **pi 内部请求不计入**——pi 压缩/摘要等内部请求（Magic Context，无会话名，真实计费，约 $0.10/天）不入 session 对话流（compaction entry 无 usage 字段），token-analyzer 结构性漏算；② **其他客户端请求只在网关**——opencode dreamer 后台任务等（约 $0.09/天）。webui 已加口径说明（.scratch/webui-gateway-disclaimer/），用户决策：结构性接受、不引入网关数据源。

## 后端与运行时（Go-only 终态，ADR-0005）

- **Go 是唯一生产后端语言**：TypeScript CLI/API/server/db/session/watch 等生产实现与迁移期 parity oracle 均已删除；运行 CLI/API/WebUI 不需要 Node/npm。
- **Query 与 Refresh 分离**：Query 只读快照；server 启动先做初次 Refresh 再对外服务；后续 refresh 走统一串行编排，多个 GET 不放大为重复同步；连续 GET 不改变 ledger 内容或同步游标。
- **Watch 新语义**：只做 source-specific change → source Refresh → query，不再直接累加 usage/cost/totals；Pi 变化只刷新 Pi，Codex 变化只刷新 Codex，失败保留 acknowledged revision 并在下一轮重试；实时 totals 与同一时刻普通 query 完全一致；append/partial/truncate/rewrite/fork/cache/pricing 等规则只存在于 source adapter。`all` 未配置可用 Pi root 时降为 Codex-only（不建立 Pi binding，也不读取 Pi history），但仅限 ledger 确认没有 token-analyzer 自有 Pi history：一旦发现无 binding 的 legacy Pi history，Refresh/Query 都以 `ErrSourceRootBindingRequired` fail closed，不隐藏也不自动认领历史行；`pi` source 仍对不可用 root fail closed。
- **WebUI 单一源码**：唯一人工维护源为 `internal/server/webui.html`（Go embed 直引），无 copy/sync；Go binary 自带完整 WebUI。
- **canonical 契约**：`testdata/canonical` synthetic fixtures + golden expected 为长期行为契约（Pi/Codex 字段级断言，cost 容差 1e-9，排序稳定 tie-breaker）；不再依赖跨 runtime parity。
- **版本与发布**：Git `v<版本>` tag 是唯一版本来源；Makefile 仅将 tag 派生值注入 Go CLI，非 tag 本地构建显示 `dev`。release workflow 只构建 root `token-analyzer` module，并在发布前验证平台 artifact 的 `--version` 与 tag 一致；OpenCode Analyzer 的测试与发布属于外部项目。
- **Pi root binding**：schema v3 的 `source_root_bindings` 将一个 ledger 绑定到一个 Pi root 的真实路径（`Abs/Clean + EvalSymlinks`）。Pi Refresh 首次绑定，其他 root 的 Refresh、Query、QueryDetail 明确拒绝；拒绝不会删除已有历史行。旧 v2/无 binding 的 ledger 在只读 Query 中 fail closed；可写 migration 只补齐 v3 schema，若已有 Pi 历史行，首次 Refresh 也不得自动认领，必须显式迁移/确认或使用新 ledger，只有无历史行的空 ledger 才能首次绑定。root 不存在或 symlink target 无法解析也拒绝，不把词法路径静默当作物理 identity。校验通过后 Refresh/Query/rename 复用同一个已固定的 canonical physical root，不再重新解析词法路径：候选 project 与 `.jsonl` symlink 一律拒绝且不跟随，失败即拒绝（游标、绑定与外部文件保持不变）。该保护是校验式 containment：discovery 阶段过滤，同步前与 rename 前分别复核 candidate/parent 的物理 containment；它不提供 dirfd/renameat 级原子隔离，最后一次复核与实际 open/rename 之间仍存在窗口。

## OpenCode 产品边界

- **OpenCode Analyzer**：独立于 token-analyzer 的外部项目（[OpenCode Analyzer](https://github.com/heihei0299/opencode-analyzer)），归档 OpenCode 云端用量并与本地 Pi 消耗对账；OpenCode 不是 token-analyzer 的 usage source，也不属于 All。
- **OpenCode 对账**：OpenCode 官方扣费与本地 Pi 消耗的比较，仅属于 OpenCode Analyzer；该项目独立维护四载体、门控、fork/request/semantic 去重与月份边界实现；token-analyzer 不处理 OpenCode credential 或对账数据。

