# 09 — 请求模型归属研究

Type: research
Status: resolved

## Question

每个请求用的是哪个模型？`message.usage` **没有** model 字段——按模型维度分析需要可靠的模型归属途径。

具体子问题：
1. message / session 结构中 model 信息在哪？（`AssistantMessage` 有无 `model` 字段？session header？compaction 记录？）
2. 不同来源的 model 信息对比（pi 配置 / provider 响应 / 会话记录），哪个权威
3. compaction / branch_summary 的 usage 归属哪个模型
4. 对分析工具的推荐：**请求级**模型归属 vs **会话级**模型归属（一个会话可能混合多模型吗？）

## 背景

用户于 2026-08 明确要求分析维度"按模型"拆分（map Destination 修订）。「usage 字段语义研究」（ticket 01）已 resolve，其 Answer 未覆盖 model 字段——本 ticket 补足。

## 调研指引

- 权威来源：pi 源码 `pi-ai/dist/types.d.ts`（message 类型定义）、`docs/session-format.md`、`~/.local/share/fnm/node-versions/v24.16.0/installation/lib/node_modules/@earendil-works/pi-coding-agent/`
- 数据源：`/home/shial/.pi/agent/sessions/` 会话样本（直接 bash 绝对路径访问，勿用 fffind 索引）
- 每个结论给出证据：源码路径 + 行号，或会话文件样本

## Answer

### 1. model 信息在哪：`AssistantMessage.model`（请求级，与 usage 同对象）

- **每个 assistant message 自带 `model: string`**，与 `usage` 在同一对象里——`pi-ai/dist/types.d.ts`（`node_modules/@earendil-works/pi-ai/dist/types.d.ts`，约 L296）：`AssistantMessage { role; content; api; provider; model: string; responseModel?; usage; ... }`。
- 另有可选 `responseModel?: string`：provider 流式响应返回的实际模型名，写入点在 `pi-ai/dist/api/openai-completions.js:315`（`output.responseModel ||= chunk.model`）。
- **session header 无 model**：真实样本 `--home-shial-Project-pi-session-anylize--/2026-08-05T10-32-38-135Z_...jsonl` 的 session entry 只有 `{type, version, id, timestamp, cwd}`。
- 模型切换由独立事件记录：`model_change` entry（`{type, provider, modelId}`，见 session-format.md「ModelChangeEntry」；session-manager.d.ts:32-34）。
- compaction/branch_summary entry 只带 `usage`，**不带 model**（session-manager.js:805-819 `appendCompaction(...)` 与 session-format.md「CompactionEntry/BranchSummaryEntry」）。

### 2. 权威来源：会话记录（`AssistantMessage.model`）

- 会话记录的 `model` 是 pi 实际发起请求时用的模型 id（`types.d.ts` 定义 + `agent-session.js` 各请求路径以 `this.model` 调用 `stream()`），反映真实发生的事实，覆盖配置与响应的偏差。
- pi 配置（models.json/settings）只描述默认/当前模型，中途会变；provider 响应（`responseModel`）是 provider 侧模型名（样本中为去前缀的 `deepseek-v4-flash`），可作交叉校验但不是请求意图。
- 真实样本佐证：`--home-shial-Project-pi-switch--/2026-08-02T01-51-15-012Z_...jsonl` 中 assistant message 的 `model` 与 `model_change` 事件完全对应。

### 3. compaction / branch_summary 的 usage 归属：执行时刻的会话当前模型（entry 不记 model）

- 源码：`dist/core/agent-session.js`——手动 compact L1423 `compact(preparation, this.model, ...)`；自动 compact L1657 同；branch_summary L2365-2369 `const model = this.model` 传入 `generateBranchSummary`。即 **summary 生成用执行 compact/branch 那一刻的当前模型**。
- entry 只写 `usage`（`appendCompaction` 签名与 `compact()` 返回 `{ summary, firstKeptEntryId, tokensBefore, usage, details }`），无 model 字段；文档亦注明该 usage「included in session token and cost totals」。
- 工具处理：compaction/branch_summary 的 usage 按时间线归属——取该 entry 之前最近的 `model_change` 或相邻 assistant message 的 `model` 推断。当前 212 个会话样本中无原生 compaction（仅 magic-context 扩展生成的 `fromHook` compaction，无 usage 无 model），此路径为防御性兜底。

### 4. 推荐：请求级归属（一个会话确实会混合多模型）

- 212 个 JSONL 中有 **8 个会话为混合模型**，如 `--home-shial-Project-matt-skills--/2026-08-02T03-56-12-068Z_...jsonl` 同会话出现 `mimo-v2.5` × 4 与 `opencode-go/deepseek-v4-flash` × 3，且伴随两次 `model_change`。会话级归属必然错分。
- `AssistantMessage.model` 与 usage 同对象、逐请求存在，请求级归属零额外成本且精确。推荐：**按每条 assistant message 的 `model` 归属其 `usage`**；`responseModel` 可用于 provider 别名归一化；`model_change` 仅作为事件轴展示，不参与归属计算。
