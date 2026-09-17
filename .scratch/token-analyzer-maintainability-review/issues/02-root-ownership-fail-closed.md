# 02: 统一 source root identity 与 legacy ownership 的 fail-closed 策略

**What to build:** 让一个 normalized ledger 只服务于明确绑定的 Pi physical root，并在新 ledger、legacy ledger、共享 ledger、非法 root 和 symlink root 场景下保持数据隔离。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [ ] 空 ledger 可以首次绑定存在且可解析的 Pi root。
- [ ] 共享 ledger 中不属于 token-analyzer Pi session 的 proxy 行不会阻止首次 binding。
- [ ] 已有 Pi 历史但没有 root binding 的 legacy ledger 不会被当前 root 自动认领。
- [ ] legacy fail-closed 过程不删除、不改写已有历史行。
- [ ] root 不存在、不是目录或 symlink target 无法解析时，在创建或迁移 ledger 前返回明确错误。
- [ ] 同一 physical root 的词法路径变化仍被识别为同一 root；切换到不同 physical target 时 Refresh 和 Query 都拒绝。
- [ ] 现有 schema version、Refresh 失败保留旧 snapshot 和 Pi/Codex ownership 契约不被改变。
- [ ] 回归测试覆盖 fresh ledger、shared rows、legacy history、matching root、mismatch root 和 symlink target switch。

## Comments

- Source spec: `.scratch/token-analyzer-maintainability-review/spec.md`
- Triage: ready-for-agent

---

## Completion note

- 修改摘要：统一 Pi root ownership predicate；共享 ledger 中非 `pi_session` proxy 行不再阻塞首次 binding；Refresh 在打开 ledger 前校验有效 root；同一 physical root 继续支持词法路径变化，不同 target 与 legacy history 继续 fail closed。
- 验证：`TOKEN_ANALYZER_DB= go test ./internal/pi ./internal/db ./internal/refresh ./internal/query`，通过。
- Review：完整 Standards/Spec 双轴 Review 已通过；empty-root、解析重复和测试覆盖 findings 均已增量复核关闭。
- Commit：`d519459 fix(refresh): fail closed on Pi root ownership`。
- 未解决边界问题：`source=all` 且未配置 Pi root 继续保留既有 Codex-only 行为。
