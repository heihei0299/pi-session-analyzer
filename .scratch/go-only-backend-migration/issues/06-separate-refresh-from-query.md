# 06: 将 Refresh 生命周期与 Query 彻底分离

**What to build:** 用户读取 totals、sessions、groups、period 或 meta 时只读取最近一次已提交 snapshot；源发现与同步由明确的 Refresh 生命周期完成。连续 GET 不再隐式写数据库，refresh 失败时用户仍能读取上一次成功结果并看到诊断。

**Blocked by:** 03: Pi 会话、请求与详情窗口切到 Go ledger; 04: Pi 分组、周期、时间范围与 meta 切到 Go ledger; 05: Codex 与 All 改为 ledger-native 查询.

**Status:** ready-for-agent

- [ ] Query 调用不执行 source discovery、文件解析、游标推进或 ledger 写入。
- [ ] Go server 启动时能完成初次 Pi/Codex refresh，再提供 snapshot 查询。
- [ ] 后续 refresh 使用单一 orchestration，Pi 与 Codex 各自由 source adapter 写入 ledger。
- [ ] 并发 refresh 被合并或串行化，多个 HTTP 读取不会放大为重复同步。
- [ ] refresh 失败不回滚上一个成功 snapshot；读取仍成功，并通过 meta/diagnostics 暴露失败信息。
- [ ] 连续调用同一 GET 查询不会改变 ledger 内容或同步游标。
- [ ] CLI 的显式同步/刷新行为与 server refresh 复用同一 source semantics，不复制 ingestion 规则。
