# 10: 让无 Pi root 的 All source 保持一致可用

**What to build:** 明确定义并实现 `source=all` 在没有可用 Pi root 时的行为，使 Codex-only 数据不会在 Refresh 成功后被 Query 的 Pi binding 校验阻断。

**Blocked by:** 02: 统一 source root identity 与 legacy ownership 的 fail-closed 策略（已完成，作为既有 root policy）

**Status:** claimed

- [ ] `source=all`、没有 Pi root 但 Codex root 有效时，Refresh 和 Query 遵守同一策略。
- [ ] 若产品策略允许 Codex-only All，Codex totals/sessions/groups/period/meta 可以正常查询，Pi 部分为空且不伪造 binding。
- [ ] 若产品策略要求 All 必须包含 Pi，则 Refresh 在写 ledger 前明确拒绝，Query 不留下半可用状态；文档和 capability 声明同步更新。
- [ ] `source=pi`、`source=codex` 和有 Pi root 的 `source=all` 行为保持不变。
- [ ] 回归测试覆盖无 Pi root 的 All、Codex-only 查询、空 Pi 数据和 root error contract。
- [ ] 不通过跳过安全校验来接受不明 root；legacy Pi history 继续 fail closed。

## Comments

- Source spec: `.scratch/token-analyzer-maintainability-review/spec.md`
- Review finding: `d519459` All source without Pi root
- Triage: ready-for-agent

---

## Completion note

- 产品语义：选择允许 Codex-only All；无 Pi root 时不建立 Pi binding、不读取 Pi history，Codex totals/sessions/groups/period/meta 正常可查，Pi 部分为空；已配置但不可用的 Pi root 仍 fail closed。
- 修改摘要：Refresh 与 Query 对无 Pi root 使用同一策略，QueryAll 跳过 Pi ledger/rollup/diagnostics 读取；新增 Codex-only All 回归和 All invalid-root 回归。
- 静态验证：Go 文件已 `gofmt`，`git diff --check` 通过，All/Query/Refresh 分支已检索；测试未执行。
- 状态：保持 `claimed`，聚焦 Go 测试与 review 待执行；未解决阻塞为本轮 HANDOFF 未授权编译/测试。
