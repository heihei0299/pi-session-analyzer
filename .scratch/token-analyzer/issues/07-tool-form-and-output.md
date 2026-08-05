# 07 — 工具形态与输出格式

Type: grilling
Status: resolved

## Question

分析工具的运行形态、输出格式与扩展能力？

需要决策：
1. **形态**: CLI 工具（可重复运行） vs 单文件脚本 vs 不限（spec 中由实现者定）
2. **输出格式**: 终端表格 / JSON / CSV / 多格式（按 flag 切换）
3. **时间维度**: 是否支持按日/周汇总？还是仅总量
4. **筛选能力**: 是否支持按项目/时间范围筛选（用户需求当前为"全部会话"）
5. **实时监控形态**: 轮询（定期扫描增量）vs tail（append-only 流）vs pi 事件/插件 API（需 10 研究支撑）
6. **模型维度**: 按模型拆分统计与展示（需 09 研究支撑）
7. **工作目录维度**: 按 cwd 拆分/筛选（03 已支撑：cwd 为权威归属）

## 背景

Destination 是「可交接的 spec」，工具形态决策是 spec 的核心内容。此决策同时受「会话边界识别」（ticket 03）与「单次请求窗口」（ticket 06）结论影响，宜在其后决策。

## Resolution

经 `/grilling` 会话与用户确认后，将结论写入本文件 `## Answer` 段落，`Status: resolved`，并在 map `Decisions so far` 追加一行。

## Answer

经 `/grilling` 会话逐项确认（2026-08）：

1. **形态**: **CLI 工具**（可重复运行，支持子命令/flag），非单文件脚本。
2. **输出格式**: **多格式按 flag 切换** —— 默认终端表格（人类可读），`--format json` / `--format csv` 输出结构化格式供脚本消费。
3. **时间维度**: **支持按日/周/月汇总**（按会话时间戳归属）+ **时间范围筛选**（`--since` / `--until`）。
4. **筛选能力**: **支持按 cwd（项目）/ 模型筛选与分组**（默认全部会话；`--cwd` / `--model` 过滤）。
5. **实时监控形态**: **tail -f 为主 + 轮询兜底**（采纳 ticket 10 结论：append-only 增量、`tail -f` 低延迟零侵入，定时 mtime/行数轮询做断线/文件替换重同步兜底；增量边界 = 一行完整 role=assistant message entry）。
6. **模型维度**: **按模型拆分统计与展示**（采纳 ticket 09：按每条 assistant message 的 `model` 请求级归属其 usage）。
7. **工作目录维度**: **按 cwd 拆分/筛选**（采纳 ticket 03：首行 header `cwd` 为权威归属，目录名仅辅助分组）。

