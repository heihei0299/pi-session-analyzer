# Token Analyzer — Map

**Map id**: `token-analyzer` — see `docs/agents/issue-tracker.md` for tracker conventions.

## Destination

一份**可交接的 spec**（`.scratch/token-analyzer/spec.md`）：描述一个分析 pi 全部会话（`~/.pi/agent/sessions/` 下 212 个会话 JSONL）token 消耗的工具，并支持**实时监控 pi 进程**的 token 消耗。指标：输入 / 输出 / 缓存（cacheRead+cacheWrite）/ 推理 / 缓存率 / 总 token / 花费。三个统计窗口：**总消耗量、会话级消耗量、单次请求消耗量**；分析维度明确按**模型**与**工作目录（cwd）**拆分。只分析 pi，不涉其他工具。spec 供后续 session 实现——本 effort 不写实现代码（用户明确要求）。

## Notes

- **Domain**: pi 会话数据（仓库 pi-session-anylize；数据源在 `~/.pi/agent/sessions/`）
- **技能**: `research`（AFK 调研，ticket 类型 research）、`grilling` / `domain-modeling`（HITL 决策，ticket 类型 grilling）、`to-spec`（收尾综合）
- **用户偏好**: 中文沟通；明确"不要写任何代码"；高确定性、流程驱动；一次只做一件事
- **会话纪律**: 每 session 最多 resolve 一个 ticket（research 除外）
- **偏差记录**: 技能提及的 `research/<name>` 分支——本仓库无提交、无 remote，分支无意义，research 发现直接写入 ticket 文件的 `## Answer` 段落
- **数据勘察基线**（2026-08 采样前 60 文件）: `totalTokens = input + output + cacheRead + cacheWrite`；带 usage 的 message 行 2695/6554；cost 非零 2638 vs 零 57；`reasoning` 恒 < `output`（疑似子集）

## Decisions so far

<!-- 每解析一个 ticket，在此追加一行：名称 + 链接 + 一句话结论 -->

- [usage 字段语义研究](issues/01-usage-field-semantics.md) — usage 挂 assistant（必填）/ toolResult（可选）/ compaction（计入）；totalTokens 按 input+output+cacheRead+cacheWrite 组件和计算（勿信字段本身）；reasoning 是 output 子集；cost 为 pi 按模型费率表自估，全 0 = 费率缺失
- [会话边界识别研究](issues/03-session-boundary-detection.md) — 仅首行 `type: session` 为合法会话（208/212）；首行 `cwd` 为权威项目归属（目录名是有损编码，81/208 不可反解）；跳过 message/custom 文件
- [cost 字段与花费计算研究](issues/02-cost-and-pricing.md) — 直接累加 usage.cost.total 即可（同一天函数含 tier + 1h 2× 缓存写规则）；全 0 来自费率表显式价格 0（99/1109 模型，如 k3-256k/*-free）或全 0 token；无需按单价重算
- [请求模型归属研究](issues/09-request-model-attribution.md) — `AssistantMessage.model` 为请求级权威模型归属（与 usage 同对象）；compaction 归属需结合会话配置
- [缓存率定义](issues/04-cache-hit-ratio-definition.md) — 口径 A：缓存率 = cacheRead/(input+cacheRead+cacheWrite)；分母为 0 记 0；聚合先求和分子分母再除
- [实时监控途径研究](issues/10-realtime-monitoring-path.md) — 会话 JSONL append-only、消息级即时写盘；推荐增量轮询/tail 方案（详见 Answer）
- [工具形态与输出格式](issues/07-tool-form-and-output.md) — CLI 工具（可重复运行）；默认终端表格 + `--format json/csv`；按日/周/月汇总 + 时间范围筛选；按 cwd/模型筛选分组；实时监控 tail -f 为主 + 轮询兜底
- [推理与输出 token 关系](issues/05-reasoning-vs-output.md) — 口径 B：output 为总量（含推理），reasoning 单独成列；两列各自累加字段值，不做减算
- [统计口径（哪些 usage 计入总量）](issues/11-statistical-scope.md) — 口径 A：总/会话窗口只认 assistant 消息 usage，toolResult / compaction / branch_summary 一律不计入（与 06 同锚）；当前数据下 A/B 数字一致（toolResult 10069 行、compaction 8 行均无 usage）
- [综合 spec](issues/08-compose-spec.md) — 已交付 `.scratch/token-analyzer/spec.md`（ready-for-agent）：全部 11 个 ticket 决策综合为可交接 spec，含 20 条 user stories、单 seam 测试方案（JSONL fixture → CLI 输出）、13 个口径边界用例

## Not yet specified

<!-- 当前无未 ticket 化的 fog：全部 11 个 ticket 已 resolve（含综合 spec 08）；map 已到终点，route 清晰，spec 可交接实现 -->



## Out of scope

- **其他工具对比**（Cursor / Claude Code 等）——用户明确只分析 pi
- **token 优化建议**——本 effort 只做计量，不做优化
- **实现代码**——用户明确"不要写任何代码"；spec 是终点

