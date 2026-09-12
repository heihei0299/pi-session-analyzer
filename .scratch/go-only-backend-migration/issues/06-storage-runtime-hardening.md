# 06: 收尾 storage lifecycle 与 runtime 变更检测

**What to build:** 补齐 Go-only 迁移后仍缺失的长期运行能力：恢复 `rollup_and_prune(30)` 生命周期、让 partial-day 查询不会误用 daily rollup、为旧 Pi ledger 提供一次性自愈迁移，并把 Codex watch 从“周期性全量读取/解压”收敛为廉价且按 source 触发的变更检测。

**Blocked by:** 05: 删除 TypeScript 后端并完成 Go-only contract / release 收尾.

**Status:** resolved

- [x] Refresh 成功后恢复统一的 rollup/prune maintenance：将超过保留窗口的 raw usage 聚合写入 `usage_daily_rollups`，随后删除对应 raw rows，并按既有 storage contract 执行必要的 incremental vacuum。
- [x] rollup writer 保持幂等：重复 maintenance 不得重复累计同一 raw usage；失败不得留下“已 rollup 但 raw 未删”或“raw 已删但 rollup 未完整写入”的半状态。
- [x] 明确 daily rollup 的查询能力边界：只有能够由完整自然日安全回答的 MessageTimeRange 才可直接使用 rollup。
- [x] 当查询包含小时/分钟级 partial-day 边界、而对应历史 raw 已被 prune 时，不得把整日 rollup 当作精确结果；必须返回明确 coverage warning / partial coverage 状态，而不是静默给错数字。
- [x] 增加 raw + rollup + partial-day synthetic regression，覆盖 totals / model groups / period，并证明 session/request/detail 不伪造 rollup identity。
- [x] 增加 Pi sync semantics version（或等价 migration marker），用于识别可能由旧实现写入的 ledger/cursor。
- [x] 从旧 sync semantics 升级时，使受影响的 Pi cursor 失效并触发安全重扫，使旧 partial→final replacement 错账能够按新 replacement 规则自愈。
- [x] 自愈迁移不得重复计数；已有正确 raw ledger 在重扫后保持幂等。
- [x] 如果历史记录已经只存在于 rollup，必须明确并测试其可恢复边界；无法重建 request/session identity 的信息不得伪造。
- [x] Codex watch 不再每个轮询周期为了判断“有没有变化”而完整读取/解压全部 rollout。
- [x] Pi 与 Codex 使用 source-specific fingerprint / refresh：Pi 变化只触发 Pi Refresh，Codex 变化只触发 Codex Refresh；`source=all` 不因一侧变化无条件重扫另一侧。
- [x] Codex change detector 使用廉价 physical revision（例如 path/size/mtime/必要的轻量 tail metadata），只有确认候选变化后才进入完整 rollout revision / parse。
- [x] Watch failure retry、same-size rewrite 检测与 snapshot 保留语义不得回归。
- [x] 更新 `CONTEXT.md` / ADR，明确 rollup writer、partial-day coverage、自愈 migration 和 source-specific watch 的终态语义。

## Answer

- 已完成 30 天 `RollupAndPrune` 生命周期：raw usage 在事务内聚合、删除并幂等维护；rollup 保留 reasoning，旧 rollup schema 可迁移且只读 Query 可兼容。
- 已完成 partial-day coverage、Pi sync semantics migration、自愈重扫、Codex cheap physical fingerprint 与 Pi/Codex/All source-specific watch；CLI 与 server 均在 refresh 成功后确认 source revision，失败保留旧快照并重试。
- 验证：`env -u TOKEN_ANALYZER_DB GOMAXPROCS=2 go test -p 1 ./...` 通过；`(cd opencode-analyzer && GOMAXPROCS=2 go test -p 1 ./...)` 通过；`git diff --check` 与 `gofmt` 静态检查通过。

## Acceptance focus

```text
Refresh
  -> atomic raw ledger
  -> idempotent rollup/prune maintenance
  -> Query (raw + safe rollup coverage)

Watch
  -> cheap source-specific change detection
  -> refresh changed source only
  -> same Query snapshot
```

完成后，长期运行的数据库不会无限只增不减，历史 rollup 不会在小时级过滤下给出伪精确结果，旧 bug 写入的 Pi ledger 可以在升级后自愈，Codex 历史增大也不会让 watcher 周期性做全量解压扫描。
