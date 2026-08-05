# 03 — 模型 / cwd 维度拆分与筛选

**What to build:** 给统计增加两个分析维度的拆分与筛选——**模型**与**工作目录（cwd）**。支持 `--model` / `--cwd` 过滤（只统计指定项）与分组（按维度汇总），二者可交叉。三个窗口（总/会话/单请求）都可按维度拆分。

端到端行为：
- 模型维度：按每条 assistant 消息的 `model` 字段归属其 usage（请求级权威归属，与 usage 同对象）；同一会话混合多模型时各自正确归属
- cwd 维度：以会话首行 header 的 `cwd` 字段为权威归属键，聚合前规范化（绝对路径、去尾斜杠、解析符号链接）；目录名（有损编码）仅作展示辅助，不参与归属
- `--model <id>` 只统计指定模型；`--cwd <path>` 只统计指定项目；无 flag 时默认全部
- 交叉分组：按 model × cwd 汇总
- 全 0 花费的模型标注「费率未配置（免费/未定价）」

**Blocked by:** 01 — 最小闭环：总消耗量统计

**Status:** resolved

## Acceptance criteria

- [x] 模型维度拆分正确：同会话混合多模型时按请求归属到各自模型，与真实数据（如 8 个混合模型会话）核对一致
- [x] cwd 维度拆分正确：按 header cwd 归属，不受目录名编码歧义影响
- [x] `--model` / `--cwd` 过滤生效，可交叉分组
- [x] fixture 覆盖：同会话多模型归属、多 cwd 目录 + 有损目录名按 header 归属

## 实施总结
- 提交：`0c96309` — feat: 模型/cwd 维度拆分与筛选
- 实现的 seams：S1 --model 过滤（三窗口）/ S2 --cwd 过滤（规范化比较）/ S3 --by model 分组（请求级归属）/ S4 --by cwd 分组（规范化合并）/ S5 交叉分组 + 过滤交集 / S6 model_change 行不干扰归属
- 验收标准：4 条全部 `- [x]`（见上）
- 测试结果：28/28 全绿（`npm test`）
- typecheck：通过（`npm run typecheck`，tsc --noEmit strict）
- 真实数据核对：CLI --by model 与独立 python 实现对同一快照 9 个模型分组逐指标完全一致；混合模型会话数 = 8（spec ticket 09 基线吻合）；--by cwd 分组按 header cwd 归属正常
- Code Review 修复：normalizeCwd 相对路径也 resolve（spec 规范化要求）/ groupRowsFromFiles 文件级 cwd 规范化缓存（避免逐消息 realpath）/ --model 过滤后隐藏完全无匹配会话（避免全 0 行）/ model_change 覆盖测试 / GroupBy 类型上移 aggregate.ts / 指标列定义去重（render.ts METRIC_COLUMNS）
- 遗留 / 后续建议：--by 分组仅作用于 totals 窗口（阶段②与用户确认的范围，spec 理想是三个窗口都可按维度拆分，如需要可在 issue 04/05 扩展）；sessions 窗口多模型会话仍标 mixed 不拆行
