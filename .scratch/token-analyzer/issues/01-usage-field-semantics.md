# 01 — usage 字段语义研究

Type: research
Status: resolved

## Question

pi 会话 JSONL 中 `message.usage` 各字段的**精确定义**是什么？

具体子问题：
1. 各字段语义：`input` / `output` / `cacheRead` / `cacheWrite` / `reasoning` / `totalTokens` / `cost`（含 `cost` 内各子字段）
2. 哪些 message 行携带 usage？（role / type / 状态条件）——勘察基线显示 2695/6554 行带 usage
3. `totalTokens` 的组成公式？——勘察基线显示 `totalTokens = input + output + cacheRead + cacheWrite`，需源码佐证
4. `reasoning` 与 `output` 的关系？——勘察基线显示 `reasoning` 恒 < `output`（2643 例），疑似子集

## 调研指引

- 数据源：`~/.pi/agent/sessions/`（212 个 JSONL 会话文件，可采样）
- 权威来源：pi 源码（`~/.local/share/fnm/node-versions/v24.16.0/installation/lib/node_modules/@earendil-works/pi-coding-agent/`）与 pi 文档（`docs/` 下 environment-variables、sdk 等）
- 每个结论给出证据：源码文件路径 + 行号，或会话文件样本

## Answer

### 结论摘要

- `usage` 出现在会话 JSONL 的 message 行中时，**几乎只挂在 `role: "assistant"` 的消息**上（类型必填字段）；`toolResult` 消息**可选**携带（工具内部发生嵌套 LLM 工作）；`user` 消息**从不**携带。此外 `compaction` 与 `branch_summary` entry 顶层**可选**携带 `usage`（生成摘要的 LLM 用量）。
- `totalTokens = input + output + cacheRead + cacheWrite` **成立**（对 Anthropic 类 provider 由 pi 源码显式计算；对 OpenAI 类直接取 provider 报告值，会话统计与上下文估算均用同一公式兜底）。
- `reasoning` 是 **`output` 的子集**：`output` 已包含 reasoning token（源码注释与 API 拆解双重证实）；勘察基线「reasoning 恒 < output」与该语义一致。
- `cost` 不是 provider 报告值，而是 **pi 按模型费率表自行估算**（`token 数 × 费率 / 1e6`），全 0 表示该模型费率表中价格为零/未配置。

---

### 1. 各字段精确定义

权威类型定义在 **`pi-ai/dist/types.d.ts:251-272`**（`interface Usage`）：

| 字段 | 类型 | 语义（源码注释原文/行为） |
|------|------|------|
| `input` | number | 本次请求**未命中缓存的输入 token**。OpenAI 适配器中为 `input_tokens − cached − cache_write`（OpenAI 把缓存 token 计入 input，pi 减去两者得到纯新输入；`openai-responses-shared.js:414-416`） |
| `output` | number | **输出 token，已包含 reasoning**（见第 4 节） |
| `cacheRead` | number | **命中缓存**的输入 token（`cache_read_input_tokens`） |
| `cacheWrite` | number | **写入缓存**的输入 token（`cache_creation_input_tokens`） |
| `cacheWrite1h` | number, 可选 | `cacheWrite` 中按 1h 保留期写入的子集；**仅 Anthropic 报告**（types.d.ts:256-257 注释："Subset of `cacheWrite` written with 1h retention. Only Anthropic reports this split."） |
| `reasoning` | number, 可选 | 推理/思考 token。**是 `output` 的子集**："This is a subset of `output`: `output` already includes these tokens"（types.d.ts:258-263）。暴露推理拆分的 provider 填数字（可能为 0），不暴露的为 `undefined` |
| `totalTokens` | number | 总 token 数（组成见第 3 节） |
| `cost.input` | number | `input × inputRate / 1e6`（见第 5 节） |
| `cost.output` | number | `output × outputRate / 1e6` |
| `cost.cacheRead` | number | `cacheRead × cacheReadRate / 1e6` |
| `cost.cacheWrite` | number | `(cacheWriteRate × shortWrite + inputRate × 2 × longWrite) / 1e6`（Anthropic 1h 缓存写按 2 倍输入费率计） |
| `cost.total` | number | 上述四项之和 |

> 注：`docs/session-format.md` 中的 Usage 类型（少 `cacheWrite1h`/`reasoning`）是文档侧旧版简化，以 `pi-ai/dist/types.d.ts` 为准。

### 2. 哪些行携带 usage

**类型层面**（`pi-ai/dist/types.d.ts:264-289` + `docs/session-format.md`）：

- `UserMessage`：**无** `usage` 字段（`types.d.ts:264-266`）；
- `AssistantMessage`：**必填** `usage: Usage`（`types.d.ts:282`）；
- `ToolResultMessage`：**可选** `usage?: Usage`，注释 "Nested LLM work performed by the tool"（`docs/session-format.md` ToolResultMessage 定义）——仅当工具内部调用了 LLM 才带；
- `compaction` / `branch_summary` entry：顶层**可选** `usage`，注释 "LLM usage from generating the summary; included in session token and cost totals"（`docs/session-format.md` 两处）——**计入**总 token/花费统计；
- `retainedTail` 内嵌的 assistant 消息同样带 usage（`docs/session-format.md` compaction 示例）。

**运行时层面**：

- assistant 消息的 usage 由 provider 适配器组装：初始化为全 0（`anthropic-messages.js:333-346`），`message_start` 事件填 input/output/cacheRead/cacheWrite（`anthropic-messages.js:388-393`），`message_delta` 更新 reasoning 与 totalTokens（`anthropic-messages.js:550-561`）；
- 中止/出错的 assistant 消息也带 usage，值为全 0 的 `EMPTY_USAGE`（`pi-agent-core/dist/agent.js:341-350`）；
- 会话统计只认 `type === "message" && role === "assistant"` 的 usage，其余（含 toolResult、compaction、branch_summary）被忽略（`pi-agent-core/dist/harness/session/jsonl-storage.js:233-240`）；
- 这解释了勘察基线 2695/6554（≈41%）：assistant 消息约占消息行四成，user 行恒无 usage，toolResult 行通常也无（工具嵌套 LLM 罕见）。

### 3. `totalTokens` 组成公式（已验证）

- **Anthropic 适配器显式计算**：`anthropic-messages.js:559-561` — 注释 "Anthropic doesn't provide total_tokens, compute from components"，即 `totalTokens = input + output + cacheRead + cacheWrite`；
- **OpenAI 适配器取 provider 值**：`totalTokens: response.usage.total_tokens || 0`（`openai-responses-shared.js:426`）；`openai-completions.js:1078` 则自算 `input + outputTokens + cacheReadTokens + cacheWriteTokens`；
- **会话统计自算**：`jsonl-storage.js:250` — `totalTokens += usage.input + usage.output + usage.cacheRead + usage.cacheWrite`（不读字段值，直接累加组件）；
- **上下文估算兜底**：`pi-ai/dist/utils/estimate.js:4` — `usage.totalTokens || usage.input + usage.output + usage.cacheRead + usage.cacheWrite`。

样例验证：`input=16172, output=212, cacheRead=3456, cacheWrite=0` → `16172+212+3456+0 = 19840 = totalTokens` ✓（Anthropic 风格字段集）。

> 对 spec 的含义：聚合时**不要信任 `totalTokens` 字段本身**（OpenAI 可能不精确等于组件和），应始终按 `input + output + cacheRead + cacheWrite` 计算，与 pi 自身统计口径（jsonl-storage.js）保持一致。

### 4. `reasoning` 与 `output` 的关系（子集，已证实）

- 类型注释（`types.d.ts:258-263`）："This is a subset of `output`: `output` already includes these tokens"；
- Anthropic：reasoning 取自 `message_delta` 的 `output_tokens_details.thinking_tokens`，源码注释 "a subset of output_tokens"（`anthropic-messages.js:550-553`）；
- OpenAI：reasoning 取自 `output_tokens_details.reasoning_tokens`（`openai-responses-shared.js:426`）。

因此 `output = 文本输出 + reasoning`，`reasoning ≤ output` 恒成立。勘察基线「reasoning 恒 < output（2643 例）」符合：reasoning 一般占 output 一部分；严格小于是因为文本输出非零，极端情况下 reasoning 可等于 output（全推理无文本）。

> 对 spec 的含义：若单列「推理」指标，应保持 **B 口径**（推理独立成列但属于输出的一部分），避免把推理与输出相加导致重复计数；`reasoning` 为 `undefined` 时按 0 处理（provider 不报告）。

### 5. `cost` 语义（pi 自估，非 provider 报告）

- `calculateCost(model, usage)` 在 `pi-ai/dist/models.js:371-388`：按模型费率表（`model.cost`，每百万 token 价格）估算四项费用，`total` 为四项之和；
- 1h 缓存写按 2 倍输入费率计（`models.js:379-380`，仅当 `cacheWrite1h` 存在时，即 Anthropic）；
- 费率按 `input + cacheRead + cacheWrite` 分级（`models.js:372-376`，tier 阈值）；
- 样例 `cost` 全 0 → 该模型（或该版本费率表）无价格配置或价格为零，不代表免费。

> 对 spec 的含义：若做「花费」指标，直接累加 `cost.total` 即可（pi 已统一口径）；注意零花费可能源于费率缺失，而非真实免费。

### 证据索引

| 结论 | 证据 |
|------|------|
| Usage 类型与字段注释 | `pi-ai/dist/types.d.ts:251-272` |
| AssistantMessage.usage 必填 | `pi-ai/dist/types.d.ts:282` |
| ToolResultMessage.usage 可选 | `docs/session-format.md`（ToolResultMessage 定义） |
| compaction/branch_summary 可选 usage | `docs/session-format.md`（两处 "included in session token and cost totals"） |
| 失败消息带全 0 usage | `pi-agent-core/dist/agent.js:341-350`（EMPTY_USAGE） |
| Anthropic totalTokens 公式 | `pi-ai/dist/api/anthropic-messages.js:559-561` |
| OpenAI totalTokens 取 provider | `pi-ai/dist/api/openai-responses-shared.js:426` |
| 会话统计公式 | `pi-agent-core/dist/harness/session/jsonl-storage.js:250` |
| 上下文估算兜底公式 | `pi-ai/dist/utils/estimate.js:4` |
| Anthropic reasoning 子集 | `pi-ai/dist/api/anthropic-messages.js:550-553` |
| cost 估算逻辑 | `pi-ai/dist/models.js:371-388` |
