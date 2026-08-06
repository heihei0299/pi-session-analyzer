# Token Analyzer — token 总量统计与网关 pi-switch 对齐（Spec）

**状态**: ready-for-agent（2026-08-06，grill-to-spec 综合：grilling 五轮决策 + ADR-0002 用户确认 + 对账事实；实现按此 spec 进行）

## Problem Statement

用户拿 token-analyzer 的「总 token」与 pi-switch 网关对比对不上（全量 1.48G vs 网关 932.7M），无法信任统计。差异来自四层：口径定义（组件和 vs 网关总输入+输出）、窗口（session 含 8/1 前网关无数据）、时区（本地 CST vs 网关 UTC）、覆盖（直连/其他客户端请求结构性缺失）；另有 fork 复制历史重复计入（npm 版缺 ticket 25 去重）。

## Solution

1. **口径对齐**：`totalTokens` 显式定义为「总输入 + 输出」= `input + cacheRead + output`，`cacheRate` 分母同步不含 cacheWrite（ADR-0002）。cacheWrite 降为独立列。
2. **窗口对齐**：webui 新增「自 8/1（网关可比）」预设并**默认激活**（since=`2026-08-01T00:00:00Z`，UTC 精确 = 网关数据起点）；「全部」仍可切。
3. **时区语义文档化**：时间参数按本地时区（CST），网关 ts 为 UTC（保持现状，文档注明）。
4. **fork 去重上线**：npm 重新发布，使已实现的 ticket 25 去重进入发布版（本地 dist 已 build 修复）。

## User Stories

1. 作为用户，我想让「总 token」的定义与网关 total（总输入+输出）完全一致，以便两个工具在同一窗口下数字直接可比。
2. 作为用户，我想打开 webui 首屏即看到与网关对得上的总量（8/1 起累计差 0.6%），以便快速核对。
3. 作为用户，我想用「自 8/1」预设精确对齐网关数据起点（UTC 8/1 00:00），以便对账时窗口一致。
4. 作为用户，我想仍能切到「全部」查看含 8/1 前数据的完整历史，以便了解历史总消耗。
5. 作为用户，我想 cacheWrite 保持为独立指标列展示，以便在总量之外看到缓存写明细。
6. 作为用户，我想在文档/状态行中看到时区语义（时间参数本地时区、网关 UTC），以便理解「今天」边界为何与网关差 8 小时。
7. 作为用户，我想 npm 安装版与源码版统计一致（fork 去重生效），以便不再被 019fd362 之类 fork 复制历史虚高误导。
8. 作为用户，我想知道剩余差异来自覆盖结构（8/1 网关刚启用、直连/其他客户端请求），以便判断哪些差异是预期、哪些是异常。
9. 作为用户，我想 CLI 的时间筛选语义保持稳定（会话级），以便不破坏已有脚本与已发布版本行为。
10. 作为用户，我想缓存率分母与网关口径一致（cacheRead / (input + cacheRead)），以便缓存命中率数字与网关可比。

## Implementation Decisions

- 聚合收尾 `finalizeTotals`：`totalTokens = input + cacheRead + output`（不含 cacheWrite）；`cacheRate` 分母 = `input + cacheRead`（不含 cacheWrite，对齐网关 `cachedTokens/promptTokens`）。当前 cacheWrite 恒 0，数值无变化，纯定义锁定。
- cacheWrite 保持独立累加与展示（CLI 表格「缓存写」列、JSON/CSV 字段、webui 明细「缓存」列），仅总量与缓存率分子分母不含。
- webui 时间预设新增「自 8/1（网关可比）」：`since = "2026-08-01T00:00:00Z"`（parseTimestamp 支持带时区后缀原样解析），默认激活；原「全部」保留可切；预设按钮组顺序「今天/7天/30天/自 8/1/全部/自定义」。
- webui 状态行：默认「自 8/1」窗口时显示范围（`2026-08-01T00:00:00 ~ 现在`）并标注「网关可比」；切换其他窗口不标注。
- 文档（README + CONTEXT.md）：totalTokens 网关口径、网关可比窗口、时区语义、覆盖差异说明、对账参考表（8/1 起 940.5M vs 935.0M，0.6%；8/2/8/4 分毫不差）。

## Testing Decisions

- 好测试 = 只测外部行为：聚合结果数值（给定 usage 序列 → totalTokens/cacheRate 期望值），不测内部累加过程。
- **聚合层**：改造 `s6-component-sum` 为网关口径测试——cacheWrite 非零 fixture 断言 totalTokens 不含 cacheWrite、cacheRate 分母不含 cacheWrite；cacheWrite=0 fixture 断言与旧组件和数值相同（回归保护）。
- **回归**：`02-s4-json`（导出字段值）、`02-s6-unpriced`（展示）、`07-api`（端点 totalTokens）、`helpers`（fixture 构造）、`24-watch-fork-dedup`——cacheWrite=0 时值不变，验证不破坏。
- **webui**：`08-overview-ui` 类 UI 测试——「自 8/1」按钮默认 active、状态行标注、切「全部」恢复。
- **对账验证**：一次性 node 脚本（读 session 目录 + 网关日志，8/1 起消息级对账，断言差 <1%）作为验收证据，不进测试套件（依赖外部数据）。

## Out of Scope

- CLI `--since/--until` 会话级口径**不改**（Q4 决策，保持口径 A）。
- 时间参数时区**不改**（Q3 决策，保持本地 CST 语义，文档化）。
- **不消除覆盖差异**：8/1 网关刚启用、直连请求只在 session、其他客户端请求只在网关——无法从 session 文件判断请求是否走网关，结构性接受。
- **不读网关日志作为数据源**（用户约束：数据只来自 session 目录）。
- 不做消息 id 级跨会话去重（ADR-0001 备选方案，未采纳）。
- webui 会话管理、导出字段结构不变（输入列保持原始字段，ticket 24 决策）。

## Further Notes

- **npm 重新发布**：ticket 25 fork 去重（src 已含）未进入 2026.8.6 发布版（npm 包 dist 无去重逻辑）。按发布流程（记忆 #40）bump `2026.8.6-1` prerelease → push tags → Actions 自动 typecheck+test+build+publish。本地 dist 陈旧问题已 build 修复（.gitignore 不追踪 dist）。
- **对账采样**（2026-08-06）：8/1 起累计 session 940.5M vs 网关 935.0M（+5.5M，0.6%）；8/2、8/4 分毫不差；8/1 网关刚启用（仅 4 条记录，覆盖缺失 +37.2M）；8/3/8/5 网关多出其他客户端请求（-11M/-20M）；8/5 的 019fd362 fork 会话（39.55M 复制历史）去重后差异转负。
- **CLI 对账提示**：CLI 全量窗口含 8/1 前数据（约 494M），与网关不可比；对账建议走 webui「自 8/1」或 `--since` 会话级注意跨天语义。
