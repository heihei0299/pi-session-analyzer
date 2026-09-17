# 06: 保留 Pi 增量同步的 diagnostics 历史

**What to build:** 让 Pi 文件中的 malformed record、跳过记录和同步失败在增量 Refresh、cursor replay 和重试后仍然可观察，同时不影响有效 usage 和 cursor 原子性。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] 首次同步产生的 malformed/invalid usage diagnostics，在文件追加合法记录后仍保留。
- [x] cursor 命中时继续 replay 该文件已有 diagnostics。
- [x] 成功增量同步会合并旧 summary 与新 suffix diagnostics，而不是无条件覆盖旧 summary。
- [x] revision、parse、mutation 或 commit 失败不会推进错误 cursor，也不会删除已有有效 ledger rows。
- [x] 下次 Refresh 可以重试失败文件，并且成功重试不会重复记账。
- [x] QueryMeta、CLI 输出和 HTTP/WebUI 状态能够观察到保留后的 source diagnostics。
- [x] 回归测试覆盖 malformed record、追加合法记录、cursor hit、mutation failure 和 retry。

## Comments

- Source spec: `.scratch/token-analyzer-maintainability-review/spec.md`
- Triage: ready-for-agent

---

## Completion note

- 修改摘要：Pi 增量同步在 cursor seek 时合并既有同源 diagnostics 与新 suffix diagnostics；failure persistence 保留旧 summary，跨 source summary fail closed；新增 append/revision failure 回归。
- 验证：`TOKEN_ANALYZER_DB= go test ./internal/pi ./internal/query`，39 个测试通过。
- Review：完整 Standards/Spec 双轴 Review 已通过；SQL seam、旧 summary 覆盖和跨 source 隔离 findings 均已增量复核关闭。
- Commit：待提交后记录。
- 未解决边界问题：无。
