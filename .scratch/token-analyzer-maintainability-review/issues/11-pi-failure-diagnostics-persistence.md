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

- 修改摘要：Pi mutation rollback 和 commit failure 均在事务结束后通过 summary-only persistence 记录 source-tagged diagnostics，不推进 cursor；retry 继续复用旧 cursor/有效 usage 并保持幂等。
- 回归证据：新增 mutation failure → diagnostics persistence → retry/no-duplicate 测试；既有 cursor/retry 测试保持适用；测试未执行。
- 静态验证：Go 文件已 `gofmt`，`git diff --check` 通过，rollback/commit/persist 调用链已检索。
- 状态：保持 `claimed`，聚焦 Go 测试与 review 待执行；未解决阻塞为本轮 HANDOFF 未授权编译/测试。
