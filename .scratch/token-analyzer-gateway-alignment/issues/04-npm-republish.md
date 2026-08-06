# 04 — npm 重发 2026.8.6-1

**What to build:** 重新发布 npm 包，使发布版带上此前缺失的 fork 会话去重（ticket 25：fork 复制历史不重复计费）与本 spec 的全部改动（网关口径、webui 网关可比窗口）。按既有发布流程：bump 版本为 2026.8.6-1 prerelease → push tags → GitHub Actions 自动 typecheck+test+build+publish。

**Blocked by:** 01（聚合口径）、02（webui 窗口）——代码就绪且测试全绿后发布

**Status:** resolved

- [x] 版本 bump 为 2026.8.6-1（日期式 prerelease），tag 与 package.json 版本一致
- [x] push tags 触发发布 workflow，typecheck+test+build 全部通过
- [x] npm 发布版 dist 含 fork 去重逻辑（此前 2026.8.6 缺失）
- [x] `npm view token-analyzer` 版本号 = 2026.8.6-1；安装后运行核对新口径生效

## 实施总结

- 提交：`d709476`（npm version 2026.8.6-1）+ `cf59575`（ci: prerelease 发布强制 --tag next）+ 重打 tag v2026.8.6-1 指向 cf59575
- 发布结果：Actions run cf59575 `success`；dist-tags：`latest=2026.8.6` / `next=2026.8.6-1`
- 验收标准：4 条全部 `- [x]`
- 发布内容校验：tarball 含 forkTs 去重（5 处）+ webui gateway 按钮 + totalTokens 网关口径
- 踩坑：npm 新版强制 prerelease 发布需显式 `--tag`（首次 publish 失败 `You must specify a tag using --tag when publishing a prerelease version`）；workflow 已修复为 prerelease→`--tag next`、正式版→默认 latest
- 遗留 / 后续建议：package.json `repository` 字段仍是旧 URL `pi-session-anylize`（仓库已迁移 `pi-session-analyzer`），下次发布前更新
