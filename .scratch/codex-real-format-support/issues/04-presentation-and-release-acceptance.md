# 04: 展示口径收口与 Codex 适配发布验收

**What to build:** 用户的按日/周/月汇总落在实际使用的日子上，All 模式既能看见 Pi 的美元成本、也知道这个合计里含未定价源；维护者能用固定命令判断这次 Codex 适配是否真的可交付。

**Blocked by:** 02: Codex usage 口径对齐与覆盖率可见性；03: 源能力声明与出口诚实性（Go、npm/TS 与文档）

**Status:** resolved

- [x] Codex 与 All 的 period 窗口按 usage event 的消息 timestamp 归属，跨天 rollout 的消耗按实际使用日拆分，与领域术语表的消息级归属一致（不再按 rollout header timestamp 整段落到起始日）。
- [x] All 模式保留可定价源的美元合计并标注「含 unpriced 源 / 部分可用」：不把含未定价源的窗口整块显示为 unpriced 而丢失已知成本，也不显示为真实 `$0`。
- [x] Codex 单源窗口成本仍为 `unpriced`；JSON 与 CSV 的 `costStatus` 仍能区分 unpriced 与真实零花费。
- [x] 真实同形 fixture 端到端跑通 `pi|codex|all` × totals / sessions / groups / period / meta 全窗口，且每条修复都有「改动前会红」的用例。
- [x] 本机真实 Codex home 人工对账记录写入验收说明：物理 rollout 全部被发现（本机 74 个）、token 汇总与 Codex 自报一致（按修正口径约 1.457 亿，而非修复前的 2.86 亿）、`cacheRate` 正确（约 96.6%）、未计入覆盖率以诊断形式可见。
- [x] 按项目编译授权执行并通过：Go 单元与集成测试、CLI 与服务端构建、Linux/macOS/Windows release 交叉构建。
- [x] 领域术语表的 Codex 计入口径与源能力词条、以及跨源 input / cacheRead 语义归一的 ADR 已落库，ADR-0002 的 `totalTokens` 公式未被改动。
- [x] 确认未读取或提交任何真实 rollout、会话文件、本地数据库、token 或凭据；fixture 全部为合成数据。

## Implementation summary

- period 归属改为逐 usage event 的 message timestamp：Go `SessionData.PeriodRowsFromFiles` 与 TS `SessionData.periodRowsFromFiles` 均按 item timestamp 分桶；跨天 rollout 会拆到实际使用日。
- All 成本呈现：Go aggregate 继续保留 Pi 的 `cost` 汇总并维持 `costStatus=unpriced` 表示含未定价源；WebUI 新增 `fmtCostCell` / 卡片 partial 分支，会话/分组/period 表显示已知金额并标注「含 unpriced 源」，卡片标题标注「部分可用」。
- CLI table 对 partial 成本显示 `$X*`，并在表尾追加 `* 含 unpriced 源：cost 为可定价源合计，成本仅部分可用`；JSON/CSV 继续输出 `costStatus` 与真实 `cost`，可区分完全 unpriced、partial 与 priced zero。
- 新增合成 fixture `internal/codex/testdata/codex-period-home`：单个跨天 rollout（2026-09-08 23:59 → 2026-09-09 00:00），无任何真实数据；新增 Go query 端到端用例覆盖 codex/all 的 totals / sessions / groups / period / meta，以及 All 部分成本。
- 新增 TS `periodRowsFromFiles` 跨天回归与 WebUI partial 成本静态回归。
- 文档：`CONTEXT.md` 增加 All 模式成本状态词条；README 更新花费与时间归属说明。

## Manual acceptance record

沿用 issue 02 在同一实现基线上执行的本机真实 Codex home 人工对账，未在仓库内复制任何真实文件：

- 物理 rollout 发现：**74/74**（修复前 0）。
- 1109 条 Codex usage event 全部入账；`totalTokens` = **145,748,798**（逐条等于 Codex 自报 `usage.total_tokens`；修复前 285,975,614，虚高 96.2%）。
- `input` = 4,937,882（非缓存）、`cacheRead` = 140,226,816、`output` = 584,100、`cacheWrite` = 0、`reasoning` = 271,816。
- `cacheRate` = **0.9660**（修复前 0.4911）。
- 覆盖率诊断：25 个 snapshot-only 物理 rollout、738 条未计入 `token_count` 快照，`meta.uncountedSnapshots = 738`；连续两次查询保持一致。
- 本机验收全程使用临时账本库，仓库内 fixture 均为占位 ID 的合成数据。

## Comments

- 2026-09-11 验证：
  - `go test ./...` 全绿（设置临时 `TOKEN_ANALYZER_DB` 隔离默认库）；新增 `TestQueryCodexPeriodUsesMessageTimestampAndAllKeepsKnownCost`、`TestRenderTotalsShowsPartialCostAnnotation`。
  - `go vet ./...` clean；`go build ./cmd/token-analyzer` 通过；`make release` Linux/darwin/windows 5 个交叉产物通过。
  - `npm run typecheck` clean；`npm test` 320/320 通过（含新增 `test/40-cost-and-period-presentation.test.ts`）。
  - CLI 手验 `--source codex --period day`：跨天 fixture 输出 2026-09-08 / 2026-09-09 两行，13 / 26 tokens；All 手验输出 `$0.2500*` 与「含 unpriced 源」图例。
