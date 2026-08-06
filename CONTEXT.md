# CONTEXT — token-analyzer 领域术语表

> 单上下文仓库。词汇表供实现/诊断/审查命名时使用；命名请用本文术语，勿漂移到同义词。
> 关联决策见 `docs/adr/`。

## 数据域

- **会话文件**：`~/.pi/agent/sessions/` 下递归收集的 `*.jsonl`；首行 `type == "session"` 才是合法会话（`type: message`/`custom` 单条导出视为残留，跳过）。
- **header**：会话文件首行 JSON entry，权威字段 = `id`（会话 ID）、`timestamp`（会话创建时间）、`cwd`（项目归属）、`parentSession`（fork 标记，见下）。
- **项目归属（cwd）**：以 header `cwd` 为权威（完整绝对路径）；目录名是 cwd 的有损编码（`--` 包裹、`/`→`-`），不可反解，仅展示/辅助分组。聚合按规范化 cwd（resolve 去尾斜杠/符号链接）。
- **计入口径消息（口径 A）**：`type == "message"` 且 `message.role == "assistant"` 且 `message.usage != null`。toolResult / compaction / branch_summary 等一律不计入。

## 统计窗口

- **totals（总窗口）**：全量计入口径消息的汇总（requests / input / output / cacheRead / cacheWrite / reasoning / totalTokens / cost / cacheRate）。
- **sessions（会话级窗口）**：每会话一行（按 header 归属）。
- **requests（单请求级窗口）**：每条计入口径消息一行（按消息 timestamp 归属）。
- **cacheRate**：先求和分子分母再除（cacheRead / (input + cacheRead + cacheWrite)）；分母为 0 记 0。
- **totalTokens**：input + output + cacheRead + cacheWrite（组件和为准）。

## 时间归属

- **消息级归属**（webui 统计端点 totals/groups/period/requests 的 since/until 语义）：按消息 `timestamp` 闭区间过滤，跨天会话中落在范围内的请求/消耗计入当天。用户决策（ticket 22/23）。
- **会话级归属**（sessions 明细端点、CLI `--since/--until`）：按会话 header timestamp 过滤，跨天会话整段归 header 日。口径 A 原语义。
- 两个层级在跨天场景数字有**预期差异**（webui 统计 vs CLI），spec 已记录。

## Fork 会话（ticket 25 已实现）

- **fork 会话**：header 含 `parentSession`（父会话文件绝对路径）的会话，由 pi 的 fork 功能创建，复制父会话历史消息（保留原 timestamp 与 usage）。
- **复制历史**：fork 会话中 `message.timestamp < header.timestamp`（fork 创建时间）的消息——其 usage 已在父会话统计过，**fork 本身未消耗这些 token**。
- **forkTs**：fork 创建时间 = fork 会话 header.timestamp，去重切分边界。
- **fork 去重**：analyzeFile 数据层剔除复制历史，fork 后新增消息（ts ≥ forkTs）保留；CLI/webui 一致（数据读取层）。嵌套 fork 链按各层自身 forkTs 自动正确。

## 字段语义

- **input（非缓存输入）**：usage.input，本次请求未命中缓存的新输入。
- **cacheRead（缓存命中输入）**：usage.cacheRead，独立维度；单请求总输入 = input + cacheRead（与网关 prompt 一致）。
- **总输入（webui 展示语义，ticket 24）**：input + cacheRead；webui 卡片/分组/明细的「输入」列显示总输入，与 pi-switch 网关 Input 对齐；CLI 与导出 JSON/CSV 保持原始字段。
- **output / reasoning**：usage.output（输出）、usage.reasoning（推理，output 子集，不重复累加）。

## 外部对比基准（非数据源）

- **pi-switch 网关**：转发请求日志（`~/.pi-switch/requests.log`），统计口径 total = 总输入 + output。是**对比基准**，不是 token-analyzer 的数据源——token-analyzer 数据只来自 session 目录（用户约束）。两者覆盖范围结构性不同（pi 直连请求只在 session 目录、其他客户端请求只在网关），fork 去重后同源请求数字对齐（差异 ≈0.2%）。
