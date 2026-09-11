# 04: 展示口径收口与 Codex 适配发布验收

**What to build:** 用户的按日/周/月汇总落在实际使用的日子上，All 模式既能看见 Pi 的美元成本、也知道这个合计里含未定价源；维护者能用固定命令判断这次 Codex 适配是否真的可交付。

**Blocked by:** 02: Codex usage 口径对齐与覆盖率可见性；03: 源能力声明与出口诚实性（Go、npm/TS 与文档）

**Status:** ready-for-agent

- [ ] Codex 与 All 的 period 窗口按 usage event 的消息 timestamp 归属，跨天 rollout 的消耗按实际使用日拆分，与领域术语表的消息级归属一致（不再按 rollout header timestamp 整段落到起始日）。
- [ ] All 模式保留可定价源的美元合计并标注「含 unpriced 源 / 部分可用」：不把含未定价源的窗口整块显示为 unpriced 而丢失已知成本，也不显示为真实 `$0`。
- [ ] Codex 单源窗口成本仍为 `unpriced`；JSON 与 CSV 的 `costStatus` 仍能区分 unpriced 与真实零花费。
- [ ] 真实同形 fixture 端到端跑通 `pi|codex|all` × totals / sessions / groups / period / meta 全窗口，且每条修复都有「改动前会红」的用例。
- [ ] 本机真实 Codex home 人工对账记录写入验收说明：物理 rollout 全部被发现（本机 74 个）、token 汇总与 Codex 自报一致（按修正口径约 1.457 亿，而非修复前的 2.86 亿）、`cacheRate` 正确（约 96.6%）、未计入覆盖率以诊断形式可见。
- [ ] 按项目编译授权执行并通过：Go 单元与集成测试、CLI 与服务端构建、Linux/macOS/Windows release 交叉构建。
- [ ] 领域术语表的 Codex 计入口径与源能力词条、以及跨源 input / cacheRead 语义归一的 ADR 已落库，ADR-0002 的 `totalTokens` 公式未被改动。
- [ ] 确认未读取或提交任何真实 rollout、会话文件、本地数据库、token 或凭据；fixture 全部为合成数据。
