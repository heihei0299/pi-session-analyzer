# 11: 持久化 Pi mutation 与 commit failure diagnostics

**What to build:** 让 Pi 文件在 ledger mutation 或事务 commit 失败时，既保留旧 cursor/有效 usage，又把失败诊断持久化到可被后续 Query、CLI 和 HTTP 重放的状态。

**Blocked by:** None (can start immediately)

**Status:** claimed

- [ ] 事务内 mutation 失败会 rollback，不推进该文件 cursor，不删除已有有效 ledger rows。
- [ ] rollback 后失败 diagnostics 通过独立安全路径持久化，且不会伪装成成功 revision。
- [ ] commit 失败也会持久化 source-tagged diagnostics，并保留旧 cursor 供下一次 Refresh 重试。
- [ ] 下次 Refresh 能成功重试同一文件，成功后不重复记账。
- [ ] QueryMeta、CLI 输出和 HTTP/WebUI 状态能够观察到 mutation/commit failure。
- [ ] 与 malformed record、incremental diagnostics、cursor-hit replay 的既有行为兼容。
- [ ] 回归测试覆盖 mutation failure、commit failure、retry、旧 cursor 和有效 totals 保留。

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
