# 06 — 单次请求窗口定义

Type: grilling
Status: resolved
Blocked by: 01

## Question

「单次请求 token 消耗量」窗口的边界是什么？

候选口径：
- A: 一条携带 usage 的 message 即一次请求（最细粒度，语义直接）
- B: 按 parentId 链聚合（一次用户提问 → 后续工具调用/回复链算一次请求）
- C: 其他（请说明）

## 背景

需求明确三个统计窗口：总消耗量、会话级消耗量、单次请求消耗量。「单次请求」的边界直接影响聚合逻辑与输出（如请求数统计）。此决策被「usage 字段语义研究」（ticket 01）关于 message 结构的结论支撑。


## Answer

**决策：口径 A** —— 一条携带 usage 的 message 即一次请求（最细粒度）

- 窗口成员：**`type=message && role=assistant` 且携带 usage 的消息**（assistant 消息的 usage 必填，见 ticket 01）——请求数 = 带 usage 的 assistant 消息数
- **toolResult 消息的 usage 不计入**单次请求窗口：pi 自身统计口径只认 assistant（`jsonl-storage.js:233-240`），且 toolResult 无 model 字段，计入会破坏按模型拆分（ticket 09）；其消耗归属由「统计口径」ticket 另行决策
- **compaction / branch_summary 永不计入**请求数：它们不是 message/request，无请求语义
- 边界推论（01/04 的推论，非独立决策）：全 0 usage 的失败/中止消息是真实发生的请求，**计入请求数**（token 消耗为 0；缓存率分母为 0 记 0，见 ticket 04）
- 聚合规则（与 ticket 04 一致）：单请求窗口内各指标**先求和分子分母再除**；请求数即消息数，与 token 值无关
- 依据：ticket 01（usage 粒度 = 单次 API 调用）、ticket 09（模型归属为请求级）、pi 会话统计口径（jsonl-storage.js 只认 assistant）

> 遗留：总/会话窗口是否计入 toolResult / compaction / branch_summary 的 usage 属「统计口径」决策（map Not yet specified 中的 fog，已毕业为 ticket 11）
