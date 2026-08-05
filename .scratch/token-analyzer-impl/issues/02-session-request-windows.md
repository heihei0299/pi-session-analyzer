# 02 — 会话级与单请求级窗口 + JSON/CSV 输出

**What to build:** 在总消耗量（ticket 01）基础上，增加另外两个统计窗口——**会话级消耗量**（按会话分组，每会话一行）与**单次请求消耗量**（逐 assistant 消息，最细粒度）——并支持 `--format json` / `--format csv` 结构化输出。三个窗口共用同一套指标计算（ticket 01 的聚合模型复用），口径一致。

端到端行为：
- 会话级窗口：按 sessionId 分组，每会话输出一行（含会话时间戳、模型、cwd、各指标、缓存率）
- 单请求级窗口：逐条 assistant 消息输出（含 model、时间、各指标）
- 三窗口指标一致：请求数、输入、输出、缓存读、缓存写、推理、总 token（组件和）、花费、缓存率
- `--format json` / `--format csv` 输出与终端表格字段一一对应，供脚本消费；默认仍为终端表格
- 全 0 花费的会话/模型标注「费率未配置（免费/未定价）」而非「免费」

**Blocked by:** 01 — 最小闭环：总消耗量统计

**Status:** resolved

## Acceptance criteria

- [x] 会话级窗口正确分组：每会话一行，指标为会话内 assistant usage 之和，与总窗口的数字对得上（总 = 各会话之和）
- [x] 单请求级窗口逐消息输出，请求数 = 带 usage 的 assistant 消息数（含全 0 失败消息）
- [x] `--format json` / `--format csv` 与终端表格字段一致
- [x] fixture 覆盖：多会话分组求和正确、零花费标注生效

## 实施总结
- 提交：`6667d8d` — feat: 会话级与单请求级窗口 + JSON/CSV 输出
- 实现的 seams：S1 会话级分组求和 / S2 单请求级逐消息（含全 0 失败消息）/ S3 会话行元数据（timestamp/cwd/model，混合模型标 mixed）/ S4 JSON 输出（totals/sessions/requests 原始数值）/ S5 CSV 输出（字段与 JSON 一致）/ S6 零花费标注 / S7 向后兼容（默认 totals）；+ review-fixes 4 项（空会话 model 为 "-"、CSV 转义、非法 --format 报错、空数据 CSV 表头）
- 验收标准：4 条全部 `- [x]`（见上）
- 测试结果：22/22 全绿（`npm test`）
- typecheck：通过（`npm run typecheck`，tsc --noEmit strict）
- 真实数据核对：三窗口求和一致（total requests = Σsessions = Σrequests = 9265）；CSV totals 含 window 列与 JSON 对齐；零花费标注「费率未配置（免费/未定价）」
- 遗留 / 后续建议：同 sessionId 跨文件合并未实现（真实数据每文件唯一 id，低风险，如遇续写会话需在 issue 03 处理）；render/serialize 字段清单可抽单一来源（加列需改 3 处，当前可接受）
