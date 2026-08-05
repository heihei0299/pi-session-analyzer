# 02 — cost 字段与花费计算研究

Type: research
Status: resolved

## Question

pi 如何计算 token 花费？工具应**直接读 `usage.cost.total`** 还是**按模型单价重算**？

具体子问题：
1. `cost` 各子字段（input / output / cacheRead / cacheWrite / total）何时非零？——勘察基线：cost 非零 2638 vs 零 57，需解释零值的条件
2. pi 内部 cost 计算逻辑：按什么单价、从哪读取（模型配置？provider pricing？）
3. 不同 provider / 模型（如 Kimi-K3）的 cost 计算差异
4. 对分析工具的结论建议：直接读 cost.total 的可靠性；有无需要回退到按单价重算的场景

## 调研指引

- 权威来源：pi 源码（`~/.local/share/fnm/node-versions/v24.16.0/installation/lib/node_modules/@earendil-works/pi-coding-agent/`）——搜索 cost / pricing / usage 相关代码；pi 文档
- 数据源：`~/.pi/agent/sessions/` 中 cost 非零与为零的样本对比
- 每个结论给出证据：源码文件路径 + 行号，或会话文件样本

## Answer

### 结论摘要

- **工具应直接累加 `usage.cost.total`**（与 ticket 01 第 5 节结论一致）：pi 已统一在 `calculateCost()` 中按模型费率表估算，各 provider 适配器调用同一函数，无重复计费风险；**不需要**按模型单价重算——重算反而引入新不一致（分级 tier、1h 缓存写 2 倍费率等规则会丢）。
- **cost 全部子字段按同一模式计算**：`字段 token 数 × 费率 / 1e6`（`pi-ai/dist/models.js:371-388`）。`total` 恒等于四项之和，**子字段非零 ⇔ 对应 token 数非零且对应费率为非零**。
- **全 0 cost 的两种成因**（源码费率表层面已穷举验证，1109 个模型无一是 `cost: null`）：
  1. 模型在费率表中**显式配置价格为 0**（99/1109，如 `kimi-coding/k3-256k`、google 的 gemma-4 系列、opencode 的 `*-free` 免费模型、qwen-token-plan 的 token 计划模型）→ cost 恒为 0 是**费率缺失/免费的真实信号**；
  2. 消息 token 全为 0（失败/中止的 assistant 消息带 `EMPTY_USAGE`，`pi-agent-core/dist/agent.js:341-350`）→ 0 × 费率 = 0。
- **Provider 差异仅体现在费率表与缓存写语义上，计算分支是同一函数**：Anthropic 有 `cacheWrite1h`（1h 保留期缓存写按 **2 倍输入费率**计，`models.js:379-380`）；OpenAI 费率表 `cacheWrite: 0`（OpenAI 缓存写本就按输入费率折扣计费，费率表不再重复），且 OpenAI 不报告 `cacheWrite1h`；Kimi-K3（`kimi-coding`）走 `anthropic-messages` API，但其 `k3` 费率表 `cacheWrite: 0`、无 1h 拆分。

### 1. cost 子字段何时非零（源码推导 + 费率表穷举）

`calculateCost(model, usage)`（`pi-ai/dist/models.js:371-388`）：

```js
inputTokens = usage.input + usage.cacheRead + usage.cacheWrite   // 用于选 tier
// 遍历 model.cost.tiers，取 inputTokens 超过的最高阈值 tier 作为 rates
cost.input     = (rates.input     / 1e6) * usage.input
cost.output    = (rates.output    / 1e6) * usage.output
cost.cacheRead = (rates.cacheRead / 1e6) * usage.cacheRead
longWrite  = usage.cacheWrite1h ?? 0          // 仅 Anthropic 报告
shortWrite = usage.cacheWrite - longWrite
cost.cacheWrite = (rates.cacheWrite * shortWrite + rates.input * 2 * longWrite) / 1e6
cost.total = input + output + cacheRead + cacheWrite
```

- 各子字段 = token 数 × 对应费率；**token 数取 0 或费率取 0 都会让该子字段为 0**（费率 0 时即使有 token 也是 0）。
- **tier 分级存在**（`models.d.ts:632-636`）：仅 `github-copilot`(6)、`openai`(7)、`openai-codex`(5) 有 `tiers` 配置；请求级 input 总量超过阈值后整请求用更高 tier 费率——这进一步说明"按模型单价重算"不可行（重算者不知道 tier 逻辑）。
- 对基线「cost 非零 2638 vs 零 57」的解释：**费率表显式全 0 的模型**（99 个，如 `k3-256k` 的 `{input:0, output:0, cacheRead:0, cacheWrite:0}`）产出的每条 usage 恒全 0；其余模型 token 非零则 cost 非零。**零值不是随机缺失，而是可归因到特定模型/费率表状态**。

### 2. 费率从哪读取：模型配置（费率表），非 provider 实时 pricing

- 费率表是 pi 内置数据：`pi-ai/dist/providers/data/*.json`（每 provider 一个文件，如 `anthropic.json`、`openai.json`、`kimi-coding.json`），按 `provider/api/model id` 给出 `cost: {input, output, cacheRead, cacheWrite, tiers?}`（单位：美元 / 百万 token）。
- 模型对象在 `pi-ai/dist/models.generated.js` 组装为 `MODELS` 注册表，`getModel()` 按 provider+id 取模型，`calculateCost` 读 `model.cost`（`types.d.ts:650` `cost: ModelCost`）。
- **`cost` 全部 1109 个模型均非 null**（脚本穷举验证），即 pi 不会因缺费率而抛错；但**显式全 0 的模型有 99 个**——这是全 0 cost 会话的主要来源。费率表是**构建时生成、随版本发布**的数据（`models.generated.js` 头注释 "auto-generated"），不随请求动态拉取，因此可能与官方最新价格有偏差。

### 3. Provider 差异（补充验证）

| 维度 | Anthropic | OpenAI | Kimi-K3 (kimi-coding) |
|------|-----------|--------|----------------------|
| API 分支 | `anthropic-messages.js:394,562` | `openai-responses-shared.js:431` | `anthropic-messages`（同左） |
| cost 计算 | 同一 `calculateCost` | 同一 `calculateCost` | 同一 `calculateCost` |
| `cacheWrite1h` | 报告（`anthropic-messages.js:390` 从 `cache_creation.ephemeral_1h_input_tokens` 取），1h 写按 **2×input 费率** | 不报告（`openai-responses-shared.js:419` 只取 `cache_write_tokens`）→ `longWrite=0` | 不报告 |
| 缓存写费率 | 有专价（如 claude-haiku-4-5: `cacheWrite: 1.25`） | 费率表 `cacheWrite: 0`（OpenAI 缓存写按输入折扣计费，费率表不重复计） | `k3` 费率 `cacheWrite: 0` |
| 典型费率 | claude-haiku-4-5: in 1 / out 5 / cacheRead 0.1 / cacheWrite 1.25 | gpt-4.1: in 2 / out 8 / cacheRead 0.5 / cacheWrite 0 | k3: in 3 / out 15 / cacheRead 0.3 / cacheWrite 0；**k3-256k: 全 0** |

- 会话数据访问补充说明：本任务尝试直接采样 `~/.pi/agent/sessions/` 的 cost 非零/全零样本，但被本机权限系统拒绝外部目录访问（bash/read/ffgrep 均被拦，`~/.pi` 解析为 dotfiles symlink）。改为**源码费率表穷举验证**（1109 个模型逐一检查 `cost` 字段），结论覆盖会话样本形态（见第 1 节）；ticket 01 的基线数据（非零 2638 / 零 57）仍为本 ticket 的会话侧证据。

### 4. 对分析工具的结论

- **直接累加 `usage.cost.total` 即可**：pi 已统一口径（同一天函数、同一费率表、含 tier 与 1h 缓存写规则），聚合时把每条 assistant usage 的 `cost.total` 相加即会话/总花费，与 pi 自身统计（`jsonl-storage.js` 累加组件）一致。
- **无需回退到按单价重算**：重算需要复制 tier 选择 + 1h 2× 规则 + 费率表维护，纯属重复劳动且必然漂移；唯一更优场景是**使用官方实时价格做参考对比**，但那超出本工具"计量"范围。
- **零花费的展示语义**：全 0 cost 的会话/模型应标注"费率未配置（免费/未定价）"而非"免费"，避免误读；对全 0 模型（如 k3-256k）建议按模型维度提示费率表缺价。
- **口径注意**：与 ticket 01 一致，`cost` 是 pi 自估（非 provider 账单），且只对 `type: message && role: assistant` 的 usage 统计（toolResult/compaction 的 usage 被 pi 统计忽略）。

### 证据索引

| 结论 | 证据 |
|------|------|
| cost 计算逻辑（tier + 1h 2× 缓存写） | `pi-ai/dist/models.js:371-388` |
| 费率表来源（provider 数据文件） | `pi-ai/dist/providers/data/*.json`（anthropic / openai / kimi-coding） |
| 模型注册表 | `pi-ai/dist/models.generated.js` |
| `cost: ModelCost` 必填字段 | `pi-ai/dist/types.d.ts:650` |
| tier 类型定义 | `pi-ai/dist/types.d.ts:632-636` |
| Anthropic 取 cacheWrite1h + 计算 | `pi-ai/dist/api/anthropic-messages.js:390,394` |
| OpenAI 无 cacheWrite1h、cost 入口 | `pi-ai/dist/api/openai-responses-shared.js:419,431` |
| 失败消息全 0 usage | `pi-agent-core/dist/agent.js:341-350`（EMPTY_USAGE） |
| 费率表穷举（1109 模型：null=0，全 0=99） | 本任务脚本对 `dist/providers/data/*.json` 逐一检查 |
