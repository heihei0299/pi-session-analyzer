# 03 — 版本号与 tag 管理约定

Type: grilling
Status: resolved

## Question

版本号与 tag 采用什么管理约定？（workflow 校验 + bump 流程 + 首发版本号）

具体子问题：
1. **一致性校验**：push 的 tag `vX.Y.Z` 与 `package.json.version` 不一致时，workflow 应 fail 还是自动同步？
2. **bump 流程**：本地 `npm version <patch|minor>`（自动改 version + 打 tag）还是手动改 package.json + `git tag`？
3. **首发版本号**：当前 `0.1.0`（private）——直接以 0.1.0 首发，还是升 0.2.0（已有多次功能迭代）或直接 1.0.0？
4. **semver 纪律**：0.x 阶段 breaking 是否允许 minor bump（npm 惯例）？后续迭代节奏？

## Answer

### 结论摘要（grilling 逐问确认，2026-08-06）

1. **首发版本号**：日期式 semver——`version = 2026.8.6`，`tag = v2026.8.6`（用户指定 `v20260806` 的合法落地；npm `version` 字段必须为 `X.Y.Z`）
2. **bump 流程**：`npm version 2026.8.6`（自动改 package.json + commit + 打 tag `v2026.8.6`），随后 `git push && git push --tags`——tag 与 version 天然一致
3. **一致性校验**：workflow 从 tag 去 `v` 前缀提取期望版本，与 `package.json.version` 比对，**不一致即 fail**（自动同步与不校验均否决）
4. **迭代节奏**：常规发布用当天日期（`YYYY.M.D`）；同日再次发布用 prerelease 后缀（`2026.8.6-1`、`-2`）；无 breaking 语义限制

### 对后续 ticket 的输入

- **05（发布 workflow 规格）**：解锁——workflow 需含「tag 去 `v` 前缀 vs `package.json.version` 比对、不一致 fail」步骤
- **07（首次真实发布验证）**：push `v2026.8.6` 触发
- **06（构建打包）**：version 由 `npm version` 命令维护，无需手动改
