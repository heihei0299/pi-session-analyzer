# Go-only 后端迁移 Spec

**Status**: ready-for-agent  
**日期**: 2026-09-12  
**数据域**: token-analyzer 多数据源 token usage  
**实现目标**: 将生产后端完整收敛为 Go；Pi 与 Codex 统一写入 normalized SQLite ledger；所有统计窗口通过单一 Go Query Engine 查询；TypeScript 仅在迁移期作为行为 oracle，最终退出生产运行时。

---

## Problem Statement

用户当前使用的是一个已经开始向 Go 原生后端迁移、但仍然存在双实现的 token usage 分析工具。相同的领域规则同时分布在 Go 与 TypeScript 中，Pi、Codex、Watch 还存在不同的统计执行路径，因此同一个“token 用量”概念并不总是经过同一个事实中心。

从用户视角，这带来以下问题：

- 不同入口可能使用不同的统计路径，导致同一批会话数据存在口径漂移风险；
- Pi 与 Codex 虽然都能展示 totals / sessions / groups / period 等窗口，但后台数据流并不真正统一；
- 某些查询会在读取时隐式执行同步写入，使“查询”行为的成本与副作用不可预测；
- Watch 维护独立的增量统计状态，可能与普通查询出现结果差异；
- npm/TypeScript 与 Go 版本能力不同，用户需要理解两个后端版本及其差异；
- WebUI 需要适配两个后端的 capability 差异，并维护重复静态资源；
- 每次修改计入口径、缓存语义、时间语义、去重规则时，都需要同时检查多套实现，增加回归概率；
- 现有跨运行时 parity 测试对部分窗口只验证行数，无法充分证明领域结果真的等价。

项目已经通过既有 ADR 确立了 normalized SQLite ledger、Pi 四载体计入口径、双账本去重、指纹增量和跨源 input/cacheRead 统一语义，因此当前主要问题不是缺少设计，而是实现尚未完全收口到这些已接受的决策上。

---

## Solution

将 token-analyzer 的生产后端完整切换为 Go，并形成一条统一的数据链路：

**源数据 → source adapter refresh → normalized SQLite ledger → Go Query Engine → CLI / HTTP API / Watch / WebUI**

迁移完成后：

- Go 是唯一生产后端语言；
- Pi 与 Codex 的源特有规则只存在于各自的 ingestion/refresh adapter 中；
- normalized SQLite ledger 是 token usage 的唯一事实中心；
- totals、sessions、requests、groups、period、detail、meta 都通过一个 Go Query Engine；
- Query 是纯读取，不负责发现、解析或同步；
- Refresh 明确承担发现、解析、去重、增量写入和诊断；
- Watch 只负责检测变化、触发 Refresh，再读取 Query Engine 的结果；
- CLI 与 HTTP API 都是 Query/Refresh 的薄 adapter；
- WebUI 只由 Go server 提供，并且只有一个人工维护的静态资源源；
- OpenCode 保持“外部对比基准 / audit”领域定位，但其生产后端能力也完整迁移到 Go；
- TypeScript 在迁移期间只作为行为 oracle，待 Go 覆盖完成后从生产运行时删除；
- canonical fixtures 和 golden expected results 替代“两个后端互相证明正确”，成为迁移后的长期行为契约。

测试 seam 采用最高层的单一主路径：

**source refresh → normalized ledger → query engine → CLI/API/WebUI 可观察结果**

只有无法通过主 seam 清晰定位的纯规则，才保留窄的 adapter/domain 单测。

---

## User Stories

1. 作为 token-analyzer 用户，我想只安装和运行一个 Go 后端，这样我不需要理解 npm 版与 Go 版的能力差异。
2. 作为 Pi 用户，我想让所有 Pi 用量都先进入 normalized ledger，这样所有展示入口使用同一份事实。
3. 作为 Pi 用户，我想让 assistant、toolResult、compaction、branch_summary 四载体在所有入口保持同一计入口径，这样 workflow 与子代理不会被某个入口漏算。
4. 作为 Pi 用户，我想让 failed 与 aborted 的计入规则在 CLI、API、WebUI 和 Watch 中完全一致，这样错误请求不会因入口不同而消失。
5. 作为 Pi 用户，我想让 fork copied history 只统计一次，这样 fork 不会重复增加历史 token。
6. 作为 Pi 用户，我想让嵌套 fork 继续按照各自 forkTs 正确去重，这样复杂会话树仍然可信。
7. 作为 Pi 用户，我想让子代理会话保持独立列表行、在详情中合并到主会话，这样迁移不会改变现有产品语义。
8. 作为 Pi 用户，我想让文件追加只导入新增完整行，这样长期运行不会重复重扫或双算。
9. 作为 Pi 用户，我想让半行不推进同步游标，这样正在写入中的消息不会产生半条用量。
10. 作为 Pi 用户，我想让截断、同尺寸替换与文件重写保持幂等，这样日志变化不会让历史数字增长。
11. 作为对账者，我想让 requestId 与 semanticId 的双账本去重规则保持不变，这样跨文件重扫仍然可验证。
12. 作为对账者，我想让 input 始终表示非缓存输入、cacheRead 始终表示缓存命中输入，这样 Pi 与 Codex 可以直接比较。
13. 作为对账者，我想让 totalTokens 始终等于 input + cacheRead + output，这样所有源与网关口径保持可解释。
14. 作为对账者，我想让 cacheRate 始终先汇总再计算，这样分组和总览不会因为逐行平均而失真。
15. 作为 Codex 用户，我想让 rollout 的 durable usage event 写入同一个 normalized ledger，这样 Codex 不再需要转换成另一套会话内存模型才能查询。
16. 作为 Codex 用户，我想让 plain 与 zstd 的同一物理 rollout 只统计一次，这样表示切换不会双算。
17. 作为 Codex 用户，我想让 snapshot-only rollout 继续产生可见诊断但不被当作真实 usage event，这样覆盖率缺口不会伪装成用量。
18. 作为 Codex 用户，我想让 cached_input_tokens 大于 input_tokens 的异常继续被诊断，而不是静默修正或产生负值。
19. 作为 Codex 用户，我想让 totals、sessions、groups、period、meta 直接从 ledger 查询，这样它们与 Pi 使用同一统计引擎。
20. 作为 All 模式用户，我想让 Pi 与 Codex 的合计由同一 Query Engine 完成，这样跨源汇总没有两套 merge 逻辑。
21. 作为 All 模式用户，我想继续看到可定价 Pi 成本，同时明确标注包含 unpriced Codex 源，这样已知成本不会被隐藏。
22. 作为 CLI 用户，我想让 totals、sessions、requests、groups、period 都调用同一个 Query Engine，这样 CLI 不会拥有独立统计逻辑。
23. 作为 API 用户，我想让 GET 查询只读取已提交 snapshot，不在请求过程中隐式扫描或写数据库，这样延迟与副作用可预测。
24. 作为 API 用户，我想让 refresh 失败时仍然能读到上一次成功 snapshot，这样临时文件错误不会让面板完全不可用。
25. 作为 API 用户，我想让并发读取不会触发重复同步，这样多个 WebUI 请求不会互相放大工作量。
26. 作为 WebUI 用户，我想让 source capability 由 Go 后端真实声明，这样界面不会展示后端做不到的入口。
27. 作为 WebUI 用户，我想让不支持 Codex/All 的 requests、详情或重命名入口明确禁用并解释原因，这样不会得到误导性结果。
28. 作为 WebUI 用户，我想让 Pi 会话详情、子代理合并和重命名在 Go-only 后端下保持现有行为，这样迁移不损失日常功能。
29. 作为 WebUI 用户，我想让按消息时间的筛选与 period 归属保持现有语义，这样跨天会话仍然落在实际使用日期。
30. 作为 CLI 用户，我想让会话级 since/until 保留 SessionTimeRange 语义，这样现有脚本的筛选结果不会变化。
31. 作为 Watch 用户，我想让实时显示的 totals 与普通 totals 查询完全一致，这样我不用怀疑实时模式使用了另一套规则。
32. 作为 Watch 用户，我想让 Watch 只触发 refresh，而不重新实现 token 统计，这样新增载体或口径时无需再修改 Watch。
33. 作为 OpenCode 对账用户，我想让同步、导出、audit 与 WebUI 对账能力由 Go 提供，这样删除 TypeScript 后功能不会缺失。
34. 作为 OpenCode 对账用户，我想让 OpenCode 继续保持 benchmark/audit 身份，而不是被混入 Pi/Codex usage ledger，这样领域含义保持清晰。
35. 作为发布用户，我想让单个 Go binary 自带 WebUI，这样部署不需要额外 Node runtime 或静态文件复制步骤。
36. 作为发布用户，我想让项目只发布一种 token-analyzer 后端产品，这样文档、下载与问题排查不会再区分 Go edition / npm edition。
37. 作为维护者，我想让 WebUI 只有一个人工维护源，这样 UI 修改不会产生双副本漂移。
38. 作为维护者，我想让 TypeScript 在迁移期只承担 oracle 角色，这样不会一边迁移一边继续增加双实现债务。
39. 作为维护者，我想在删除 TypeScript 前拥有完整 canonical fixtures，这样删除旧后端不会删除唯一的行为证明。
40. 作为维护者，我想让 canonical fixtures 覆盖 Pi 四载体、fork、dedup、incremental、时间语义和 Codex cache/diagnostics/zstd/revert，这样高风险规则都有长期回归保护。
41. 作为维护者，我想让 golden tests 比较完整可观察结果而不是只比较行数，这样字段级漂移会立即失败。
42. 作为维护者，我想让 Query Engine 可以仅凭 ledger fixture 测试，而不需要真实 Pi/Codex 文件系统，这样 query 与 source ingestion 真正解耦。
43. 作为维护者，我想让 source adapter 可以独立用 synthetic source fixtures 验证 refresh 结果，这样上游格式变化容易定位。
44. 作为维护者，我想让 module/repository/release 命名最终统一到当前项目名称，这样 import、release 和文档不再携带旧拼写。
45. 作为维护者，我想让领域术语表与 ADR 在迁移完成后描述真实实现，而不是描述已删除的双后端架构。
46. 作为贡献者，我想让每个迁移 ticket 都能在一个新上下文中独立完成并验证，这样迁移不依赖巨型 PR。
47. 作为审阅者，我想让每个 ticket 都交付一个可观察的纵向行为，而不是只改某一层，这样每一步都能独立判断是否正确。
48. 作为审阅者，我想让旧实现只在新实现已经覆盖对应行为后删除，这样迁移始终保持可回滚。
49. 作为安全责任人，我想让测试继续只使用合成 fixture，不提交真实 session、rollout、数据库、cookie 或 token，这样迁移不会引入数据泄漏。
50. 作为项目所有者，我想让最终代码中任何 usage 统计规则只有一个生产实现，这样后续增加新 source 时只需要新增 ingestion adapter，而不需要复制整套后端。

---

## Implementation Decisions

- Go 成为唯一 canonical backend。迁移完成后不再存在长期维护的 TypeScript 后端产品。
- normalized SQLite ledger 是 token usage 的唯一事实中心。Pi 与 Codex 都先归一入账，再进行任何统计窗口查询。
- 保留既有 Pi 四载体、billable/cost/failed 门控、双账本去重、指纹增量、forkTs、半行游标等已接受语义；本迁移不重新定义这些规则。
- 保留 ADR-0002 / ADR-0004 的跨源 token 语义：input 恒为非缓存输入，cacheRead 独立，totalTokens 不含 cacheWrite，reasoning 不重复计入 output。
- Source adapter 是 source-specific semantics 的终点。Pi/Codex 上游字段、文件格式、物理表示、诊断与去重在写入 ledger 前归一；Query Engine 不理解上游原始格式。
- Query Engine 是统计领域的唯一生产 seam。totals、sessions、requests、groups、period、detail、meta 都从 ledger 派生。
- Pi、Codex、All 的 source 选择属于 Query Filter，而不是三套独立 query implementation；All 是对 normalized records 的组合查询。
- Query 与 Refresh 分离。Query 不扫描文件、不解析源数据、不更新游标、不写 ledger；Refresh 承担 discovery、parse、identity、incremental write 与 diagnostics。
- Server 生命周期负责初次 refresh 与后续 refresh orchestration；并发 refresh 必须合并，读取请求不能放大为重复同步。
- Refresh 失败保留上一个已提交 snapshot；错误通过 diagnostics/meta 暴露，不破坏纯读 query。
- Watch 只检测变化、触发 refresh、重新 query；Watch 不再维护独立 usage/cost aggregation。
- CLI 与 HTTP API 是薄 adapter：负责参数/协议转换、capability validation、调用 Query/Refresh、渲染或序列化，不重新实现领域统计。
- WebUI 由 Go server 内嵌并只保留一个人工维护源；不再依赖构建期双副本同步。
- WebUI 的 source capability 继续以 backend meta 声明为唯一事实；不支持的交互显式禁用。
- OpenCode 生产能力完整迁移到 Go，但保持“外部 benchmark/audit”领域定位，不进入 Pi/Codex normalized source union，除非未来另立决策。
- TypeScript 在迁移完成前作为 oracle 保留，但冻结新增生产能力；每一类行为只有在 Go 已通过 canonical acceptance 后才能删除对应 TS 路径。
- 删除 TypeScript 后，canonical expected fixtures 与 Go tests 成为长期行为契约，不能依赖已删除 runtime 才能判断正确性。
- module/repository/release 命名统一属于迁移收尾，不与核心数据流重构混在同一早期 slice。
- 不为了目录美观提前做大规模 package rename；先收口行为 seam，再做机械命名清理。
- 不新增抽象仅为了“架构对称”。优先保留少量深 module，只有真实存在第二个 adapter/caller 时才抽取共享 seam。
- 当前迁移不改变用户已接受的 JSON 字段、时间语义、costStatus 语义或 OpenCode 领域定位；任何行为变化必须另行记录。

---

## Testing Decisions

**什么算好测试**

- 只验证外部可观察行为或稳定领域 seam，不锁定私有函数、内部调用次数或临时数据结构。
- 主测试 seam 是完整的“source refresh → normalized ledger → query engine → 可观察出口”。
- 在可以通过 Query Engine 直接验证时，不重复为 CLI 与 HTTP 写大量相同数字断言；adapter 测试只验证协议转换与 capability 行为。
- 每个迁移 slice 必须包含至少一个“旧路径被替换后仍然得到同一结果”的回归场景。
- 测试只使用 synthetic fixture；禁止读取开发者真实 home 数据作为 CI 前提。

**Seam ①（主 seam）：Refresh + Ledger + Query 端到端**

- Pi fixture 覆盖四载体、failed/aborted、fork/nested fork、task、requestId/semanticId 去重、append、partial line、truncate、rewrite、mixed model、pricing。
- Codex fixture 覆盖 durable usage、cached input、cached > input、snapshot-only、diagnostics replay、plain/zstd sibling、revert、archive、非 canonical 输入。
- 对每套 fixture 验证 totals、sessions、支持的 requests、groups、period、meta/diagnostics。
- 重复 refresh 必须幂等。
- Query 前后 ledger 不发生写入变化。

**Seam ②：Query Engine ledger fixture**

- 在不访问 Pi/Codex 文件系统的条件下，以固定 ledger snapshot 验证 source/model/cwd/time filter、排序、分页、costStatus、All 组合和 detail。
- 这是迁移后最重要的纯读取测试 seam，用于证明 query 不依赖 source adapter。

**Seam ③：CLI / HTTP adapter 集成**

- 相同 Query Request 经 CLI 与 HTTP 应映射到相同 domain result。
- HTTP GET 不触发 refresh/write。
- unsupported source/action 返回明确错误，不伪装为空窗口或错误目录操作。
- Server refresh 失败后继续服务上一次 snapshot。

**Seam ④：Watch 行为**

- 给定一次可观察文件变化，Watch 触发 refresh 后展示的 totals 与普通 query totals 完全相同。
- Watch 测试不直接断言独立 token 计算，因为迁移后不应存在该逻辑。

**Seam ⑤：WebUI / OpenCode acceptance**

- WebUI source selector 与 backend capabilities 一致。
- Pi session detail / 子代理 / rename 保持现有行为。
- Codex/All 不支持的操作明确禁用。
- OpenCode sync/export/audit 的用户可见能力在 Go-only server 下可用。
- Go binary 能独立 serve WebUI，不依赖 Node runtime。

**迁移期 parity**

- TypeScript 仅在删除前作为 oracle，用 canonical expected 对照完整结果，不再只比较行数。
- 一旦某类行为由 canonical expected + Go tests 完整覆盖，对应 TS parity 可以在最终 contract ticket 中删除。
- prior art：现有跨运行时 parity、Pi storage alignment、Codex real-format fixture、HTTP server 集成与 WebUI 回归测试。

---

## Out of Scope

- 重新定义 Pi 四载体、fork 去重、双账本、cacheRate、totalTokens 等既有领域规则。
- 为 Codex 增加当前明确不支持的请求级窗口或会话管理能力。
- 将 OpenCode 改造成正式 usage source 或加入 All。
- 引入新的前端框架或为 WebUI 做视觉重设计。
- 为了迁移而更换 SQLite 实现或存储技术。
- 将 pi-switch 网关升级为 token-analyzer 的数据源。
- 解决所有历史数据库兼容问题；迁移仅保证现行 schema/ADR 下的行为。
- 在核心数据流收口前做大规模 package/目录重命名。
- 把真实用户 session、rollout、SQLite 数据库或凭据提交到仓库。
- 在本 spec 内增加新的模型定价策略或订阅额度逻辑。

---

## Further Notes

- 本 spec 的“完成”标准不是仓库中没有 TypeScript，而是**usage 统计领域只剩一个生产实现**。
- TypeScript 删除属于 expand–migrate–contract 的 contract 阶段；在 Go vertical slices 完整之前不得提前执行。
- 最高测试 seam 已根据前序审计与用户确认的 Go-only 目标确定为“Refresh → Ledger → Query → 出口”。本次不再重新访谈范围。
- 现有审计报告与执行计划继续作为背景材料；本 spec 与 tickets 是后续 agent 执行时的规范性入口。
- 按仓库安全规则，实际实现 ticket 时 build / test / lint / typecheck 仍需在执行当次获得明确授权。
