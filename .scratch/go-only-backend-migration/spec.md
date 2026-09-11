# Go-only 后端迁移 Spec

**Status**: ready-for-agent  
**日期**: 2026-09-12  
**数据域**: token-analyzer 的 Pi / Codex token usage  
**实现目标**: 将生产后端完整收敛为 Go；Pi 与 Codex 统一写入 normalized SQLite ledger；所有统计窗口通过单一 Go Query Engine 查询；TypeScript 仅在迁移期作为行为 oracle，最终退出生产运行时；OpenCode 从 token-analyzer 核心解耦，并迁入仓库内独立 `opencode-analyzer/` 项目目录，作为未来独立发布单元。

---

## Problem Statement

token-analyzer 当前已经开始迁移到 Go，但同一 token usage 领域仍分散在 Go、TypeScript、Pi 文件扫描、Codex ledger 回绕和 Watch 独立累加等多条执行路径中。

从用户视角，这意味着：

- 同一批 Pi/Codex 数据可能经过不同统计实现；
- Query 有时隐式承担同步写入，读取行为不够可预测；
- Watch 维护第三套 usage/cost 聚合逻辑；
- npm/TypeScript 与 Go 后端能力不同；
- WebUI 为两个后端和多种旁路能力承担额外复杂度；
- parity 测试对部分窗口只比较行数，不能证明字段级语义一致；
- 当前仓库还把 OpenCode RPC、同步、存储、HTTP API、WebUI audit 等独立领域能力直接嵌在 token-analyzer 内部，使两个未来应独立演进、独立发布的产品共享代码边界。

项目已经通过既有 ADR 固化 normalized SQLite ledger、Pi 四载体、双账本去重、指纹增量和跨源 input/cacheRead 语义。当前问题不是缺少设计，而是实现尚未完全收敛，同时混入了不属于 token-analyzer 核心域的 OpenCode 集成。

---

## Solution

将 token-analyzer 收敛成一个边界清晰的 Go-only Pi/Codex usage analyzer：

**Pi/Codex 源数据 → source refresh → normalized SQLite ledger → Go Query Engine → CLI / HTTP API / Watch / WebUI**

迁移完成后：

- Go 是唯一生产后端语言；
- Pi 与 Codex 是当前项目唯一正式 usage sources；
- normalized SQLite ledger 是唯一 usage 事实中心；
- totals、sessions、requests、groups、period、detail、meta 只通过一个 Go Query Engine；
- Query 只读，不执行 discovery、parse、sync 或游标更新；
- Refresh 承担 source-specific discovery、parse、identity、dedup、incremental write 与 diagnostics；
- Watch 只检测变化、触发 Refresh，再读取 Query；
- CLI / HTTP 是 Query/Refresh 的薄 adapter；
- WebUI 只由 Go server 提供，并只有一个人工维护源；
- TypeScript 只在迁移期作为 oracle，最终从生产运行时删除；
- canonical fixtures / golden expected 成为长期行为契约；
- **OpenCode 不属于 token-analyzer 的功能域**：现有 OpenCode 能力迁入仓库根下独立 `opencode-analyzer/` 项目目录，拥有自己的入口、依赖、storage、API、WebUI、测试与文档边界；
- `opencode-analyzer/` 不允许 import 或复用 token-analyzer 的 `internal/*`，token-analyzer 也不依赖 `opencode-analyzer/`；两者暂时同仓只是托管关系，不是运行时关系；
- OpenCode 不进入 token-analyzer 的 normalized ledger、`meta.sources`、`source=all`、HTTP API 或 WebUI；
- 本次只完成可整目录迁出的 standalone project boundary，不负责把 `opencode-analyzer/` 发布到独立仓库或上线。

主测试 seam：

**source refresh → normalized ledger → Query Engine → CLI/API/WebUI 可观察结果**

OpenCode 解耦以双向独立为验收 seam：token-analyzer 在没有 `opencode-analyzer/` 时仍完整工作；`opencode-analyzer/` 不引用 token-analyzer 私有实现，并可在未来整目录迁出为独立仓库。

---

## User Stories

1. 作为 token-analyzer 用户，我想只安装和运行一个 Go 后端，这样我不需要理解 npm 与 Go 两个版本。
2. 作为 Pi 用户，我想让所有 Pi usage 先进入 normalized ledger，这样所有入口使用同一事实。
3. 作为 Pi 用户，我想让 assistant、toolResult、compaction、branch_summary 在所有入口使用同一计入口径。
4. 作为 Pi 用户，我想让 failed/aborted 的计入规则在 CLI、API、WebUI、Watch 中一致。
5. 作为 Pi 用户，我想让 fork copied history 只统计一次。
6. 作为 Pi 用户，我想让 nested fork 按各自 forkTs 正确去重。
7. 作为 Pi 用户，我想让子代理会话与现有详情语义保持一致。
8. 作为 Pi 用户，我想让 append 只导入新增完整行。
9. 作为 Pi 用户，我想让 partial line 不推进同步游标。
10. 作为 Pi 用户，我想让 truncate、rewrite、same-size replacement 保持幂等。
11. 作为对账者，我想让 requestId/semanticId 双账本语义保持不变。
12. 作为对账者，我想让 input 始终表示非缓存输入。
13. 作为对账者，我想让 cacheRead 始终表示缓存命中输入。
14. 作为对账者，我想让 totalTokens 始终等于 input + cacheRead + output。
15. 作为对账者，我想让 cacheRate 继续先汇总再计算。
16. 作为 Codex 用户，我想让 durable usage event 写入同一个 normalized ledger。
17. 作为 Codex 用户，我想让 plain/zstd 同一物理 rollout 只算一次。
18. 作为 Codex 用户，我想让 snapshot-only rollout 保持“诊断可见但不计入 usage”。
19. 作为 Codex 用户，我想让 cached > input 异常继续显式诊断。
20. 作为 Codex 用户，我想让 totals/sessions/groups/period/meta 直接从 ledger 查询。
21. 作为 All 用户，我想让 Pi/Codex 合计通过同一 Query Engine。
22. 作为 All 用户，我想保留已知 Pi 成本并明确标注包含 unpriced Codex。
23. 作为 CLI 用户，我想让所有查询窗口调用同一 Query Engine。
24. 作为 API 用户，我想让 GET 查询只读取已提交 snapshot。
25. 作为 API 用户，我想让 refresh 失败时仍可读取上一次成功 snapshot。
26. 作为 API 用户，我想让并发读取不会触发重复同步。
27. 作为 WebUI 用户，我想让 source capability 完全由 Go 后端真实声明。
28. 作为 WebUI 用户，我想让 Codex/All 不支持的交互明确禁用。
29. 作为 WebUI 用户，我想让 Pi session detail、子代理与 rename 保持现有行为。
30. 作为 WebUI 用户，我想让 MessageTimeRange 的现有语义保持不变。
31. 作为 CLI 用户，我想让 SessionTimeRange 的现有语义保持不变。
32. 作为 Watch 用户，我想让实时 totals 与普通 query 完全一致。
33. 作为 Watch 用户，我想让 Watch 不再重新实现 token 统计。
34. 作为发布用户，我想让单个 Go binary 自带完整 WebUI。
35. 作为发布用户，我想让项目只发布一种 token-analyzer 后端产品。
36. 作为维护者，我想让 WebUI 只有一个人工维护源。
37. 作为维护者，我想让 TypeScript 在迁移期只承担 oracle 角色。
38. 作为维护者，我想在删除 TypeScript 前拥有完整 canonical fixtures。
39. 作为维护者，我想让 golden tests 比较完整结果而不是只比较行数。
40. 作为维护者，我想让 Query Engine 可以只凭 ledger fixture 测试。
41. 作为维护者，我想让 source adapter 用 synthetic source fixture 独立验证。
42. 作为项目所有者，我想把现有 OpenCode 能力整体迁入独立 `opencode-analyzer/` 目录，这样它可以作为单独产品继续演进。
43. 作为项目所有者，我想让 `opencode-analyzer/` 拥有自己的入口、依赖、storage、API、WebUI、测试和 README，这样未来拆仓不需要重构产品边界。
44. 作为项目所有者，我想让 token-analyzer 的 HTTP API 与 WebUI 不再暴露 OpenCode 能力，这样两个产品没有运行时耦合。
45. 作为 CLI 用户，我想让 token-analyzer 不再包含 OpenCode 专用命令，而 OpenCode 专用命令由 `opencode-analyzer/` 自己提供。
46. 作为安全责任人，我想让 OpenCode cookie/credential 只由 `opencode-analyzer/` 处理，token-analyzer 永远不读取这些敏感信息。
47. 作为维护者，我想让 `opencode-analyzer/` 不依赖 token-analyzer 的 `internal/*`，这样未来可以整目录迁出仓库。
48. 作为维护者，我想保留现有 OpenCode 对账能力，而不是在解耦过程中删除它，这样后续可以直接发展成独立项目。
49. 作为维护者，我想让 module/repository/release 命名最终统一到当前项目名称。
50. 作为维护者，我想让 CONTEXT/ADR 描述最终真实架构而不是已删除的双后端和 OpenCode 内嵌设计。
51. 作为贡献者，我想让每个迁移 ticket 都能在一个新上下文中完成和验证。
52. 作为审阅者，我想让旧实现只在新实现通过 canonical acceptance 后删除。
53. 作为安全责任人，我想让 fixtures 全部为合成数据，不提交真实 session、rollout、数据库或凭据。
54. 作为项目所有者，我想让任何 usage 统计规则最终只有一个生产实现。

---

## Implementation Decisions

- Go 是唯一 canonical backend。
- 当前项目的正式 source 仅为 Pi 与 Codex；All 仅表示 Pi + Codex。
- normalized SQLite ledger 是 usage 唯一事实中心。
- Pi 四载体、billable/cost/failed、forkTs、双账本、指纹增量、半行游标等既有决策保持不变。
- ADR-0002 / ADR-0004 的 input/cacheRead/totalTokens/reasoning 语义保持不变。
- Source adapter 是 source-specific semantics 的终点；Query 不理解上游原始格式。
- Query Engine 是 totals/sessions/requests/groups/period/detail/meta 的唯一生产统计 seam。
- Query 与 Refresh 分离；Query 不扫描文件、不解析源、不写 DB。
- Refresh 失败保留上一成功 snapshot，并通过 diagnostics/meta 暴露。
- Watch 只检测变化、触发 Refresh、重新 Query。
- CLI / HTTP 是薄 adapter，不重算业务统计。
- WebUI 由 Go server 内嵌，只保留一个人工维护源。
- TypeScript 在迁移完成前冻结新增生产能力，只作为 oracle。
- canonical golden contract 替代长期跨 runtime parity。
- **OpenCode 作为独立产品从 token-analyzer 核心抽离，不做 token-analyzer Go parity。**
- 仓库根下建立独立 `opencode-analyzer/` 项目边界，承接现有 OpenCode RPC client、Seroval、credential/cookie、workspace discovery、sync、storage、cursor、lock、export、HTTP API、WebUI audit 与相关测试/文档。
- `opencode-analyzer/` 必须自包含：不得 import token-analyzer 的 `internal/*`，不得依赖 token-analyzer 的 ledger/query/server 作为运行前提；token-analyzer 也不得反向依赖它。
- token-analyzer 不保存、不读取、不要求任何 OpenCode 凭据，不提供 `source=opencode`，也不把 OpenCode 加入 All。
- 本迁移只要求 standalone folder boundary 和现有能力保留；独立仓库、独立域名、部署流水线与正式上线属于后续 OpenCode 项目工作。
- 不为了迁移提前做无关的大规模目录 rename。
- module/repository/release 命名统一放在收尾 contract 阶段。

---

## Testing Decisions

**什么算好测试**

- 测试外部可观察行为与稳定领域 seam，不锁定私有实现。
- 主 seam 是 Refresh → Ledger → Query → CLI/API/WebUI。
- Query Engine 应可在完全没有 Pi/Codex 文件系统访问的环境中，只凭 ledger fixture 测试。
- adapter 测试只覆盖协议转换/capability，不重复领域数字断言。
- synthetic fixture 是 CI 唯一数据来源。

**Seam ①：Refresh + Ledger + Query**

- Pi 覆盖四载体、failed/aborted、fork、task、dedup、incremental、rewrite、time/model/pricing。
- Codex 覆盖 durable usage、cache、snapshot-only、diagnostics、plain/zstd、revert/archive。
- 验证 totals、sessions、支持的 requests、groups、period、meta。
- 重复 refresh 幂等；Query 前后 ledger 不写入。

**Seam ②：Ledger-only Query Engine**

- 固定 snapshot 验证 source/model/cwd/time filter、排序、分页、costStatus、All、detail。
- 测试环境无需 source 文件系统。

**Seam ③：CLI / HTTP**

- CLI/API 对同一 query 映射到同一 domain result。
- HTTP GET 不触发 refresh/write。
- unsupported source/action 返回明确错误。
- refresh 失败仍服务旧 snapshot。

**Seam ④：Watch**

- 文件变化 → Refresh → Query 后的 totals 与普通 query 完全一致。
- Watch 不存在独立 token/cost 计算测试，因为迁移后不应有该逻辑。

**Seam ⑤：WebUI**

- source selector 与 Go meta.sources 一致。
- Pi detail/rename/subagent 保持现有行为。
- Codex/All 不支持动作明确禁用。
- Go binary 独立 serve WebUI，无 Node runtime。

**Seam ⑥：OpenCode standalone boundary**

- token-analyzer 在不存在 `opencode-analyzer/` 目录、OpenCode 配置、cookie 或网络访问的环境中仍完整运行。
- token-analyzer 的 CLI help / API routes / WebUI 不暴露 OpenCode 专用能力。
- `opencode-analyzer/` 拥有独立入口、依赖、storage、API/UI 和测试边界，不引用 token-analyzer `internal/*`。
- 将 `opencode-analyzer/` 整目录移出当前仓库后，token-analyzer 的 Pi/Codex canonical contract 不变；反过来，OpenCode 项目也不需要 token-analyzer runtime 才能运行其自身功能。

**迁移期 parity**

- TypeScript 只在删除前作为 Pi 行为 oracle。
- 一旦 canonical expected + Go tests 覆盖对应行为，就不再依赖 TS 判断正确性。

---

## Out of Scope

- 将 `opencode-analyzer/` 正式拆到新 Git 仓库、配置独立域名/CI/CD、部署上线。
- 保证旧 OpenCode CLI/API/WebUI 集成的兼容性。
- 将 OpenCode 改造成正式 source 或加入 All。
- 重新定义 Pi 四载体、fork、双账本、cacheRate、totalTokens 等已接受规则。
- 为 Codex 新增当前明确不支持的 requests 或 session management。
- 引入新的前端框架或视觉重设计。
- 更换 SQLite 技术栈。
- 将 pi-switch 网关变成数据源。
- 解决所有历史数据库兼容问题。
- 把真实 session、rollout、DB、cookie/token 放入仓库。
- 新增模型定价或订阅额度功能。

---

## Further Notes

- 本 spec 的完成标准不是“没有 .ts”，而是 token-analyzer usage 统计领域只剩一个生产实现，并且 OpenCode 已成为同仓独立项目边界。
- OpenCode 的解除耦合是**抽离并保留**，不是删除：先迁入 `opencode-analyzer/`，未来再整目录拆仓并独立上线；本迁移不强制其后端语言跟随 token-analyzer 的 Go-only 决策。
- 迁移采用 expand → migrate → contract：先冻结 contract，再迁 Pi/Codex，再收口生命周期与 server，最后删除 TS。
- 现有审计报告与执行计划是背景材料；本 spec 与 tickets 是规范性执行入口。
- 按仓库安全规则，实际实现 ticket 时 build / test / lint / typecheck 仍需当次明确授权。
