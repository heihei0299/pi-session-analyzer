# ADR-0004 — 跨源 input / cacheRead 语义归一（input 恒为非缓存输入）

- **状态**: Accepted（2026-09-09，Codex 真实格式对齐 spec 用户确认）
- **影响组件**: Codex rollout 适配（usage 映射）、domain 聚合（ADR-0002 收尾计算）、totals/sessions/groups/period 四窗口、CLI/JSON/CSV/WebUI 展示

## 背景

同一批 token 在不同数据源的上游字段语义并不相同：

- Pi 会话的 `usage.input` 是**非缓存**输入（Anthropic 语义，`cacheRead`/`cacheWrite` 独立成列）；
- Codex 的 `usage.input_tokens` 是**含缓存**的 prompt 总量，`cached_input_tokens` 是其子集。

Codex 适配最初直接把 `input_tokens` 记作 `input` 再把 `cached_input_tokens` 叠加为 `cacheRead`，
于是同一份数据被计入两次：本机真实 Codex home 1109 条 usage event 汇总 `totalTokens` 285,975,614，
而 Codex 自报合计 145,748,798（虚高 96.2%）；`cacheRate` 由真实的 96.6% 被算成 49.1%。
更严重的是 Pi / Codex / All 三个源的 `input` 列混着两种语义，跨源对账必然失准。

## 决策

`input` 在所有数据源上**恒为非缓存输入**：Codex 侧记账时执行饱和减法
`input = input_tokens - cached_input_tokens`（不为负），`cacheRead = cached_input_tokens`。
ADR-0002 的公式不变：`totalTokens = input + cacheRead + output`，因此 Codex 窗口的 `totalTokens`
等于上游自报的 `usage.total_tokens`（`input_tokens + output_tokens`）——唯一例外是口径异常的记录（见下）。

口径异常（`cached_input_tokens > input_tokens`，例如第三方 provider 语义不同）不静默修正：
数值按 0 饱和并产生诊断；此时该行 `totalTokens = cached + output` 会大于上游自报值，
等式只在正常口径下成立，异常以诊断而非静默错数现形。`cacheWrite` 与 `reasoning` 保持独立成列、不参与 `totalTokens`。

## 后果

**正面**

- Codex 窗口可直接与 Codex 自报数字对账；Pi/Codex/All 的同名列语义一致，`all` 源合计不双算。
- 缓存命中率恢复真实量级（本机 96.6% 而非 49.1%），成本判断不再被误导。
- 第三方 provider 的口径差异以诊断现形，而不是变成静默错数。

**负面 / 约束**

- 依赖「`input_tokens` 含缓存」这一上游约定；上游若改变语义，需要按 `cli_version` 重新确认（诊断会把 cached > input 的情形暴露出来）。
- 本修正不做历史账本迁移：`proxy_request_logs` 里已存在的 codex 行按 `response_id` 去重、重扫只做跳过，不会改写。实际不构成用户负担——发现契约（ticket 01）与本修正同批交付，在它之前真实 Codex home 一个 rollout 也发现不了，因此不存在用旧口径写入的真实 codex 行；只有同一开发期内用旧口径写过的临时库需要换库或删行后重扫。

## 备选方案（未采纳）

- **直接把上游 `usage.total_tokens` 写进账本**：能省一次减法，但会让 `totalTokens` 变成「上游字段搬运」，与 ADR-0002 的计算口径和 Pi 侧聚合分叉，也让 cacheWrite/reasoning 的边界失去统一解释。
- **保持 `input` 含缓存、只把 `cacheRead` 排除出 `totalTokens`**：能修总量，但 Pi/Codex 的 `input` 列语义仍然不一致，跨源比较依旧失真。
