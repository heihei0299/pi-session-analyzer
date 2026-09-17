# 05: 让 server refresh 状态以并发安全快照暴露

**What to build:** 让 watcher、RefreshNow 和 HTTP 查询并发运行时，refresh status 的错误、时间戳和 source 归属始终可安全读取，不发生 data race 或可变状态泄漏。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] HTTP QueryMeta 和 db/meta 读取 refresh 状态时不会与 watcher/RefreshNow 发生 map 并发读写。
- [x] 状态 accessor 返回值快照或等价的不可变副本，调用方不能在锁释放后读取共享可变 map。
- [x] Pi 和 Codex 的失败状态继续按请求 source 过滤，不互相污染。
- [x] Refresh 失败时旧 normalized ledger snapshot 仍可查询，失败信息仍可通过既有 meta/diagnostics 观察。
- [x] 回归测试覆盖并发状态读取、Pi-only、Codex-only 和 All source。
- [x] 不新增独立的 refresh 状态存储或第二套错误传播路径。

## Comments

- Source spec: `.scratch/token-analyzer-maintainability-review/spec.md`
- Triage: ready-for-agent

---

## Completion note

- 修改摘要：`lastRefresh` 在锁内返回 detached error-map snapshot，避免 watcher/RefreshNow 与 HTTP QueryMeta/db-meta 的 map race；新增 Pi/Codex/All 过滤及 watcher 并发回归。
- 验证：`TOKEN_ANALYZER_DB= go test ./internal/server`，27 个测试通过；`TOKEN_ANALYZER_DB= go test -race ./internal/server -run '^TestRefreshStatusSnapshotIsSafeDuringConcurrentReads$'`，通过。
- Review：完整 Standards/Spec 双轴 Review 已通过；private accessor、source filtering 与 watcher concurrency findings 已增量复核关闭。
- Commit：待提交后记录。
- 未解决边界问题：无。
