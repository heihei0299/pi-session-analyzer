# 09: 固定 root identity，消除 Refresh 与 rename 的 TOCTOU

**What to build:** 让 Refresh 和 Pi session rename 在通过 root binding 校验后，始终对同一个 canonical physical root 执行后续读取、枚举和 mutation，不能因 symlink 在校验后变化而跨 root。

**Blocked by:** 02: 统一 source root identity 与 legacy ownership 的 fail-closed 策略；03: 让 Pi session rename 遵守 root binding（均已完成，作为既有契约）

**Status:** claimed

- [ ] root identity 校验和后续 source file 操作使用同一个固定的 canonical physical root。
- [ ] 校验后替换 symlink target 不会导致 Refresh 从未绑定目录导入数据。
- [ ] 校验后替换 symlink target 不会导致 rename 查找或修改未绑定目录中的文件。
- [ ] matching root 的正常 Refresh、Query 和 rename 行为保持不变。
- [ ] mismatch、root unavailable 和并发 symlink 变化都 fail closed，不删除已有 ledger rows。
- [ ] 回归测试覆盖 Refresh 与 rename 的 root identity race/替换场景；若平台限制无法稳定制造 race，至少覆盖校验后 canonical root 不再重新解析的行为契约。

## Comments

- Source spec: `.scratch/token-analyzer-maintainability-review/spec.md`
- Review finding: `d519459` / `1d617b1` TOCTOU
- Triage: ready-for-agent

---

## Completion note

- 修改摘要：新增 canonical physical root 返回值；Refresh 在打开 ledger 前固定 canonical root，Query/QueryDetail/rename 在后续读取或 mutation 中复用 canonical path；增量 diagnostics path 也按 physical containment 判断。
- 回归证据：新增 symlink target switch 后 RefreshResolved 与 rename gate 测试，覆盖校验后不重新解析词法 symlink 的契约；测试未执行。
- 静态验证：Go 文件已 `gofmt`，`git diff --check` 通过，canonical root 调用链已检索。
- 状态：保持 `claimed`，聚焦 Go 测试与 review 待执行；未解决阻塞为本轮 HANDOFF 未授权编译/测试。
