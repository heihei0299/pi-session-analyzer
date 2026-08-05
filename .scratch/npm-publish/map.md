# npm 发布自动化 — Map

**Map id**: `npm-publish` — see `docs/agents/issue-tracker.md` for tracker conventions.

## Destination

token-analyzer 成为**可发布的 npm CLI 包**：`tsc` 编译 dist 产物（JS + `.d.ts`，兼容 Node ≥18、保持零运行时依赖），`serve` 所需的 `webui.html` 随包分发；push `vX.Y.Z` tag 由 GitHub Actions 跑 typecheck + test + build 后自动 `npm publish` 到 **npmjs.org**（`NPM_TOKEN` 认证）。终点 = **首次真实发布成功**：npmjs 上可通过 `npm i -g <包名>` 安装并运行 `token-analyzer` 与 `token-analyzer serve`。

## Notes

- **Domain**: npm 包发布 / GitHub Actions CI / Node CLI 打包
- **技能**: `research`（01/02，AFK）、`grilling`（03/05，HITL）、`task`（04 HITL / 06、07 AFK）
- **已定基线**（用户 grilling 拍板）: ① 打包对象 = 当前项目整体（CLI 三窗口 + serve WebUI）；② 构建方式 = tsc 编译 dist；③ 自动化 = GitHub Actions tag 触发发布；④ registry = npmjs.org
- **用户偏好**: 中文沟通；npm 包管理器（AGENTS.md 铁律，禁 pnpm/yarn）；保持零运行时依赖；Node 24 开发环境；一次只做一个 ticket，不逐步确认
- **会话纪律**: 每 session 最多 resolve 一个 ticket（research 除外）
- **携带执行**: destination 是 in-place change（可发布的包 + 自动发布管道），决策清晰后由 06/07 task ticket 在同一 effort 内完成实现与真实发布验证，不 hand off 给独立 impl effort
- **偏差记录**: 沿用 token-analyzer map 先例——research 发现直接写入 ticket 的 `## Answer` 段落（本仓库无独立 research 分支惯例）

## Decisions so far

<!-- 每解析一个 ticket，在此追加一行：名称 + 链接 + 一句话结论 -->

- [npm 包名可用性研究](issues/01-package-name-availability.md) — `token-analyzer` 在 npmjs 可用（registry 404），直接采用；备选 `@heihei0299/token-analyzer` 等均可用
- [Node 运行时 API 审计](issues/02-node-runtime-audit.md) — src/ 全为老牌 Node API，`engines >=18` 可行；沿用 ES2022/NodeNext，build 加 outDir/declaration；`.ts` 后缀导入由 rewriteRelativeImportExtensions 覆盖
- [版本号与 tag 管理约定](issues/03-release-version-convention.md) — 日期式 semver：首发 `2026.8.6` / tag `v2026.8.6`；`npm version` 命令 bump；workflow tag↔version 不一致即 fail；同日再发用 prerelease 后缀（-1、-2）
- [配置 GitHub Actions 发布凭据](issues/04-npm-token-provisioning.md) — 用户已配 `NPM_TOKEN`（npmjs Automation token → GitHub Actions secret）；凭据仅存于 repo secret，07 发布时实体验证
- [构建与打包配置落地](issues/06-build-and-pack.md) — 可发布包已落地：cli.ts shebang + tsconfig.build（dist+d.ts）+ package.json（name/bin/files/engines>=18/MIT）+ LICENSE + webui.html 随包 + .gitignore dist/；验收全绿（91 测试、pack 20 文件 34.6kB、`i -g` 与 serve 冒烟通过）
- [发布 workflow 规格](issues/05-publish-workflow-spec.md) — 已落地 `.github/workflows/publish.yml`：仅 tags v* 触发、Node 24、tag↔version 不一致 fail、NPM_TOKEN 认证（无 provenance——OIDC 需包已存在）、失败不重试、发布前 pack --dry-run

## Not yet specified

<!-- 尚未 sharp 到可 ticket 化的 fog；frontier 推进后逐步毕业 -->

- 版本迭代节奏（首发后的后续版本沿用同一 tag 机制；本 effort 只覆盖首发闭环）

## Out of scope

- **私有 registry / 多 registry** — 用户明确选 npmjs.org
- **semantic-release / changesets / release-please 等版本工具** — 用户明确选手动 tag 触发
- **pnpm 支持** — 仓库 AGENTS.md 规定 npm 包管理器（pnpm-lock.yaml 为未跟踪遗留，不纳入）
- **Node 运行时二进制打包** — 「将 node 打包」已确认指当前项目，不含把 node 运行时打进包
- **WebUI 拆分为独立包** — 用户明确选整体发布
