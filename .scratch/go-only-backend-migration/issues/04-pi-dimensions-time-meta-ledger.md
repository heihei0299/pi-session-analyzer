# 04: Pi 分组、周期、时间范围与 meta 切到 Go ledger

**What to build:** Pi 用户按 model/cwd 分组、按 day/week/month 查看趋势、使用 since/until 或读取 meta 时，所有结果通过同一个 ledger-backed Query Engine 产生，并完整保留 SessionTimeRange 与 MessageTimeRange 的既有双语义。

**Blocked by:** 02: Pi totals 全链路切到 Go normalized ledger.

**Status:** ready-for-agent

- [ ] groups 支持 model、cwd、model+cwd，并与 totals 使用同一 usage 语义。
- [ ] period 按 usage event 的消息 timestamp 归属，跨天会话按实际使用日拆分。
- [ ] WebUI 使用 MessageTimeRange，CLI 会话级 since/until 保留 SessionTimeRange 语义。
- [ ] 纯日期边界继续按本地日首/日尾解释，无效时间戳保持既有保守策略。
- [ ] meta 的 session count、data range、sources/diagnostics 使用真实 ledger/source snapshot，不通过旧扫描聚合路径伪造。
- [ ] groups/period/meta 的 CLI/API/WebUI 可观察结果与 canonical expected 一致。
- [ ] Query Engine 对同一 ledger snapshot 的输出 deterministic，并有稳定排序。
