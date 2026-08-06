# ADR-0002 — totalTokens 口径对齐 pi-switch 网关（总输入 + 输出）

- **状态**: Accepted（2026-08-06，grill-to-spec 用户确认）
- **影响组件**: `src/aggregate.ts`（finalizeTotals 聚合收尾）、CLI / webui / 导出（共用聚合层）

## 背景

token-analyzer 与 pi-switch 网关（`~/.pi-switch/requests.log`）对比总量时口径定义不同：

- token-analyzer `totalTokens = input + output + cacheRead + cacheWrite`（组件和，原始 spec 决策锚）
- 网关 `total = promptTokens + completionTokens`（总输入 + 输出）

实测 pi 会话 cacheWrite 恒为 0，两者当前数值相等；但定义不同，未来某模型开始记录
cacheWrite 时数字即漂移，对账将失准。

## 决策

`totalTokens` 显式定义为「总输入 + 输出」= `input + cacheRead + output`（与网关 total 定义一致），
cacheWrite 降为独立指标列（CLI/JSON/CSV/webui 保留展示），不再计入总量。

## 后果

**正面**

- 与网关 total 定义完全一致，防未来 cacheWrite 非零时漂移；对账语义清晰。
- 实测对账（2026-08-06，fork 去重生效后）：8/1 起累计 session 940.5M vs 网关 935.0M（差 0.6%）；8/2、8/4 分毫不差。

**负面 / 约束**

- 与原始 spec「组件和」决策锚及已发布 npm 版行为不同（当前数值相同，无实际差异）。
- CLI/JSON/CSV/webui 展示与测试需同步调整（totalTokens 计算与断言）。
- 8/1 前数据（session 独有，网关无）仍使「全部」窗口与网关不可比——结构性，见 spec「网关可比窗口」。

## 备选方案（未采纳）

- **保持组件和**（input+output+cacheRead+cacheWrite）：与 pi 自身 jsonl-storage.js 口径一致，但定义未对齐网关，未来 cacheWrite 非零即漂移。
