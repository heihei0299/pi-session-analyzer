# 01 — 最小闭环：总消耗量统计

**What to build:** 一个 CLI 命令，遍历 `~/.pi/agent/sessions/` 下全部会话 JSONL，识别合法会话，按统计口径 A 提取消耗数据，输出**总消耗量窗口**的终端表格。这是工具的骨架：数据读取层 + 聚合模型的最小闭环，后续所有切片（02/03/04/05）都构建在它之上。

端到端行为：
- 遍历会话目录（含子目录），读每个文件首行：`type == "session"` 才是合法会话；`type: message` / `type: custom` 的残留文件跳过；非标准命名的合法会话照常纳入
- 逐行解析合法会话，仅提取 `type=message && role=assistant` 且携带 usage 的消息；toolResult / compaction / branch_summary / user 等一律忽略（口径 A）
- 全 0 usage 的失败/中止 assistant 消息**计入请求数**（token 为 0）
- 聚合总窗口指标：请求数、输入、输出、缓存读、缓存写、推理、总 token（按 `input+output+cacheRead+cacheWrite` 组件和计算，勿信 totalTokens 字段）、花费（直接累加 `cost.total`）
- 缓存率 = `cacheRead/(input+cacheRead+cacheWrite)`，分母为 0 记 0；聚合先求和分子分母再除
- 终端表格输出（人类可读）

**Blocked by:** None — can start immediately

**Status:** resolved

## Acceptance criteria

- [x] 对真实数据运行：合法会话被纳入（当前 208+ 个），残留 message/custom 文件被跳过
- [x] 表格输出包含全部 8 个指标列 + 缓存率，总 token 与 pi 自身统计口径（组件和）一致
- [x] fixture 测试框架建立：最小合成 JSONL fixture → CLI 输出，覆盖：残留文件跳过、非标准文件名纳入、toolResult/compaction 带 usage 被忽略、全 0 失败消息计入请求数、totalTokens 字段与组件和不一致时按组件和、缓存率分母为 0 记 0
- [x] 输出数字可对照 spec Further Notes 的 2026-08 基线交叉核对（带 usage 的 assistant 消息 ~9,022 条）

## 实施总结
- 提交：`77a266a` — feat: token-analyzer 最小闭环——总消耗量统计 CLI
- 实现的 seams：S1 最小闭环 / S2 残留文件跳过 / S3 非标准文件名纳入 / S4 口径 A 过滤 / S5 全 0 失败消息计入请求数 / S6 组件和计算 / S7 缓存率聚合（+ S4b 健壮性：usage null/缺失、坏 JSON 行防御）
- 验收标准：4 条全部 `- [x]`（见上）
- 测试结果：8/8 全绿（`npm test`）
- typecheck：通过（`npm run typecheck`，tsc --noEmit strict）
- 真实数据交叉核对：CLI 与独立 python 实现对同一数据快照 9 项指标完全一致（requests=9096、总 token=936,681,305、cost=6.243032、缓存率=97.73%）；2026-08 基线 9,022 条 → 当前 9,096+（数据实时增长，属预期）
- 遗留 / 后续建议：零花费标注「费率未配置（免费/未定价）」属 issue 02 范围，本 issue 未实现；reasoning 边界（=0/undefined/<output）在 S5/S1 顺带覆盖，未设独立用例，可在 issue 02 补
