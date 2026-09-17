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

### 第二轮（remediation）

- 修改摘要：根 identity 固定为 pinned canonical physical root：新增 `BindPinnedSourceRoot` / `CheckPinnedSourceRoot`，不重解析，要求 root 自身就是已解析的绝对物理路径；RefreshResolved、Query、QueryDetail、rename 全部改用 pinned 版本，不再在绑定阶段重新 canonicalize 后丢弃 identity。
- 候选实体校验：`CollectPiJsonlFiles` 跳过 `.jsonl` 与 project symlink；新增 `VerifyPinnedSessionFile` / `VerifyPinnedTarget`（拒绝 symlink file/project，并校验物理 containment），RefreshResolved 在同步前逐个校验候选，rename 在 lookup 后、Stat 前、Rename 前分别校验。
- Refresh/Query 与 rename 的 root/project/.jsonl symlink target 替换、校验后不重解析的回归测试已补：`TestRefreshResolvedSkipsSymlinkProjectAndFileEscape`、`TestServerRenameRefusesSymlinkFileEscape`；既有 `TestRefreshResolvedKeepsCanonicalPhysicalRootAfterSymlinkSwap`、`TestServerRenameRejectsSymlinkTargetSwitchBeforeFilesystemRename`、`TestSourceRootBinding*` 保持适用。
- 静态验证：改动 Go 文件 `gofmt -l` 无输出（已格式化）；`git diff --check` 通过。本机未执行 `go test`、`go vet`、build 或全量测试（未获授权），所有行为断言仅为待执行回归。
- 状态：保持 `claimed`；acceptance 未勾选，等待 review 与聚焦测试执行。
