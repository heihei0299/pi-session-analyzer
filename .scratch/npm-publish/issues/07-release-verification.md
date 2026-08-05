# 07 — 首次真实发布验证

Type: task
Status: resolved
Blocked by: 04, 05, 06

## Question

完成首次真实发布：push 首个 `vX.Y.Z` tag，验证 GitHub Actions 自动发布到 npmjs.org 成功（闭环终点）。

## 工作内容

1. 前置确认：04（NPM_TOKEN 已配）、05（workflow 已落地）、06（可打包）
2. 按 03 约定 bump 版本、打 tag、push（触发 workflow）
3. 观察 Actions run：typecheck + test + build + publish 全绿
4. npmjs 侧验证：`npm view <name>` 版本存在；临时目录 `npm i -g <name>` 运行 `token-analyzer` / `token-analyzer serve` 冒烟
5. 收尾：确认 map destination 达成

## Answer

### 执行记录（2026-08-06）

**发布闭环（run 1→4）**：
- run 1（v2026.8.6@9363b1b）失败：Test——CI(UTC) 时区测试 + serve SIGINT 测试失败
- 修复 78d64eb：test script 固定 `TZ=Asia/Shanghai`（91/91 验证）+ serve SIGINT handler 前置注册 + closeIdleConnections 内聚 server.ts + setup-node pin 24.16.0
- run 2（v2026.8.6@78d64eb）失败：Publish——ENEEDAUTH（NODE_AUTH_TOKEN 在步骤级 env，setup-node 写 .npmrc 时不可见）
- 修复 ee80a54：NODE_AUTH_TOKEN 提升到 job 级 env
- run 3（v2026.8.6@ee80a54）失败：Publish——仍 ENEEDAUTH（根因：secret 未生效；用户重新添加 NPM_TOKEN）
- **run 4（v2026.8.6@ee80a54）成功**：typecheck + test(91) + build + pack + publish 全绿

**run 4 URL**：https://github.com/heihei0299/pi-session-analyzer/actions/runs/31050914740

**npmjs 验证**：
- `npm view token-analyzer`：name/version=`2026.8.6`/bin=`dist/cli.js`/engines=`>=18`/latest=`2026.8.6` ✓
- `npm i -g token-analyzer@2026.8.6` 从 npm 安装 ✓
- `token-analyzer totals`：真实数据正常（11487 请求 / 13.2 亿 token / 8.03 美元）✓
- `token-analyzer serve`：GET / HTTP 200 + /api/totals 真实数据 ✓

**备注**：
- 全局已安装 token-analyzer@2026.8.6（正式版）
- 后续发布流程（03 约定）：`npm version <日期版本>` → `git push --tags` → Actions 自动发布
