# Go-only 迁移审计修复执行计划

**基线:** `6a430d1f217ab973030eb32892fc1db725edfb4f`  
**输入:** `docs/audit/go-only-post-migration-audit-2026-09-12.md`  
**目标:** 修复审计发现，同时保持当前 Go-only / standalone OpenCode 架构，不重新引入 TS 或跨项目耦合。  
**原则:** 先正确性，再 runtime 一致性，再产品边界/清理，最后 contract 收口。

---

# 1. 执行总览

推荐按 5 个 work package 顺序执行：

```text
R1 Ledger atomicity + Pi incremental correctness
                 ↓
R2 Query/storage/discovery correctness
                 ↓
R3 Runtime snapshot consistency
                 ↓
R4 OpenCode audit parity + legacy cleanup
                 ↓
R5 Contract/tracker/release closeout
```

不要并行 R1/R2。

R3 可在 R2 稳定后执行。

R4 不依赖 token-analyzer runtime implementation，可以在 R2 后与 R3 部分并行，但为了 review 简洁，推荐顺序完成。

---

# R1 — 修复 Pi ledger 原子性与增量 replacement

**优先级:** P0  
**主要 findings:** A-01、A-03  
**核心文件:**

- `internal/pi/sync.go`
- `internal/pi/sync_helpers.go`
- `internal/pi/sync_test.go`
- 必要时 `internal/db/*`

## R1.1 用真正的 sql.Tx 替换伪事务

每个文件同步改成：

```go
tx, err := database.DB.Begin()
if err != nil { ... }

defer rollback unless committed

// dedup
// request insert/update
// pi_sessions
// session_log_sync

if err := tx.Commit(); err != nil { ... }
```

所有 mutation 都通过 `tx.Exec/tx.QueryRow`。

### 禁止

- `DB.Exec("BEGIN")`
- `DB.Exec("COMMIT")`
- `_, _ = tx.Exec(...)`
- mutation failure 后继续推进 cursor。

## R1.2 Imported / Skipped 只在 commit 成功后生效

建议每文件先维护：

```text
fileImported
fileSkipped
```

commit 后再加到 total result。

rollback 不应该污染返回统计。

## R1.3 实现 persistent replacement

需要把 canonical replacement rule 从“本批次 seen map”扩展到“已存在 ledger row”。

目标行为：

```text
same requestId:
new final stop > existing non-final
same finality → larger output wins
otherwise keep existing
```

### 推荐实现

不要仅判断 `session_usage_dedup` 是否存在。

命中同 requestId 时：

1. load existing `proxy_request_logs`；
2. 读取足够信息判断 existing finality；
3. compare canonical candidate；
4. 若新 candidate 胜出：
   - UPDATE usage/status/error/model/cost/timestamp 等 canonical fields；
   - 更新 semantic bookkeeping；
5. 若不胜出 → skip。

如果当前 schema 缺少 `stopReason/finality` 信息，优先增加最小稳定字段，而不是根据 `status_code` 猜全部状态。

## R1.4 错误注入测试

增加 transaction failure regression。

至少证明：

### Case A

```text
usage insert fails
→ cursor unchanged
→ no dedup residue
→ retry imports record
```

### Case B

```text
cursor insert fails
→ usage/dedup rollback
→ retry imports exactly once
```

可用：

- test-only trigger；
- deliberately invalid schema；
- 可注入 exec seam。

不要通过 sleep/race 猜测 transaction。

## R1.5 Cross-refresh replacement test

fixture：

```text
initial file:
request a1
usage output=10
no final stop

Refresh #1

append:
same entry/request identity
output=20
stopReason=stop

Refresh #2
```

断言最终：

- requests 仍为 1；
- output = 20；
- no duplicate；
- cursor 正确；
- repeated Refresh 幂等。

## R1 Definition of Done

- [ ] Pi 每文件使用真实 sql.Tx。
- [ ] 所有 mutation error 可见。
- [ ] failed file transaction 不推进 cursor。
- [ ] rollback 后 retry 可恢复。
- [ ] cross-refresh replacement 正确。
- [ ] existing append/partial/truncate/rewrite contract 不回归。

---

# R2 — 修复 Query / rollup / discovery / Codex scope

**优先级:** P0/P1  
**主要 findings:** A-02、A-04、A-08  
**核心文件:**

- `internal/query/ledger.go`
- `internal/pi/refresh.go`
- `internal/codex/sync.go`
- `internal/query/*_test.go`
- `CONTEXT.md` / ADR 仅在实现决策变化时修改

## R2.1 恢复 rollup-aware Query

先明确每种 window 的能力。

### totals

支持：

```text
raw proxy_request_logs
+
compatible usage_daily_rollups
```

### groups

- model grouping 可以合并 rollup；
- cwd grouping 无法从当前 rollup schema 恢复 cwd 时，不得伪造；
- 必须沿用既有 contract 的处理方式。

### period

按 date 合并 raw + rollup。

### sessions / requests / detail

rollup 无法恢复 session/request identity，因此：

- 只使用 raw rows；
- 如果历史已 prune，应明确窗口能力/coverage；
- 不要生成伪 session。

该行为要写入测试和文档。

## R2.2 增加 pruned-ledger fixture

直接构建 ledger：

- old day → only in `usage_daily_rollups`
- recent day → raw `proxy_request_logs`

断言：

- totals = old + recent；
- group model = old + recent；
- period = 两边正确；
- session/request 不凭 rollup 伪造。

## R2.3 删除 production recursive fallback

`internal/pi/refresh.go`：

```text
ResolvePiSessionRoot
→ CollectPiJsonlFiles(layout)
```

结束。

不要 recursive fallback。

调整 canonical testdata 布局，而不是污染 production behavior。

增加测试：

```text
projectDirectories layout
└─ project/
   └─ valid.jsonl         counted
└─ project/deeper/
   └─ nested.jsonl        NOT counted
```

## R2.4 Path-aware Codex home containment

抽一个小 helper，例如：

```go
func pathWithin(root, candidate string) bool
```

要求：

- clean/abs；
- `filepath.Rel`；
- root 本身允许；
- `../` sibling 拒绝；
- Windows volume/sep 可工作。

统一用于：

- source_sessions scope；
- diagnostics scope。

测试：

```text
/home/u/.codex
/home/u/.codex-old   // must not match
/home/u/.codex/a     // match
```

## R2 Definition of Done

- [ ] raw + rollup totals 不漏历史。
- [ ] rollup 能力边界明确。
- [ ] production Pi discovery 不 recursive fallback。
- [ ] Codex sibling prefix 不串数据。
- [ ] canonical Query tests 覆盖 pruned ledger。

---

# R3 — 修复 runtime snapshot 一致性

**优先级:** P1  
**主要 findings:** A-05、A-06、A-07、A-10  
**核心文件:**

- `internal/sessiondata/sessiondata.go`
- `internal/refresh/orchestrate.go`
- `internal/server/server.go`
- `cmd/token-analyzer/main.go`
- runtime/watch/server tests

## R3.1 重写 deterministic sort comparator

不要：

```go
return !less
```

使用 compare function：

```text
primary
→ timestamp
→ sessionId / stable row key
```

规则：

- asc 与 desc 只翻转非零 comparison；
- equal 必须返回 false；
- pagination 前排序必须 deterministic。

测试至少包括：

- 两行 primary metric 相同；
- timestamp 相同；
- stable id 不同；
- 多次 sort 结果一致；
- page 1/page 2 不漂移。

## R3.2 Watch acknowledgement 只在 Refresh 成功后推进

Server watcher：

```text
observed fingerprint != acknowledged fingerprint
→ Refresh
→ success: acknowledged = observed
→ failure: keep old acknowledged and retry next tick
```

CLI Watch 同一语义。

增加 test：

1. source change；
2. injected Refresh failure；
3. source 不再变化；
4. next tick retry；
5. success 后 snapshot 更新。

## R3.3 fingerprint 覆盖 same-size rewrite

不要只 hash `path+size+mtime`。

推荐把 change detector 建立在 source revision 之上。

Pi 可复用：

- file size
- tail fingerprint
- complete
- mtime

Codex 使用对应 rollout revision。

如果为了 orchestrator 解耦需要 source 提供：

```go
Fingerprint(root) (string, error)
```

可以由 source adapter 实现。

### Test

- 写 fixture；
- 获取 fingerprint；
- same-size 内容替换；
- restore mtime；
- fingerprint 必须变化。

## R3.4 Rename 后 snapshot 必须一致

推荐执行流：

```text
validate
→ rename filesystem
→ Refresh Pi
→ QueryDetail verifies new metadata
→ 200
```

需要定义 Refresh failure 行为。

推荐 API 语义：

- rename filesystem 成功但 refresh 失败：
  - 不尝试危险 rename rollback；
  - 返回 500/503 + 明确“文件已改名但 snapshot refresh failed”；
  - watcher 后续 retry；
- 成功响应必须保证 Query immediately sees new name。

测试：

```text
POST rename → 200
immediately GET detail
immediately GET sessions
both expose new displayName
```

## R3 Definition of Done

- [ ] equal-key sorting deterministic。
- [ ] Watch failure 自动 retry。
- [ ] same-size rewrite 可触发 Refresh。
- [ ] rename 200 后 snapshot 已一致。
- [ ] GET 仍无写副作用。

---

# R4 — OpenCode audit parity 与 SessionData legacy cleanup

**优先级:** P1/P2  
**主要 findings:** A-09、A-11  
**核心文件:**

- `opencode-analyzer/internal/piaudit/*`
- `opencode-analyzer/internal/piaudit/*_test.go`
- `internal/sessiondata/sessiondata.go`
- server rename locator

## R4.1 给 standalone OpenCode 建立自己的 Pi audit contract

保持产品完全独立：

```text
opencode-analyzer -X-> token-analyzer/internal/*
```

但不能用 assistant-only 代替旧 audit semantics。

至少覆盖：

- assistant
- toolResult
- compaction
- branch_summary
- billable/cost/failed gate
- fork copied history
- duplicate request identity
- semantic duplicate
- MessageTimeRange/month boundary
- totalTokens = input + cacheRead + output
- local cost behavior

## R4.2 使用 synthetic fixture 固化 audit

在 `opencode-analyzer` 自己维护 fixture。

不要引用 token-analyzer testdata 的相对路径，否则未来拆仓又产生隐式耦合。

允许 fixture 内容重复，因为这是刻意的产品边界 contract。

## R4.3 缩小 SessionData 到 query DTO/helpers

当前 `sessiondata` 应最终只保留类似：

- Filter
- View
- QueryResult
- QueryMeta
- shared domain helpers
- normalize/display/sort（若仍确实属于这里）

Rename 文件定位改成专用 service/helper：

```text
Pi session locator:
enumerate file
→ read first line only
→ header.id match
```

删除 rename 为了找路径而触发的：

- full JSONL parse；
- usage parsing；
- old assistant aggregation；
- old file cache 若无其他使用。

## R4 Definition of Done

- [ ] OpenCode audit 不退化为 assistant-only。
- [ ] OpenCode 可整目录迁出。
- [ ] OpenCode testdata 自包含。
- [ ] token-analyzer rename 不依赖旧 usage parser。
- [ ] SessionData 不再携带第二套历史 stats implementation。

---

# R5 — Contract、tracker、build/release 收尾

**优先级:** P2  
**主要 findings:** A-12、A-13  
**核心文件:**

- `.scratch/go-only-backend-migration/issues/03-*.md`
- `.scratch/go-only-backend-migration/issues/04-*.md`
- `.scratch/go-only-backend-migration/issues/05-*.md`
- `Makefile`
- `CONTEXT.md`
- ADR-0005
- README（仅实际行为变化时）

## R5.1 Make build 名称收口

```make
BINARY_NAME = token-analyzer
```

普通 build：

```text
dist/token-analyzer
```

release 继续：

```text
token-analyzer-linux-amd64
...
```

## R5.2 更新 tracker

只有 R1–R4 验收通过后再做。

03/04/05：

```text
Status: resolved
```

并填写 `## Answer`：

- implemented boundaries；
-关键 commit；
- regression tests；
-最终验证命令。

不要在实际修复完成前提前勾 resolved。

## R5.3 文档与代码再对齐

重点核对：

- rollup query contract；
- Pi discovery；
- Watch fingerprint/retry；
- Rename snapshot consistency；
- OpenCode audit independence；
- SessionData 的最终职责。

ADR/CONTEXT 应描述代码实际存在的 architecture，而不是目标 architecture。

---

# 2. 最终验证矩阵

> 以下命令属于 build/test，按仓库规则仅在用户明确授权后执行。

## token-analyzer

最低：

```bash
GOMAXPROCS=2 go test -p 1 ./...
```

重点 regression 必须包含：

- Pi transaction rollback；
- cursor atomicity；
- cross-refresh replacement；
- append / partial / truncate / rewrite；
- rollup + raw union；
- Codex path containment；
- deterministic sort + pagination；
- Watch retry；
- same-size rewrite watcher；
- rename immediate snapshot；
- canonical Pi；
- canonical Codex；
- Go-only boundary/deletion contract。

## opencode-analyzer

```bash
cd opencode-analyzer
GOMAXPROCS=2 go test -p 1 ./...
```

必须含 standalone Pi audit canonical fixture。

## release verification

```bash
make build
make release
```

检查：

- `dist/token-analyzer`
- platform artifacts；
- binary 不要求 Node；
- binary 不要求 `opencode-analyzer/` 存在。

---

# 3. 每个 work package 的 review gate

每个 R ticket 完成后只做一次 review，避免重复双轴审查。

## Standards gate

- 不新增 parallel stats implementation；
- DB mutation errors 不被吞；
- source-specific semantics 不泄漏进 Query；
- OpenCode 不反向耦合 token-analyzer；
- 不恢复 Node/TS runtime。

## Spec gate

- 对照本计划对应 finding；
- 每个 finding 必须有 regression；
- 不通过“修改文档来迎合 bug”规避已接受 contract；
- 若必须改变 contract，单独提出 ADR decision。

---

# 4. 推荐 commit 切分

建议最多 5 个逻辑 commit，与 work package 对齐：

```text
fix(pi): make ledger sync atomic and replace finalized requests
fix(query): restore rollup contract and strict source scoping
fix(runtime): make watch and rename snapshot-consistent
fix(opencode): preserve standalone Pi audit semantics
chore(contract): close Go-only migration tracker and release naming
```

不要把所有修复 squash 成一个超大 commit；这五个边界正好与回归范围一致。

---

# 5. 不做事项

本修复计划不包括：

- 恢复 TypeScript backend；
- 把 OpenCode 再并回 token-analyzer；
- 把 OpenCode 正式拆到新 repo；
- 前端视觉重做；
- 新增 source；
- 新模型定价；
- 改写已经接受的 Pi/Codex token semantics；
- 引入网关作为数据源。

---

# 6. 最终 Definition of Done

```text
Pi/Codex source
     ↓
atomic Refresh
     ↓
complete normalized ledger
 raw + supported rollup history
     ↓
pure deterministic Query
     ↓
CLI / API / Watch / WebUI
```

并满足：

- [ ] Pi sync transaction 真正原子。
- [ ] cursor 与 usage 不可能分叉提交。
- [ ] cross-refresh final record replacement 正确。
- [ ] historical rollup 不漏统计。
- [ ] Pi discovery 严格按 layout。
- [ ] Codex home scope path-safe。
- [ ] Query 排序稳定。
- [ ] Watch failure 自动重试。
- [ ] same-size rewrite 可触发 refresh。
- [ ] rename 成功后立即可见。
- [ ] OpenCode audit standalone 且语义不退化。
- [ ] SessionData 不再承载旧 stats engine。
- [ ] 03/04/05 tracker 与真实实现一致。
- [ ] 普通 binary 名称为 token-analyzer。
- [ ] canonical + regression tests 全部通过（在得到测试授权后）。
