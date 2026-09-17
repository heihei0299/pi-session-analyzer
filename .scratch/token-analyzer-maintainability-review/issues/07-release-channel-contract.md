# 07: 统一 release tag 与 GitHub Release channel

**What to build:** 让 release tag、CLI version、构建产物和 GitHub Release channel 遵守同一版本契约，稳定版与候选版不会互相误标。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] 合法的日期式 stable tag 发布为 stable GitHub Release。
- [x] 合法的日期式 `-N` suffix tag 发布为 prerelease GitHub Release。
- [x] 非法年份、月份、日期和格式仍在构建前被拒绝。
- [x] Makefile、CLI version、artifact smoke check 和 release tag 继续使用同一个权威版本值。
- [x] release workflow 只验证和发布当前仓库拥有的 token-analyzer 产物，不重新引入已迁出的 OpenCode delivery dependency。
- [x] 回归检查覆盖 stable tag、prerelease tag、非法 tag 和 `--version` 一致性。
- [x] 不引入新的 release tool、runtime dependency 或独立版本常量。

## Comments

- Source spec: `.scratch/token-analyzer-maintainability-review/spec.md`
- Triage: ready-for-agent

---

## Completion note

- 修改摘要：release workflow 使用真实日历校验日期式 tag，统一输出 tag 派生 version，并将 numeric suffix 派生为 GitHub prerelease；stable tag 保持正式 Release，产物只来自 root token-analyzer。
- 验证：workflow tag shell block 通过 `bash -n`；stable/prerelease/非法日期样例已纳入 workflow；未执行本机 build。
- Review：完整 Standards/Spec 双轴 Review 已通过；workflow 实现细节测试 finding 已增量复核关闭。
- Commit：`16bb41b ci(release): align tag channels and versions`。
- 未解决边界问题：GitHub Actions 真实发布由 CI/release runner 执行。
