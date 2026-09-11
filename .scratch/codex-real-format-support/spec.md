# Codex rollout 真实格式对齐 Spec

**Status**: ready-for-agent
**日期**: 2026-09-09
**数据域**: token-analyzer 多数据源 token usage（Codex rollout）
**实现目标**: Go 侧 Codex 适配从「fixture 通过」推进到「真实 rollout 可用且口径可对账」；共享 WebUI 与 npm/TS 分发的源能力诚实性同步收口。

---

## Problem Statement

用户按 README 执行 `token-analyzer totals --source codex`（或 WebUI 切到 Codex/All）时，拿到的是**空结果或明显偏大的数字**，而工具不会提示任何「读不到」的信号——用户无法区分「Codex 确实没消耗」和「适配读错了」。

在一台真实开发机（Codex home 下 74 个物理 rollout、其中 1109 条 Codex usage event、43 个物理 rollout 只有 `event_msg/token_count` 快照）上可复现的事实：

1. **一个 rollout 都发现不了**。发现阶段对 canonical 文件名的判定要求时间戳后紧跟字面量 `Z`，而上游真实文件名是「秒级 UTC 时间戳 + `-` + thread id」，不含 `Z`：74/74 个真实 rollout 全部被判为「非 canonical」跳过，`--source codex|all`、WebUI Codex/All 一律返回空窗口加一片诊断噪声，且不报错。合成 fixture 恰好用了带 `Z` 的命名，因此 9 项验收全绿。
2. **即使能发现，token 也会虚高 96.2%**。Codex 的 `input_tokens` 已包含缓存输入，适配却把它当作非缓存输入、再把 `cached_input_tokens` 叠加为 cacheRead：1109 条 event 汇总 `totalTokens` 285,975,614，而 Codex 自报合计 145,748,798；`cacheRate` 由真实的 96.6% 被算成 49.1%。同理，Codex/All 的 input 列与 Pi 的 input 列（不含缓存）语义不同却混在同名窗口里，跨源对账必然对不上。
3. **58% 的历史 rollout 静默无数据**。43/74 个物理 rollout 只有 `token_count` 快照、没有 durable usage record；当前既不计入也不产生任何专门诊断（真正重要的缺失是无声的，不重要的未知事件类型反而逐条告警）。
4. **WebUI 交互对 Codex 是坏的**。会话列表点开详情对任何 Codex 行都返回 404（详情按 Pi 目录解析会话），重命名会去写 Pi 侧会话。
5. **npm 分发版本有 Codex 按钮、没有 Codex 后端**。共享 WebUI 被一并打进 npm 包，而 TS 后端从不解析 `source` 参数、DB 聚合固定只看 Pi 行：用户在 npm 版面板选 Codex，会看到 Pi 的数字被标成 Codex。
6. **All 模式把 Pi 的真实花费一起藏了**。窗口里出现任一 Codex 文件就把 cost 状态置为 unpriced，WebUI 随即把成本整块显示为 unpriced，Pi 的美元金额不可见。

根因归纳：适配的输入契约（canonical 文件名、usage 字段语义）来自 fixture 假设而不是上游真实输出，fixture 与生产之间没有共同事实；并且「不计入」在实现里默认是静默的。

---

## Solution

把 Codex 适配的**输入契约钉到上游真实输出上**，并让「读不到 / 不计入 / 不可用」在任何出口都可见：

- 真实命名（含 `archived_sessions`、`.jsonl.zst`、revert 形态）能被发现与解析，与 Pi 同一套账本、同一套窗口；
- Codex 的 token 口径与 Codex 自报一致：`input` 恒为非缓存输入，`totalTokens` 与上游 `total_tokens` 对得上，`cacheRate` 正确；
- 未计入的物理 rollout（无 durable usage record）显式告警，用户不会再把它误读成「用量少」；
- WebUI 与 CLI 只在后端真的支持该 source 时提供该能力：Codex/All 下的请求级窗口、会话详情、重命名都明确不支持而不是报 404 或写错目录；
- All 模式给出 Pi 的美元合计并标注「含 unpriced 源」，不把未知成本伪装成零，也不隐藏已知成本。

---

## User Stories

1. 作为一个本机 Codex 用户，我想让 `--source codex` 发现我真实的 rollout 文件，这样我才能看到 Codex 的消耗而不是空窗口。
2. 作为一个 Codex 用户，我想让归档目录与 zstd 压缩表示都被读到，这样冷会话不会静默消失。
3. 作为一个 Codex 用户，我想让 plain 与压缩 sibling 只算一次，这样表示切换不会造成双算。
4. 作为一个对账者，我想让 Codex 窗口的 `totalTokens` 与 Codex 自报的 `total_tokens` 一致，这样我能用同一个数字和 Codex 自己对账。
5. 作为一个对账者，我想让 `input` 在两个源上都表示非缓存输入，这样 Pi/Codex/All 的同名列可以直接相加比较。
6. 作为一个对账者，我想让 `cacheRate` 反映真实的缓存命中比例，这样我不会据此做出错误的成本判断。
7. 作为一个对账者，我想在 Codex 的 `cached_input_tokens` 大于 `input_tokens`（口径异常或第三方 provider 语义不同）时收到告警，而不是看到负数或被静默修正。
8. 作为一个 Codex 用户，我想让没有 durable usage record 但有 `token_count` 快照的 rollout 生成明确诊断，这样我知道这部分没有被计入。
9. 作为一个 Codex 用户，我想让诊断带上未计入快照的数量，这样我能判断覆盖率缺口有多大。
10. 作为一个 Codex 用户，我想让诊断在 CLI（stderr 与 JSON meta）、HTTP `/api/meta` 和 WebUI 诊断条上都能看到，这样任何入口都不会隐藏漏算。
11. 作为一个 Codex 用户，我想让真实存在但不产生 usage 的事件类型不再逐条告警，这样重要诊断不会被噪声淹没。
12. 作为一个 Codex 用户，我想让真正未知的事件类型仍然被报告，这样上游格式变化时我能第一时间发现。
13. 作为一个 Codex 用户，我想让 model 归属继续走 turn → session → unknown 的回退链，这样模型维度拆分不会因缺失字段而整体失真。
14. 作为一个 Codex 用户，我想让 cost 继续以 `unpriced` 表达，这样我不会把未知花费当成零。
15. 作为一个 WebUI 用户，我想在 Codex/All 下看到请求级窗口被禁用并给出原因，而不是一个只含 Pi 数据的伪合计。
16. 作为一个 WebUI 用户，我想让 Codex 会话行不再提供会 404 的「会话详情」，这样我不会以为是我的数据坏了。
17. 作为一个 WebUI 用户，我想让重命名在 Codex/All 下不可用，这样我不会误以为改过名（实际写到 Pi 目录）。
18. 作为一个 WebUI 用户，我想让源选择器只在我使用的后端确实支持时出现，这样我不会被假选择器误导。
19. 作为一个 npm 安装用户，我想让面板不提供它做不到的数据源，这样我不会把 Pi 的数字当成 Codex 的数字。
20. 作为一个 npm 安装的 CLI 用户，我想在传入 `--source` 或 `--codex-dir` 时得到明确报错与指引（例如改用 Go 版本），而不是静默忽略参数。
21. 作为一个发布维护者，我想让 README 的安装方式与能力矩阵一致，这样用户不会按文档得到错误结论。
22. 作为一个 WebUI 用户，我想让 Codex 的按日汇总按消息时间归属，这样跨天的长 rollout 消耗会落在我实际使用的日子上。
23. 作为一个 WebUI 用户，我想在 All 模式下同时看到 Pi 的美元成本与「含 unpriced 源」的标注，这样我既不会误以为总花费为零，也不会丢失已知成本。
24. 作为一个重复执行同步的用户，我想让重扫、追加、截断、替换、plain/zstd 切换都保持幂等，这样我不会因多次查询而看到数字增长。
25. 作为一个在写入中查看的用户，我想让半行不推进游标，这样未落盘的响应不会产生半条记录。
26. 作为一个审阅者，我想让 fixture 使用与上游同形的命名和 usage 语义，这样测试通过才意味着真实可用。
27. 作为一个审阅者，我想让 CI 能因「真实命名不被接受」而失败，这样这条缺陷不会悄悄回来。
28. 作为一个下游实现者，我想让 spec 里的口径写入领域术语表（并与既有 ADR 一致），这样后续改动不会再次漂移。
29. 作为一个跨平台用户，我想让上述行为在 Linux/macOS/Windows 的 release 构建中一致，这样我不会因平台差异看到不同结论。
30. 作为一个安全责任人，我想让验收只用合成 fixture 加本机人工对账，这样真实会话与数据库不会进入仓库。

---

## Implementation Decisions

**本轮已确认（用户决策）**

- 范围 = P0 + P1：真实格式可发现、usage 口径修正、覆盖率可见性、WebUI 交互诚实性、npm/TS 能力诚实性，合并为一个可交付 spec。
- 测试 seam = 三条：源查询适配器端到端（主）、HTTP server 集成、Codex 适配包内两条窄规则单测。
- 老格式 rollout：本 spec 只做可见性诊断，fallback 入账不在范围。
- 展示层默认：period 改为消息级归属；All 模式保留可定价源的美元合计并标注「含 unpriced 源」。

**发现（Discovery）契约**

- canonical 文件名按上游解析规则判定，而不是靠正则猜测：前缀 + 秒级 UTC 时间戳（日期与时间均以 `-` 分隔、无 `Z` 后缀）+ thread id；revert 形态在 thread id 之后追加 `_<rollout id>`；`.jsonl` 与 `.jsonl.zst` 属于同一个物理 rollout。
- `sessions` 与 `archived_sessions` 两个根都递归枚举，目录层级（按年/月/日分目录）不作为语义，只作为路径。
- plain 与压缩 sibling 同时存在时只取 plain；两者互斥，不得重复计入。
- 非 canonical 文件仍跳过并累计诊断；sibling 切换视为同一物理 rollout 的表示变化，重扫但不双算。
- 文件名只用于发现、排序与物理 identity；会话/线程归属一律以 `session_meta` 为准。

**usage 计入口径**

- Codex 的 `input_tokens` 是含缓存的 prompt 总量，因此账本 `input` 恒为非缓存输入 = `input_tokens - cached_input_tokens`（饱和减，不得为负）。
- `cacheRead = cached_input_tokens`；`cacheWrite` 独立成列且不参与 `totalTokens`；`reasoning_output_tokens` 是 output 的子集，只记录不叠加。
- `totalTokens` 继续按 ADR-0002 = `input + cacheRead + output`，由此等于 Codex 自报的 `usage.total_tokens`；不得因为「上游也有 total 字段」而改变公式。
- 同一条 Codex usage event 只计一次：identity 仍是「源 + response id」，`turn_token_usage` / `thread_token_usage` 仍只作诊断。
- 当 `cached_input_tokens > input_tokens` 时产生诊断告警并保持数值非负，不静默修正、不写负数。

**覆盖率可见性**

- 物理 rollout 无 durable usage record 但存在 `token_count` 快照时：不产生 usage 行，但必须产生 per-file 诊断，声明「有 N 条快照、未计入」。
- 诊断经 `meta.warnings` 出口透传：CLI stderr 与 JSON、HTTP `/api/meta`、WebUI 诊断条；空 Codex 目录与部分失败同样只作为警告，不作为服务错误。
- 已知且不产生 usage 的上游事件类型（会话状态快照、设置应用、回合生命周期等）纳入已知集合；未知类型仍逐条告警。

**源能力与出口诚实性**

- `meta.sources` 成为「后端能提供什么」的唯一声明源，UI 不再自行假设。
- 共享 WebUI 只在后端声明了 codex/all 时提供源选择；Codex/All 下请求级窗口、会话详情、重命名入口统一禁用并给出原因。
- 服务端对 Codex/All 的详情与重命名返回明确的 unsupported（400），不返回 404，也绝不写入 Pi 目录。
- TS/npm 后端不得静默忽略 `source`：未声明能力时 UI 不提供该源；TS CLI 收到 `--source` / `--codex-dir` 明确报错并指向 Go 版本；README 的能力矩阵与安装方式一一对应。

**时间归属**

- Codex 与 All 的 period 窗口按 Codex usage event 的消息 timestamp 归属（与领域术语表的消息级归属一致），不再按 rollout header timestamp；跨天 rollout 的消耗按实际使用日拆分。
- Codex 的 totals / sessions / groups / meta 继续沿用既有消息级时间范围语义，与 Pi 对齐。

**成本呈现**

- All 模式保留可定价源的美元合计，并以「含 unpriced 源 / 部分可用」标注；不得把含 unpriced 的窗口整块显示为 unpriced 而丢失已知金额，也不得显示为真实 `$0`。
- Codex 单源窗口继续为 `unpriced`；JSON 与 CSV 保持 `costStatus` 字段可区分。

**不变项（回归护栏，不允许被本次修改破坏）**

- 复用既有账本与去重表：normalized ledger 行、usage dedup identity、文件同步游标；usage 行与游标在同一事务提交，失败不推进游标。
- 幂等：重扫、追加、截断、替换、plain/zstd 切换不得增删历史行、不得双算。
- 半行不推进游标；坏行、未知事件不阻塞同文件其他合法行。
- 不隐式合并 parent / fork / revert 关系，只保存元数据。
- Codex 或 All 的请求级窗口仍然明确拒绝。

**术语与 ADR**

- 领域术语表的 Codex 数据域补充「Codex 计入口径」（input 为非缓存输入、cacheRead ⊆ input、totalTokens 与上游对齐）与「Codex 源能力」（哪些窗口/交互在哪些源可用）。
- 新增一条 ADR 记录「跨源 input / cacheRead 语义归一」：input 恒为非缓存输入，Codex 的 `cached_input_tokens` 是 input 的子集；ADR-0002 的 totalTokens 公式不变。

---

## Testing Decisions

**什么算好测试**

- 只断言外部可观察行为：CLI 输出、HTTP 响应、源查询适配器的窗口结果与 `meta.warnings`；不测私有函数、不断言内部结构。
- 数字断言以「上游自报」为基准：Codex 窗口的 `totalTokens` 必须等于 fixture 中 `usage.total_tokens` 之和，`cacheRate` 由同一份 fixture 推导。
- fixture 与生产同形：命名必须是真实 naming（无 `Z` 后缀、时间以 `-` 分隔、可含 revert 形态、可位于按日分目录下），usage 必须使用真实字段语义（`input_tokens` 含 cached）。
- 每条修复都必须有一条「改动前会红」的测试；测试不得依赖真实 `~/.codex` 或本机数据库。

**Seam ①（最高，主 seam）：源查询适配器端到端**

- 在一套真实形状的合成 Codex home（plain + zstd、sessions + archived_sessions、revert、只有 `token_count` 的旧 rollout、未知事件、坏行、重复/冲突 response）上，以 `pi|codex|all` 三种源跑 totals / sessions / groups / period / meta。
- 断言：发现数量、`totalTokens` 与 Codex 自报一致、`cacheRate`、`unpriced`、sessions 行带 source、period 按消息日落位、`meta.sources` 与 `meta.warnings`（含未计入快照诊断）。
- Prior art：既有的 Codex 端到端 fixture 测试与源查询配置驱动测试。

**Seam ②：HTTP server 集成**

- 覆盖：`source` 参数切换、requests 对 codex/all 的明确拒绝、codex/all 的详情与重命名返回 unsupported、`/api/meta` 的 sources 与 warnings、空目录不返回服务错误。
- Prior art：既有 httptest 风格的 server 集成测试与空目录/警告用例。

**Seam ③（窄）：Codex 适配包内纯规则单测**

- 命名接受集合：真实命名（含按日目录、revert 形态、`.zst`）被接受，带 `Z` 的旧假设命名不再作为唯一合法形态，非 canonical 被拒绝并计数。
- usage 映射恒等式：`TotalTokens == usage.total_tokens`；`cached > input` 触发告警且不产生负数；缺可靠 usage 不产生行。

**人工验收（不提交真实数据）**

- 在本机真实 Codex home 上跑一次 codex totals/sessions，记录发现数量与 token 汇总，与修复前的观测值对比（发现数应由 0 变为 74；汇总应由约 2.86 亿回落到约 1.457 亿），并把数字写入验收记录。

**反回归护栏**

- 移除 fixture 中与真实语义矛盾的构造（带 `Z` 的命名、`total_tokens` 与字段相加不一致的记录）。
- CI 必须在「真实命名不被接受」或「token 被重复计入」时失败；跨平台 release 构建（Linux/macOS/Windows）行为一致。

---

## Out of Scope

- 老格式 `token_count` 快照的 fallback 入账（本 spec 只保证可见性诊断；幂等口径与多 rollout/revert 去重设计另开 spec）。
- TypeScript 侧实现 Codex 解析或账本（只做能力声明、参数诚实性与 UI 一致性）。
- Codex 的请求级窗口（requests）实现。
- sessions 窗口按 thread 折叠或合并同一 thread 的多个物理 rollout 展示。
- 同步性能与增量优化（同一文件的双重全量解码、缺尾换行文件反复全扫、每次查询都触发同步）。
- cost 估算、订阅扣费口径、账单 API、网关对账本身。
- 把任何真实 rollout、会话、数据库或凭据纳入仓库与 fixture。

---

## Further Notes

- 上游兼容是快照而非承诺：命名 grammar 与 usage 语义应连同 `cli_version` 一起出现在诊断里，便于上游变化时定位；上游再改命名时应当只有一处需要改。
- 多 provider 风险：`cached ⊆ input` 是 OpenAI 语义；第三方 provider 的反例由「cached > input」诊断覆盖，本 spec 不预先按 provider 分口径。
- 已知偏差（本次保留，需在 README/术语表中如实记录）：sessions 窗口仍按物理 rollout 一行，同一 thread 的多个 rollout 会出现重复 SessionId；请求级窗口仍不支持 Codex。
- 实现顺序建议：先修发现契约（否则任何真实数据验证都不成立），再修 usage 口径，随后是诊断可见性、WebUI/TS 诚实性、period 与 All 成本呈现。

---

## Comments

- 2026-09-09 审查与合成（工具：源码静态阅读 + 本机真实 rollout 结构化核对 + 正则语义复现；按编译授权铁律**未执行** build/test/typecheck）。
  证据：本机 Codex home 74 个 `rollout-*.jsonl`（0 个 `.zst`，无 `archived_sessions` 目录）全部不匹配发现阶段的文件名判定；1109 条 usage event 满足 `total_tokens == input_tokens + output_tokens`（1108/1109，另一条为零值记录）且 `cached_input_tokens <= input_tokens`（1109/1109）；43/74 个物理 rollout 无 durable usage record、仅有 `token_count` 快照；`turn_id` 在 1109/1109 条上命中 `turn_context` 的 model。
- 2026-09-09 与用户确认：测试 seam 三条；范围 P0 + P1；老格式只做可见性诊断；period 改消息级 + All 模式标注「部分可用」。
