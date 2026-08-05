# 07 — 首次真实发布验证

Type: task
Status: claimed
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

（执行后填：workflow run URL + npm view 输出 + 冒烟结果）
