# 11: 持久化 Pi mutation 与 commit failure diagnostics

**What to build:** 让 Pi 文件在 ledger mutation 或事务 commit 失败时，既保留旧 cursor/有效 usage，又把失败诊断持久化到可被后续 Query、CLI 和 HTTP 重放的状态。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] 事务内 mutation 失败会 rollback，不推进该文件 cursor，不删除已有有效 ledger rows。
- [x] rollback 后失败 diagnostics 通过独立安全路径持久化，且不会伪装成成功 revision。
- [x] commit 失败也会持久化 source-tagged diagnostics，并保留旧 cursor 供下一次 Refresh 重试。
- [x] 下次 Refresh 能成功重试同一文件，成功后不重复记账。
- [x] QueryMeta、CLI 输出和 HTTP/WebUI 状态能够观察到 mutation/commit failure。
- [x] 与 malformed record、incremental diagnostics、cursor-hit replay 的既有行为兼容。
- [x] 回归测试覆盖 mutation failure、commit failure、retry、旧 cursor 和有效 totals 保留。

## Comments

- Source spec: `.scratch/token-analyzer-maintainability-review/spec.md`
- Review finding: `915d501` Pi mutation/commit diagnostics not persisted
- Triage: ready-for-agent

---

## Completion note

### 第二轮（remediation）

- commit failure diagnostics 改用独立 ledger 连接持久化：新增 `persistCommitFailureDiagnostics`（`db.Open(database.Path)` 独立连接，失败回退同一路径但仅写 summary），commit 失败路径不再复用可能损坏的事务连接；写入仍为 summary-only（只在无行时插入零 cursor，`ON CONFLICT` 只更新 `diagnostics_summary`），不推进 cursor、不覆盖有效 usage。
- commit 结果不确定的契约：失败路径不写 cursor；下一次 Refresh 重新读取实际持久化的 cursor，重试依靠 `session_usage_dedup` / request_id / semantic_id 去重，不确定 commit 不会重复记账（新增 retry/no-duplicate 回归）。
- mutation rollback 路径保持原有独立语义（rollback + summary-only persist），未改行为。
- 回归：新增 `TestPiCommitFailurePersistsDiagnosticsThroughIndependentConnection`（注入 commit failure → `LoadDiagnostics` 可重放、旧 rows/cursor 保留、retry 只导入一次、再次 retry 幂等且诊断保留）；既有 `TestPiMutationFailurePersistsDiagnosticsAndRetries` 覆盖 mutation rollback。
- 静态验证：改动 Go 文件 `gofmt -l` 无输出；`git diff --check` 通过。未执行 `go test`、`go vet`、build（未获授权）；行为断言均为待执行回归。
- 状态：保持 `claimed`；acceptance 未勾选，等待 review 与聚焦测试执行。

### 第三轮（review pass）

- 测试暴露并修复的两个真实失败（同在 09-11 范围内，未削弱断言规避）：
  - 增量路径丢失 stored diagnostics（`39b4bc3` 回归）：恢复把已持久化诊断并入返回结果，同时 `fileDiagnostics` 仅保留本次新增以避免失败持久化时重复叠加；
  - `TestPiSyncRollsBackUsageFailureAndRetries` 旧断言与 issue 11 契约冲突：改为断言 usage/去重/session 零行 + 仅一行零 cursor（`last_byte_offset=0`、`sync_semantics_version=0`）的 diagnostics-only 行。
- 验证（本机已获授权，w9:p1 review 独立复跑一致）：
  - `test -z "$(gofmt -l $(git ls-files '*.go'))"` → exit 0；
  - `GOMAXPROCS=2 go vet ./...` → exit 0；
  - `TOKEN_ANALYZER_DB= GOMAXPROCS=2 go test -p 1 -parallel 1 ./...` → 154 passed in 12 packages，exit 0。
- 第 5 条（可观察性）的验证层级：`LoadDiagnostics` → `TestPiDiagnosticsAreVisibleThroughQueryMeta` 与新增回归都在该层断言；CLI/HTTP/WebUI 复用同一 meta 警告，由全量 server/query 用例覆盖，未另写渲染层断言。
- Commit：`4b38290`（独立连接持久化 commit failure）、`1f51c11`（incremental diagnostics 回归修复 + 旧断言对齐契约）；`9135c81` 为无关基线格式化。
- Review：REVIEW_ROUND_4 REVIEW PASS，无剩余阻塞。
- 未执行（未授权）：`make release`、交叉编译、打包、安装。
