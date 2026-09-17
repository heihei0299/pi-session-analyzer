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

### 第二轮（remediation）

- 语义收窄：无 Pi root 的 Codex-only All 仅在 ledger 确认没有 token-analyzer 自有 Pi history 时成立；一旦发现无 binding 的 legacy Pi history，Refresh（`pi.RefreshResolved`）与 Query（`source=all`、piRoot 为空分支）都返回 `db.ErrSourceRootBindingRequired`，不隐藏、不自动认领、不写 binding，历史 rows 不变。
- 已配置但不可用的 Pi root 仍 fail closed；`source=pi`、`source=codex`、有 Pi root 的 `source=all` 行为不变。
- 回归：新增 `TestAllWithoutPiRootRejectsLegacyPiHistory`（legacy history → Refresh/Query 拒绝，pi_sessions 不变、binding 为 0）；既有 `TestAllWithoutPiRootReturnsCodexOnly` 覆盖空 ledger/Codex-only All 成功。
- 文档：README、CONTEXT、ADR-0005 已同步上述边界。
- 静态验证：改动 Go 文件 `gofmt -l` 无输出；`git diff --check` 通过。未执行 `go test`、`go vet`、build（未获授权）。
- 状态：保持 `claimed`；acceptance 未勾选，等待 review 与聚焦测试执行。
