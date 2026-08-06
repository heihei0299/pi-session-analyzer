# Token Analyzer — 功能规格（Spec）

**状态**: implemented（本 spec 由 wayfinder effort `token-analyzer` 综合全部已决 ticket 产出，供后续 session 按此实现工具；实现已完成，见文末「实施状态」）

**数据域**: pi 会话数据（`~/.pi/agent/sessions/` 下 JSONL 会话文件）；只分析 pi，不涉其他工具。

---

## Problem Statement

pi 是长期使用的编码 agent，其所有会话以 JSONL 形式存储在 `~/.pi/agent/sessions/`（当前 212 个合法会话文件）。用户无法直观得知：各模型、各项目（cwd）实际消耗了多少 token 与花费；缓存命中率如何；实时运行中的 pi 进程正在消耗多少。这些数据散落在 200+ 个 JSONL 文件中，字段语义（`input`/`output`/`cacheRead`/`cacheWrite`/`reasoning`/`cost`）与统计口径（哪些行计入）均有微妙之处，人工无法统计。需要一个工具把这些原始数据转化为可读、可筛选、可实时监控的指标。

## Solution

一个 **CLI 工具**（可重复运行），读取 `~/.pi/agent/sessions/` 下全部合法会话 JSONL，按 **模型** 与 **工作目录（cwd）** 两个维度拆分，统计三个窗口——**总消耗量 / 会话级消耗量 / 单次请求消耗量**——的指标：输入、输出、缓存（cacheRead+cacheWrite）、推理、缓存率、总 token、花费。支持按日/周/月汇总、时间范围与 cwd/模型筛选、结构化输出（JSON/CSV），并支持 **实时监控** 运行中的 pi 进程（tail -f 为主 + 轮询兜底）。

工具只做**计量**，不做优化建议；本 effort 只产出 spec，不写实现代码。

### 数据来源与合法性判定

- 数据源：`~/.pi/agent/sessions/` 下所有 `*.jsonl` 文件（递归遍历目录，当前 216 个文件、33 个目录）。
- **合法会话判定**：仅首行 `type == "session"` 的 JSONL 是会话（当前 208/216）；`type: message` / `type: custom` 文件是单条记录导出/残留，**跳过**。
- **文件名不作判据**：存在 68 个非标准命名文件（用户扩展/备份导出），尾 UUID 与 header id 一致且首行是完整 session header，是合法会话，必须纳入。
- **项目归属**：以会话首行 header 的 `cwd` 字段为权威（完整绝对路径、无歧义）；目录名是 cwd 的有损编码（`--` 包裹、`/`→`-`），**不可反解**，仅作展示与辅助分组。聚合时按规范化 cwd（`resolve` 去尾斜杠/符号链接）。
- **fork 去重**（ticket 25 已实现）：`parentSession`（父会话文件绝对路径）标记 fork 会话——fork 复制历史（message.timestamp < header.timestamp）的 usage 已在原会话统计，analyzeFile 剔除，fork 后新增消息保留（CLI/webui 一致）。

### 统计口径（决策锚）

| 决策点 | 口径 | 来源 |
|---|---|---|
| 计入哪些行的 usage | **只认 `type=message && role=assistant` 且携带 usage 的消息**；toolResult / compaction / branch_summary 的 usage 一律不计入（三个窗口一致） | ticket 11 |
| 单次请求窗口 | 一条带 usage 的 assistant 消息 = 一次请求；请求数 = 带 usage 的 assistant 消息数；全 0 usage 的失败/中止消息**计入请求数**（token 为 0） | ticket 06 |
| 会话/总窗口 | 单请求窗口之和（同源：assistant 消息 usage）；无额外计入来源 | ticket 11 |
| 总 token | **按组件和计算**：`input + output + cacheRead + cacheWrite`（勿信 `totalTokens` 字段本身，OpenAI 侧可能不精确等于组件和） | ticket 01 |
| 推理 vs 输出 | **output 为总量（含推理）**，reasoning 单独成列；两列各自累加字段值，不做 `output−reasoning` 减算；reasoning 为 `undefined` 按 0 处理 | ticket 05 |
| 缓存率 | **口径 A**：`cacheRead / (input + cacheRead + cacheWrite)`；分母为 0 记 0 | ticket 04 |
| 聚合规则 | 各窗口内指标**先求和分子分母再除**（缓存率尤其如此，避免对零值请求做除法） | ticket 04/06 |
| 花费 | **直接累加 `usage.cost.total`**；不做按单价重算（tier 分级、1h 缓存写 2× 费率等规则重算必然漂移） | ticket 02 |
| 模型归属 | **请求级**：按每条 assistant message 的 `model` 字段归属其 usage（与 usage 同对象）；`responseModel` 可用于 provider 别名归一化；compaction/branch_summary 不计入，无归属问题 | ticket 09 |
| 零花费语义 | 全 0 cost 标注「费率未配置（免费/未定价）」而非「免费」 | ticket 02 |

### 指标与维度

- **指标**（每窗口每分组）：输入 `input`、输出 `output`、缓存 `cacheRead + cacheWrite`（可拆两列展示）、推理 `reasoning`、缓存率（见上）、总 token（组件和）、花费 `cost.total`、请求数（assistant 消息数）。
- **维度**：按模型（`AssistantMessage.model`）、按 cwd（header `cwd` 规范化）拆分；二者可交叉分组与筛选。
- **窗口**：总消耗量（全部会话汇总）、会话级消耗量（按会话分组）、单次请求消耗量（逐 assistant 消息，最细粒度）。

### 运行形态

- **CLI 工具**，可重复运行，子命令/flag 驱动；非单文件脚本。
- **输出格式**：默认终端表格（人类可读）；`--format json` / `--format csv` 输出结构化格式供脚本消费。
- **时间维度**：按日/周/月汇总（按会话/消息时间戳归属）；`--since` / `--until` 时间范围筛选。
- **筛选**：`--cwd`（项目）、`--model`（模型）过滤与分组；默认全部会话。
- **实时监控**：`tail -f` 为主（append-only、消息级即时写盘、低延迟零侵入）+ 定时轮询兜底（断线/文件替换重同步）；增量边界 = 一行完整 `role=assistant` message entry（usage 在请求完成后一次性落盘，非流式分片）。

## User Stories

1. 作为用户，我想运行工具查看**全部会话的总消耗量**（输入/输出/缓存/推理/总 token/花费/请求数），以便了解 pi 的整体使用规模。
2. 作为用户，我想按**模型**查看消耗拆分，以便知道哪个模型花钱最多、用得最勤。
3. 作为用户，我想按**项目（cwd）**查看消耗拆分，以便按工作目录归因成本。
4. 作为用户，我想查看**会话级**消耗明细（每个会话一行，含时间、模型、cwd、各指标），以便定位单个会话的花费。
5. 作为用户，我想查看**单次请求**粒度的消耗（每条 assistant 消息一行），以便排查异常高消耗的具体请求。
6. 作为用户，我想查看**缓存率**（总/会话/单请求三窗口各自），以便评估缓存命中效率与优化空间。
7. 作为用户，我想查看**推理 token** 单独成列的展示，并理解它包含在输出总量内（不重复计数）。
8. 作为用户，我想**按日/周/月汇总**消耗，以便观察使用趋势。
9. 作为用户，我想用 `--since` / `--until` 筛选**时间范围**，以便只统计某段时间的消耗。
10. 作为用户，我想用 `--cwd` / `--model` **筛选**统计范围，以便聚焦特定项目或模型。
11. 作为用户，我想输出 **JSON / CSV** 格式的结果，以便接入自己的脚本或报表。
12. 作为用户，我想**实时监控**正在运行的 pi 进程的 token 消耗，以便在长任务进行中观察成本。
13. 作为用户，我想监控在断线/文件替换后能**自动重同步**（轮询兜底），以便长时间挂机不丢数据。
14. 作为用户，我想看到**零花费**的会话/模型被标注为「费率未配置/免费」而非误导性的「免费」，以便正确理解数据。
15. 作为用户，我希望**非标准命名的会话文件**（用户导出/扩展产物）也被正确纳入统计，以便不漏计真实使用。
16. 作为用户，我希望**单条记录残留文件**（type: message/custom 的文件）被自动跳过，以便统计不被污染。
17. 作为用户，我希望**失败/中止的请求**（全 0 usage）计入请求数，以便请求计数真实反映发生过多少次调用。
18. 作为用户，我希望同一会话内**混合多模型**的消耗被按请求正确归属到各自模型，以便模型维度统计准确。
19. 作为用户，我希望缓存率在**分母为 0** 的请求上不报错、记为 0，以便统计流程稳定。
20. 作为用户，我希望总 token 按**组件和**计算而非信任字段值，以便与 pi 自身统计口径一致、数字可交叉验证。

## Implementation Decisions

- **单一数据读取层**：遍历 `~/.pi/agent/sessions/`（含子目录）→ 每文件读首行判 `type==session` → 解析 header（取 `cwd`、`timestamp`、`id`）→ 流式逐行解析后续 entry，仅提取 `type=message && role=assistant` 且携带 usage 的消息；其余 entry（toolResult/compaction/branch_summary/user/custom/model_change 等）在统计层忽略。目录名仅作展示标签。
- **聚合模型**：三层窗口（总 / 会话 / 单请求）共用同一套指标计算函数；会话级 = 按 `sessionId` 分组，单请求级 = 逐消息。每层均可按 model / cwd（或二者）交叉分组。指标分两类：**可累加标量**（input/output/cacheRead/cacheWrite/reasoning/cost.total/请求数）与**比率指标**（缓存率）——比率一律「先求和分子分母再除」，不在中间层做除法。
- **模型归属**：直接读 `AssistantMessage.model`（请求级权威）。`responseModel` 留作可选的 provider 别名归一化（第一版可不做）。不依赖 `model_change` 事件做归属（仅可作事件轴展示）。
- **cwd 规范化**：header `cwd` 为权威归属键，聚合前规范化（绝对路径、去尾斜杠、解析符号链接），目录名不参与归属计算。
- **时间归属**：会话级/日周月汇总以会话 header `timestamp` 归属；单请求级可按消息时间戳（如实现方便）或归属会话时间戳。spec 建议：日/周/月汇总与 `--since`/`--until` 以**会话时间戳**为基准（第一版），保持口径简单。
- **实时监控**：增量读取器维护每个会话文件的读取位置（offset），`tail -f` 式跟随新追加行 + 定时轮询（mtime/行数/size/inode 比对）检测文件替换与断线重同步；增量边界 = 完整一行 JSON 记录；仅 `role=assistant` 且含 usage 的行进入统计；输出采用与静态模式相同的数据结构与格式（终端/JSON/CSV），可 `--watch` 模式持续刷新。
- **花费**：直接累加每条计入消息的 `usage.cost.total`；零值归因（费率表缺价/全 0 token）不做重算，仅在展示层标注「费率未配置（免费/未定价）」。
- **输出**：终端表格默认列：窗口/分组键（模型或 cwd）/请求数/输入/输出(含推理)/推理/缓存读/缓存写/缓存率/总 token/花费。JSON/CSV 字段与表格一一对应，供脚本消费。

## Testing Decisions

- **测试目标**：只测**外部行为**——「给定一组会话 JSONL 输入 → 输出的统计数字与口径是否符合 spec」，不测内部实现细节。
- **唯一测试 seam**：以**会话 JSONL fixture 为输入**、以 **CLI 输出（表格/JSON/CSV）为断言点**。构造最小合成 fixture 覆盖口径边界，比真实数据更可控。
- **fixture 覆盖清单**（每个口径决策一个用例）：
  1. 合法会话（首行 session）与残留文件（首行 message/custom）混放 → 只统计合法会话；
  2. 非标准文件名会话 → 照常纳入；
  3. toolResult / compaction / branch_summary 带 usage 的行 → 全部忽略（总/会话/单请求三窗口均不含）；
  4. 全 0 usage 的失败 assistant 消息 → 计入请求数、token 为 0；
  5. totalTokens 字段与组件和不等（模拟 OpenAI 侧）→ 结果按组件和；
  6. reasoning = 0 / undefined / < output → output 列含推理、reasoning 列单独展示，不做减算；
  7. 缓存率分母为 0 → 记 0；聚合缓存率 = 分子和/分母和；
  8. cost 全 0 → 展示「费率未配置」标注；
  9. 同会话多模型（含 model_change）→ 请求按 model 正确归属；
  10. 多 cwd 目录、目录名有损 → 按 header cwd 归属；
  11. 时间筛选（--since/--until）与日/周/月汇总 → 按会话时间戳归属；
  12. --format json / csv 字段与表格一致；
  13. 实时监控：追加新行 → 增量输出；文件替换/截断 → 轮询重同步不丢不重。
- **验证途径**：各口径可对照 ticket 01/04/06 中已给出的真实数据基线（如 assistant 9022 行 / toolResult 10069 行全无 usage、缓存率公式样例）做交叉核对。

## Out of Scope

- **其他工具对比**（Cursor / Claude Code 等）——只分析 pi。
- **token 优化建议**——本工具只做计量，不做优化。
- **按单价重算花费**——直接采用 pi 的 `cost.total`；不维护独立费率表。
- **compaction / branch_summary 的 usage 计入**——口径 A 明确忽略（与 pi 自身统计一致）。
- **pi 进程内插件/事件方案**——实时监控走文件层（tail/轮询），不注入 pi 进程（侵入、版本敏感）。
- **parentSession fork 去重**——ticket 25 已实现（analyzeFile 按 forkTs 剔除复制历史），见上文「fork 去重」条目。
- **responseModel 别名归一化**——可选增强，第一版不实现。
- **实现代码**——本 effort 只产出 spec，不写实现。

## Further Notes

- **与 pi 自身统计的差异**：pi 文档注释称 compaction/branch_summary 的 usage "included in session token and cost totals"（`docs/session-format.md`），但 pi 自身会话统计只认 `type=message && role=assistant`（`jsonl-storage.js:233-240`）。本工具按口径 A（assistant 仅认），与 pi 会话统计一致；若与 pi 文档注释的「全量」说法对照出现数字差异，属预期。当前真实数据中 toolResult（10,069 行）、compaction（8 行）、branch_summary（0 行）均不携带 usage，A/B 口径当前数字一致——未来若这些类型开始携带 usage，本工具仍按 A 忽略，spec 已固定此规则，无需运行时判断。
- **totalTokens 字段**：聚合一律按 `input + output + cacheRead + cacheWrite` 组件和计算，与 pi 自身统计（`jsonl-storage.js:250`）口径一致，可交叉验证。
- **数据量参考**（2026-08 采样）：216 个文件、212 合法会话；带 usage 的 assistant 消息 9,022 条；总 token ≈ 9.3 亿（cacheRead 占绝对大头 9.07 亿）；cost 非零 2,638 vs 全 0 57（费率表显式全 0 模型 99/1109）。工具应能秒级处理此量级（流式逐行解析即可，无需加载全量到内存）。
- **术语**：见仓库 `CONTEXT.md` 与 wayfinder effort 各 ticket（本 spec 引用的「ticket 0N」均指 `.scratch/token-analyzer/issues/` 下同名文件）。

---

## 实施状态（Implementation Status）

**已实现**（2026-08，`.scratch/token-analyzer-impl/` effort，commit 77a266a→5c27253）。

**技术栈**：TypeScript + Node 24（原生 type-stripping 运行，零运行时依赖）；`src/` 实现 + `test/` 测试（node:test，46 用例全绿）。

**CLI 形态**（与 spec 运行形态一致，全部 20 条 user stories 覆盖）：

```
token-analyzer [totals|sessions|requests] --dir <path> \
  [--format table|json|csv] [--model <id>] [--cwd <path>] \
  [--by model|cwd|model,cwd] [--period day|week|month] \
  [--since <时间>] [--until <时间>] [--watch] [--interval <ms>]
```

**实现期确认的范围决策**（阶段②与用户逐项确认，均为 spec 未定细节）：

- `--by` 分组与 `--period` 汇总仅作用于 **totals 窗口**（sessions/requests 窗口维持逐会话/逐请求，受 `--model`/`--cwd`/`--since`/`--until` 过滤影响）；二者不可同时使用（显式报错）
- `--since`/`--until` 为**闭区间**（含端点）：日期参数 since 取当天 00:00、until 取当天 23:59:59.999
- 周汇总为 **ISO 周**（周一起始），周期键展示为起始日期
- `--watch` 实时监控仅支持 totals 窗口 + table 格式（JSON/CSV 长驻刷新未实现，记遗留）
- 实时监控基于增量读取器（offset 续读 + inode/size/mtime 轮询重同步 + 旧贡献扣减防重复），与静态统计共享同一数据读取与聚合层

**验证**：全部 5 个实现 issue（01–05）验收标准逐条通过；真实数据与独立 python 实现对同一快照逐指标核对一致（模型分组、按月汇总、watch 初始扫描 vs 静态 totals）。
