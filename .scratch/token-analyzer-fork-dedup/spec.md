# Token Analyzer — Fork 会话去重（Spec）

**状态**: ready-for-agent（grill-to-spec 综合已决 ticket 25 + 打磨共识产出；核心逻辑已实现于 analyze.ts，本 spec 补充 watch 去重与文档化）

**前置**: 数据域与统计口径见 `.scratch/token-analyzer/spec.md`；领域术语见 `CONTEXT.md`；决策记录见 `docs/adr/0001-fork-session-dedup.md`（Accepted）。

---

## Problem Statement

pi 的 fork 功能从既有会话复制出一份分支继续对话。fork 会话文件（header 含 `parentSession` 字段）**逐条复制父会话历史消息**，保留原始 timestamp 与 usage。token-analyzer 按口径 A 统计这些消息时，复制历史的 token **重复计入**（其消耗已在父会话统计过，fork 本身未产生这些请求）：

- 全库 11 个 fork 会话、734 条复制历史虚高；其中 019fd362 的 261 条全是复制历史（39.5M token），使「今天」统计虚高约 9%。
- 对比基准 pi-switch 网关从未见过 fork 会话的请求（fork 复制不产生请求），故网关统计天然不含重复——两个工具差异 9%。

用户需要：fork 复制历史的 token 不重复计费，使 token-analyzer 统计与 pi-switch 网关对齐。

## Solution

在**数据读取层**对 fork 会话做**复制历史剔除**，三种消费路径（静态 CLI / webui serve / `--watch` 实时监控）口径统一：

- **判定**：header 含非空 `parentSession` → fork 会话；`forkTs = header.timestamp`（fork 创建时间，UTC 解析）。
- **剔除**：`message.timestamp < forkTs` 的消息（复制快照，usage 已在父会话统计）跳过。
- **保留**：`message.timestamp >= forkTs` 的消息（fork 后真正新增的消耗）；无效 timestamp 保守保留（不误剔）。
- **嵌套 fork**：链式按各层自身 forkTs 切分，父会话历史只在真实发生处统计一次。
- **范围**：CLI 与 webui 一致（数据层修正）；`--watch` 增量读取器同样剔除（首次读取时解析 header 得 forkTs）。

## User Stories

1. 作为 pi 用户，我想 fork 出的对话不重复统计其复制历史 token，以便总消耗与真实账单一致。
2. 作为 pi 用户，我想 fork 后继续对话的新增消息仍被统计，以便 fork 分支的真实消耗不被遗漏。
3. 作为 pi 用户，我想 CLI 与 webui 统计一致，以便同一数据不同工具数字对得上。
4. 作为 pi 用户，我想 `--watch` 实时监控与静态统计一致，以便监控值与快照值可比。
5. 作为 pi 用户，我想统计结果与 pi-switch 网关对得上（同源请求差异 ≈0.2%），以便交叉核验。
6. 作为开发者，我想嵌套 fork（fork 的 fork）自动正确去重，以便不因链式复制重复计费。
7. 作为开发者，我想无效 timestamp 的消息保守保留，以便不误剔真实消耗。
8. 作为开发者，我想去重逻辑在数据读取层实现，以便 CLI/webui/缓存共用同一口径。

## Implementation Decisions

- **判定依据**：header `parentSession` 非空即 fork 会话（pi 的唯一 fork 标记，已核实无其他标记字段）；`forkTs` = header timestamp 按 UTC 解析。
- **去重粒度（时间戳方案）**：以 `message.timestamp < forkTs` 判定复制历史，而非消息 id 跨会话去重——零额外成本（单文件解析，不读父会话）、已验证当前 pi 复制保留原 timestamp；消息 id（8 位短 hex）跨非 fork 会话有碰撞风险，且需读父会话文件（跨文件解析复杂度），故不采用。
- **边界语义**：`ts < forkTs` 剔除、`ts >= forkTs` 保留、无效 ts 保守保留（宁可多计不漏计）。
- **影响范围（数据层修正）**：静态 CLI 与 webui 都去重——复制历史是重复记录而非筛选口径，两工具必须一致（单一事实源原则）。
- **watch 去重**：增量读取器首次读取文件时解析 header（parentSession/timestamp → forkTs），`readEntriesFrom` 过滤复制历史；已跟踪文件的增量追加路径不受影响（复制历史只存在于首次全量读）。
- **缓存交互**：serve 的 readSessionFilesCached 缓存 analyzeFile 结果（已去重），无额外处理。
- **显示名**：fork 会话显示名保持现状（与父会话同主题是自然语义），不添加 fork 标记——作为独立 issue 另议。

## Testing Decisions

- **好测试的标准**：只断言外部行为（fork 会话统计出的计入口径消息数量、各消费路径输出），不测实现细节。
- **主 seam（analyzeFile 数据读取层）**：直接以 fork fixture（header 含 parentSession + 复制历史消息 + fork 后新增消息）驱动，断言 items 数量与保留的时间戳；非 fork 会话（无 parentSession）不受影响；CLI（runCli）与 webui 端点（serve + fetch）一致性断言。既有先例：`test/23-fork-dedup.test.ts`。
- **补充 seam（watch 增量读取器）**：以单步驱动模式（既有先例 `test/05-s5-watch-step.test.ts`）验证 fork 会话首次读取时剔除复制历史、增量追加正常计入。
- **回归保护**：既有 114 用例全绿（fixture 无 parentSession 不受影响）。

## Out of Scope

- **fork 会话显示名标记**（父/子会话区分）——独立 issue 另议。
- **消息 id 级去重**——时间戳方案失效时才考虑的升级预案（ADR-0001 已记录）。
- **pi 直连请求（未走网关的会话）与网关覆盖差异**——结构性差异，不在去重范围。
- **网关（pi-switch）侧改动**——token-analyzer 数据只来自 session 目录，不改网关。

## Further Notes

- 验证：fork 去重后「今天」total 416.9M vs pi-switch 网关 417.6M（差异 0.2%，剩余为网关 unlabeled 其他客户端请求与请求/消息时间边界，结构性）。
- 前提：pi fork 复制消息时保留原始 timestamp 与消息 id（已实测 549/549 共享 id、复制消息 ts 全 < forkTs）。若 pi 未来重写复制消息 timestamp，时间戳方案失效，需切换消息 id 级去重。
- 嵌套 fork 实测存在（2 个），链式切分已正确处理。
