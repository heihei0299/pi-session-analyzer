# 05 — 推理与输出 token 关系

Status: resolved
Blocked by: 01

## Question

「推理」（reasoning）与「输出」（output）token 在展示与聚合时如何界定？

候选口径：
- A: 推理独立成列，且不计入输出（output 为纯文本输出）
- B: 推理是输出的子集（输出 = 推理 + 文本），展示时推理单独成列、输出为总量
- C: 推理并入输出，不单独展示

## 背景

勘察基线显示 `reasoning` 恒 < `output`（2643/2643 例），疑似子集；需求把「推理」列为独立指标，需锁定其与输出、总 token 的关系。此决策被「usage 字段语义研究」（ticket 01）的结论支撑。

## Answer

**决策：口径 B** —— 输出为总量（含推理），推理单独成列。

事实依据（本 session 数据验证，样本 ~1000 文件 / 8526 条带 reasoning 的 usage）：`reasoning < output` 8524 条、`==` 2 条、`>` 0 条、`reasoning==0` 1525 条——**reasoning 恒为 output 的子集，无例外**（output = 推理 + 文本）。

- **展示**：output 列 = pi 的 `usage.output` 字段（含推理），reasoning 列单独展示其子集部分。
- **聚合**：output、reasoning 两列各自直接累加字段值，不做 `output−reasoning` 重算，与 pi 自身统计口径完全一致。
- **spec 注明**："推理含于输出"，避免读者把 input/output/reasoning/cache 各列相加误解为总 token（实际 totalTokens = input + output + cacheRead + cacheWrite）。
