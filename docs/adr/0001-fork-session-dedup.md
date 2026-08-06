# ADR-0001 — Fork 会话去重（复制历史不重复计费）

- **状态**: Accepted（2026-08-06，ticket 25）
- **影响组件**: `src/analyze.ts`（analyzeFile 数据读取层）、`src/watch.ts`（--watch 增量读取器）、CLI / webui / watch（共用口径）
- **触发**: 用户报告总览 token 统计与 pi-switch 网关对不上（019fd362 会话 39.5M token 虚高）

## 背景

pi 的 fork 功能从既有会话复制出一份分支继续对话。fork 会话文件（header 含 `parentSession` 字段 = 父会话文件绝对路径）**逐条复制父会话历史消息**，且**保留原始 timestamp 与 usage 字段**。token-analyzer 按口径 A 统计这些消息的 usage，导致：

- fork 会话中复制的历史消息（其 token 已在父会话统计过）被**重复计入**；
- 实测 11 个 fork 会话共 734 条复制历史；其中 019fd362 的 261 条全是复制历史（39.5M token），使「今天」统计虚高约 9%。

对比基准 pi-switch 网关（`~/.pi-switch/requests.log`）从未见过 fork 会话的请求（fork 复制不产生请求），故网关统计天然不含这些重复——两个工具差异 9%。

## 决策

在 `analyzeFile`（数据读取层）对 fork 会话做**复制历史剔除**：

- 判定：header 含非空 `parentSession` → 该会话是 fork 会话；
- 边界：`forkTs = header.timestamp`（fork 创建时间，UTC 解析）；
- 剔除：`message.timestamp < forkTs` 的消息（复制快照，usage 已在父会话统计）跳过；
- 保留：`message.timestamp ≥ forkTs` 的消息（fork 后真正新增的消耗）；
- 消息 timestamp 无效（NaN）时保守保留（不剔除）。

放数据读取层而非端点层，使 **CLI 与 webui 口径一致**（CLI 统计同样避免虚高）。

**--watch 实时监控同口径**（issue 01-watch-fork-dedup）：增量读取器（`src/watch.ts` `readEntriesFrom`）首次读取会话文件时解析 header（parentSession + timestamp → forkTs），复制历史（ts < forkTs）不计入实时 totals；forkTs 随文件跟踪状态（FileState）持久化，文件替换/重读路径**复用初始 forkTs**（复制历史不重复计入）；追加路径不受影响（复制历史只存在于首次全量读）。过滤语义与 analyzeFile 完全一致（同 `parseUtcTimestamp` UTC 解析、`ts < forkTs` 剔除、无效 ts 保守保留）。
## 后果

**正面**

- fork 去重后「今天」total 与 pi-switch 网关对齐：416.9M vs 417.6M（差异 0.2%，剩余为网关 unlabeled 其他客户端请求与请求/消息时间边界，结构性）。
- 嵌套 fork（fork 的 fork）自动正确：每层按自身 forkTs 切分，父会话历史只在其真实发生处统计一次。
- 原始 spec（`.scratch/token-analyzer/spec.md`）早有「parentSession 续接/子会话链去重」列为可选增强，本 ADR 落实该预留设计点。

**负面 / 约束**

- 依赖 timestamp 边界可靠性：前提是 pi fork 复制消息时**保留原始 timestamp**（已验证：019fd362 复制消息 ts 全 < forkTs，且与父会话共享消息 id）。
- 若 pi 未来 fork 时重写消息 timestamp（复制为 fork 时刻），本规则会失效，需改为消息 id 跨会话去重（备选方案）。
- fork 会话的 `firstUserText`（显示名数据源）仍取自复制历史（对话主题相同，合理），未随去重调整。
- watch 替换/重读路径按 issue 决策**无条件复用初始 forkTs**：已跟踪文件被替换为不同会话（不同 header）时沿用旧 forkTs 过滤。已跟踪文件首行为空行/仅 header 无尾换行等边缘形态可能使 forkTs 解析失败（保守不去重），实际无影响（无消息可过滤，fork 后追加 ts 恒 ≥ forkTs）。
## 备选方案（未采纳）

- **消息 id 全局去重**：跨会话按消息 id 只统计一次。更精确（不受时间边界影响），但 pi 消息 id 为短 hex（8 位），跨会话可能碰撞导致误删；且需全局集合，破坏单文件独立解析架构。
- **端点层剔除（仅 webui）**：CLI 统计仍虚高，口径不一致，弃。
