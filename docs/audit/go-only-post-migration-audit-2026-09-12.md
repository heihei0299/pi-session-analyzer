# Go-only 后端迁移后审计报告

**审计日期:** 2026-09-12  
**审计范围:** `50969347ec5de93ee261676ec95b65a0ac56d0ff...6a430d1f217ab973030eb32892fc1db725edfb4f`  
**实现提交数:** 6  
**审计方式:** 静态 code review；未在本轮重新执行 build/test/lint/typecheck  
**规范来源:** `.scratch/go-only-backend-migration/spec.md`、01–05 tickets、`CONTEXT.md`、ADR-0001..0005、`AGENTS.md`

> 本文是 2026-09-12 修复前的历史基线审计，不代表当前实现或发布结论；当前验收以 06/07 ticket、代码与 CI 结果为准。

---

## 1. 结论

本轮迁移已经完成了主要架构目标：

```text
Pi / Codex
    ↓
 Refresh
    ↓
normalized SQLite ledger
    ↓
single Go Query Engine
    ↓
CLI / HTTP / Watch / WebUI
```

同时：

- OpenCode 已物理抽离到独立 `opencode-analyzer/` Go module；
- token-analyzer 的 TypeScript/Node 后端已退出；
- Go server 已成为唯一生产 server；
- WebUI 已收敛为单源码；
- Codex 已退出 `ledger → SessionFileData → memory aggregation` 回绕；
- GET 查询已切为 read-only ledger snapshot；
- Watch 已从独立 token aggregation 改为 change → refresh → query。

**但当前不建议签署“迁移完成 / 可直接发布”结论。**

本次审计确认：

- **P0: 2 项**
- **P1: 8 项**
- **P2: 3 项**

其中两个 P0 都直接影响 ledger 正确性或历史统计完整性：

1. Pi Sync 没有使用真正的 `sql.Tx`，且大量数据库错误被吞掉；
2. 新 Query Engine 未读取 `usage_daily_rollups`，与当前 ledger contract 不一致。

---

## 2. 已正确完成的架构项

### 2.1 OpenCode 产品边界已真正拆开

当前：

- `opencode-analyzer/go.mod` 为独立 module；
- token-analyzer 生产入口不再暴露 OpenCode API/UI/credential；
- OpenCode 不进入 `source=all`；
- OpenCode 不依赖 token-analyzer `internal/*`；
- token-analyzer 不反向依赖 OpenCode runtime。

这部分符合 Ticket 01 的核心方向。

### 2.2 Query / Refresh 已完成主路径分离

`internal/query.Query` 使用 `db.OpenReadOnly`，并启用：

```sql
PRAGMA query_only = ON
```

Query 本身不再做 discovery / parse / sync。

这是正确的终态边界。

### 2.3 Codex 已改为 ledger-native 查询

旧 `internal/codex/query.go` 已删除，新查询从 ledger / source_sessions 构建结果，不再恢复成 `SessionFileData` 后交给旧 SessionData aggregation。

### 2.4 Watch 已退出独立 token math

旧 `internal/watch` 已删除。CLI Watch 与 server watcher 都通过 Refresh + Query 获取结果。

### 2.5 Go-only release 主方向已落实

`src/`、npm package、TS test oracle、npm publish workflow 已移除；CI/release 转为 Go-only。

---

# 3. Findings

## A-01 — P0 — Pi Sync 的“事务”不是可靠事务

**位置:** `internal/pi/sync.go::SyncPiUsage`

当前代码形态：

```go
_, _ = database.DB.Exec("BEGIN")

_, _ = database.DB.Exec("INSERT ... session_usage_dedup")
_, _ = database.DB.Exec("INSERT ... proxy_request_logs")
_, _ = database.DB.Exec("INSERT ... pi_sessions")
_, _ = database.DB.Exec("INSERT ... session_log_sync")

_, _ = database.DB.Exec("COMMIT")
```

### 问题

`*sql.DB` 是连接池。

调用：

```go
database.DB.Exec("BEGIN")
```

之后继续使用 `database.DB.Exec(...)`，不能保证后续语句使用同一数据库连接，因此不能保证它们处在同一 SQLite transaction 中。

正确边界应该是：

```go
tx, err := database.DB.Begin()
...
tx.Exec(...)
...
tx.Commit()
```

更严重的是当前大量数据库错误通过 `_, _ =` 被直接忽略。

### 风险

可能出现：

```text
usage insert 失败
    ↓
错误被忽略
    ↓
session_log_sync cursor 写成功
    ↓
下次 Refresh 认为该字节范围已经处理
    ↓
usage 永久漏账
```

也可能出现：

- dedup 写成功、usage 写失败；
- usage 写成功、metadata/cursor 写失败；
- cursor 与 ledger 状态不一致；
- retry 后出现不可预测的 skip 行为。

### 违反的 contract

`CONTEXT.md` 明确要求：

> 写路径仅 Refresh，Pi Sync 按文件事务提交。

### 修复要求

- 每个 Pi 文件使用一个真正的 `sql.Tx`；
- dedup / usage / pi_sessions / cursor 必须同 transaction；
- 任意 SQL error → rollback → 返回 error；
- 只有 commit 成功后才增加最终 Imported/Skipped 结果；
- 禁止吞掉 DB mutation error。

---

## A-02 — P0 — Query Engine 忽略 usage_daily_rollups

**位置:** `internal/query/ledger.go`

当前查询全部直接读取：

```sql
proxy_request_logs
```

没有看到 `usage_daily_rollups` 的读取/合并。

### 与当前 contract 冲突

`CONTEXT.md` 当前明确声明：

> Query Engine 的 ledger SQL = proxy_request_logs ∪ rollups

旧 TypeScript `src/db-aggregation.ts` 也明确把 raw rows 与 `usage_daily_rollups` 合并。

### 风险

如果用户已有数据库做过历史 rollup/prune：

```text
old proxy_request_logs 被 prune
      ↓
历史数据仅保存在 usage_daily_rollups
      ↓
Go Query 只读 proxy_request_logs
      ↓
历史 totals/groups/period 直接变少
```

显式复用 cc-switch/shared DB 时风险更高。

### 必须先做的设计决定

二选一，不能保持当前模糊状态：

**方案 A — 保留现有 contract（推荐）**

恢复 Query 对 `proxy_request_logs ∪ usage_daily_rollups` 的支持，并明确哪些窗口可安全使用 rollup。

**方案 B — 正式废弃 rollup contract**

修改 ADR/CONTEXT/schema migration，并明确旧/pruned DB 不兼容。

鉴于当前 spec 明确要求保持已接受行为，本审计推荐 **A**。

---

## A-03 — P1 — Pi incremental stop replacement 只在单批次内有效

**位置:** `internal/pi/sync.go`

当前 `shouldReplacePiRecord` 只用于：

```text
本次 linesToProcess
→ seen[requestId]
```

但一旦旧 request 已经写进 ledger，后续 append 出现相同 `requestId` 的更完整记录时：

```go
SELECT 1 FROM session_usage_dedup
WHERE data_source=? AND request_id=?
```

命中后直接 `continue`。

### 风险

典型过程：

```text
Refresh #1
request X: usage 已出现，但 stopReason 尚未形成最终结论
→ 写 ledger

Refresh #2
append 最终 request X
stopReason / output / failure 状态更新
→ requestId 已存在
→ 直接 skip
```

最终 ledger 可能永久保留早期状态。

### 修复要求

持久层也必须实现“replace candidate”判断，而不只是 batch-local map。

建议 ledger 记录足以比较最终性的信息，或者在同 requestId 命中时读取现有 ledger record 后按 canonical replacement rule 做 UPDATE。

必须增加 **跨 Refresh** regression test。

---

## A-04 — P1 — Production Pi discovery 重新引入递归 fallback

**位置:** `internal/pi/refresh.go`

当前：

```go
files := CollectPiJsonlFiles(resolved.Root, resolved.Layout)
if len(files) == 0 {
    files = collectJsonlRecursive(resolved.Root)
}
```

### 冲突

`CONTEXT.md` 已明确：

> Flat vs ProjectDirectories 按 layout 枚举，不再递归兜底。

旧 TS fallback 主要是迁移期 fixture/ephemeral 兼容。

### 风险

规范布局之外更深层的 `*.jsonl` 可能被误纳入 production usage。

### 修复

- production Refresh 严格按 resolved layout；
- canonical fixture 调整为正式布局；
- 测试辅助逻辑放 test helper，不进入 production discovery。

---

## A-05 — P1 — 排序 comparator 在相等值下不满足 strict ordering

**位置:** `internal/sessiondata/sessiondata.go::sortSessions / sortRequests`

当前降序逻辑：

```go
if desc {
    return !less
}
```

若 `a == b`：

```text
less(a,b) = false
less(b,a) = false
```

降序后变成：

```text
cmp(a,b) = true
cmp(b,a) = true
```

不满足 Go sort comparator 要求。

### 风险

- 同值 rows 排序结果不稳定；
- 分页边界可能漂移；
- golden “稳定 tie-breaker”要求没有真正实现。

### 修复

改为显式三态比较：

1. primary sort key；
2. timestamp；
3. sessionId / request stable identity。

任何完全相等对象返回 false。

---

## A-06 — P1 — Watch Refresh 失败后不会自动重试

**位置:** `internal/server/server.go::StartWatch`

当前：

```go
if cur == last {
    continue
}
last = cur
_ = s.RefreshNow()
```

### 风险

```text
source changed
→ fingerprint 变化
→ last = cur
→ Refresh 临时失败
→ 下一 tick fingerprint 未继续变化
→ cur == last
→ 永远不 retry
```

快照会一直旧到下一次文件变化。

### 修复

仅在 Refresh 成功后推进 acknowledged fingerprint：

```go
if err := s.RefreshNow(); err == nil {
    last = cur
}
```

失败时保留旧 fingerprint，并按正常 interval retry。

---

## A-07 — P1 — Watch fingerprint 不足以覆盖 same-size rewrite

**位置:** `internal/refresh/orchestrate.go::fingerprintRoots`

当前 fingerprint 只包含：

- path
- size
- mtime

source adapter 的 Pi revision 明明已经支持 tail fingerprint，但 watcher 没有使用。

### 风险

same-size rewrite 且 mtime 未变化/时间精度碰撞时：

```text
source adapter 有能力发现 rewrite
但 watcher 根本不调用 Refresh
```

### 修复

watch fingerprint 至少加入 source file tail/content revision。

推荐复用 source adapter 的 revision seam，而不是再定义第四套文件变更语义。

---

## A-08 — P1 — Codex home scope 使用裸字符串前缀

**位置:**

- `internal/query/ledger.go::loadCodexMetas`
- `internal/codex/sync.go::LoadDiagnostics`

当前：

```go
strings.HasPrefix(path, home)
```

例如：

```text
home = /tmp/codex
path = /tmp/codex-old/rollout.jsonl
```

也会被判定为属于该 home。

### 修复

使用 path-aware containment：

```text
filepath.Clean
filepath.Rel
reject rel == ".." or starts ../
```

并测试 sibling prefix。

---

## A-09 — P1 — OpenCode standalone audit 的 Pi 统计语义发生回退

**位置:** `opencode-analyzer/internal/piaudit/audit.go`

当前只统计：

```text
type=message
role=assistant
usage != nil
```

### 迁移前行为

旧 OpenCode audit 通过 token-analyzer 的 DB-backed Pi query 取得本地 totals，因此继承了迁移后的 Pi ingestion semantics。

### 当前缺口

Standalone audit 不覆盖完整：

- assistant
- toolResult
- compaction
- branch_summary
- requestId/semantic dedup
- 完整 failed/aborted gate
- 完整 cross-file dedup
- pricing fallback

### 为什么这是 spec finding

Ticket 01 要求：

> 抽离并保留现有 OpenCode 用户能力。

产品边界拆分正确，但本地 Pi 对账数字不应因为拆项目而退化。

### 修复原则

OpenCode 仍保持完全独立，不 import token-analyzer `internal/*`。

可以在 `opencode-analyzer` 内维护自己的 Pi audit parser，但必须用 synthetic canonical fixture 锁住与抽离前 audit 所需的语义。

---

## A-10 — P1 — Rename 成功后立即查询仍可能返回旧 displayName

**位置:** `internal/server/server.go::handleApiSessionRename`

当前 rename：

1. rename filesystem file；
2. invalidate SessionData cache；
3. 返回 200。

但现在详情/列表的 displayName 来自 ledger `pi_sessions`。

WebUI 成功后立即：

- fetch detail；
- refresh session table；
- refresh request table。

此时 ledger 尚未 Refresh，因此可能立即读到旧 displayName，直到 watcher 下一轮同步。

### 修复

rename 的成功条件应包含 metadata 已进入 canonical snapshot。

推荐：

```text
rename file
→ Pi Refresh
→ Query 能看到新 displayName
→ return 200
```

如果 Refresh 失败：

- 文件 rename 已发生，不应该谎称完全失败；
- API 应返回明确 partial/follow-up 状态，或直接对 `pi_sessions` 做受控 metadata update；
- 需要先定义并测试一致性策略。

---

## A-11 — P2 — 旧 SessionData 文件扫描实现仍保留大量过期统计语义

**位置:** `internal/sessiondata/sessiondata.go`

虽然生产 stats Query 已经不调用旧 aggregation，但 rename 的 `FindSessionFile` 仍通过：

```text
ReadSessionFilesCached
→ AnalyzeFile
```

而 `AnalyzeFile` 仍保留 assistant-only usage parsing。

### 问题

为了“定位 session 文件”而执行完整旧 session usage parser，使已经退出生产统计的旧领域逻辑继续存活。

### 建议

新增专用、浅语义 locator：

```text
FindPiSessionFileByHeaderID
```

只：

- enumerate Pi files；
- 读 header；
- 比 session id。

然后删除：

- `MessageItem`
- `SessionFileData.Items`
- `AnalyzeFile` 中 usage aggregation 相关逻辑；
- 不再必要的 old cache/flight machinery。

这能让 ADR-0005 的 deletion test 更真实。

---

## A-12 — P2 — Ticket tracker 状态与实现不一致

当前：

- 01 = resolved
- 02 = resolved
- 03 = ready-for-agent
- 04 = ready-for-agent
- 05 = ready-for-agent

但对应代码 commit 已经存在：

- 03 → `56d695d`
- 04 → `7b570d9`
- 05 → `6a430d1`

### 风险

后续 agent 可能把 03 当 frontier 再执行一次。

### 修复

修复代码并完成最终验证后：

- 03/04/05 → `Status: resolved`
- 增加 `## Answer`
- Answer 中写 commit / 核心实现 / 验证命令。

---

## A-13 — P2 — 普通 Make build 仍输出 token-analyzer-go

**位置:** `Makefile`

当前：

```make
BINARY_NAME = token-analyzer-go
```

ADR-0005 已决定不再区分 Go/npm edition。

### 修复

普通 build 收敛到：

```text
dist/token-analyzer
```

release artifact 保持带 platform suffix。

---

# 4. 风险排序

## 必须阻止发布

1. A-01 Pi transaction / swallowed errors
2. A-02 rollup query contract

## 应在同一修复周期完成

3. A-03 cross-refresh replacement
4. A-04 discovery fallback
5. A-05 deterministic sorting
6. A-06 watch retry
7. A-07 watcher revision fingerprint
8. A-08 Codex path containment
9. A-09 OpenCode audit semantics
10. A-10 rename snapshot consistency

## 收尾

11. A-11 SessionData legacy cleanup
12. A-12 tracker resolved state
13. A-13 Makefile binary naming

---

# 5. 发布签署条件

只有同时满足以下条件才建议将 Go-only migration 标记为完成：

- Pi 每文件 ledger write 使用真实 `sql.Tx`；
- mutation SQL error 不再被忽略；
- cursor 不可能在 usage write 失败时前进；
- cross-refresh same-request finalization 有测试；
- Query 与 rollup contract 一致；
- production Pi discovery 与 CONTEXT 一致；
- sorting 对 equal keys 稳定；
- Watch refresh failure 会 retry；
- same-size rewrite 能触发 watcher；
- Codex home sibling prefix 不串数据；
- rename 200 后立即 Query 可见新名称；
- OpenCode local Pi audit 不因 standalone extraction 退化；
- 03/04/05 tracker 与真实完成状态一致；
- Go-only canonical tests 覆盖上述 regression。

---

# 6. 本轮未做事项

本轮 review 没有重新执行：

- `go test`
- build
- lint
- typecheck
- cross-platform compile

原因是仓库规则要求 build/test/lint/typecheck 需要当次明确授权。

实现提交中记录的历史测试结果可作为参考，但不能替代本审计后的修复验证。
