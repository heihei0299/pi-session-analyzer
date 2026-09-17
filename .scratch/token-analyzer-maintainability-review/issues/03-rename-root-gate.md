# 03: 让 Pi session rename 遵守 root binding

**What to build:** 让 Pi session rename 使用与 Refresh、Query、QueryDetail 相同的 root identity 和 binding 规则，避免在发现 root mismatch 后才修改文件。

**Blocked by:** 02: 统一 source root identity 与 legacy ownership 的 fail-closed 策略

**Status:** resolved

- [x] rename 在查找候选 session 文件前验证当前 physical Pi root 与 ledger binding。
- [x] root mismatch、binding 缺失或 root 不可用时不读取候选文件、不执行 rename，并返回明确可处理的错误。
- [x] matching root 下的合法 rename 仍保持原有 active-session、同名文件和文件名安全规则。
- [x] rename 成功后 Refresh 和 QueryDetail 仍能通过原 session identity 读取同一会话。
- [x] rename 失败、Refresh 失败或 snapshot verification 失败时，不伪装成完整成功，并保留可观察错误。
- [x] 回归测试证明错误 root 场景下 Pi 文件内容和文件名均不改变。

## Comments

- Source spec: `.scratch/token-analyzer-maintainability-review/spec.md`
- Triage: ready-for-agent

---

## Completion note

- 修改摘要：rename 在候选文件发现前检查 Pi root binding；mismatch、missing binding 和 unavailable root 均返回明确错误且不触碰文件；保留成功 rename、Refresh 失败和 snapshot verification 失败的可观察结果。
- 验证：`TOKEN_ANALYZER_DB= go test ./internal/server -count=1`，24 个测试通过；相关 root/query/refresh 测试通过。
- Review：完整 Standards/Spec 双轴 Review 已通过；测试覆盖与 staged caller findings 已增量复核关闭。
- Commit：待提交后记录。
- 未解决边界问题：无。
