# 04 — 配置 GitHub Actions 发布凭据（NPM_TOKEN）

Type: task
Status: resolved

## Question

在 GitHub 仓库配置 `NPM_TOKEN` secret，作为 Actions 自动发布到 npmjs.org 的认证凭据（HITL——需用户手动执行）。

## 用户操作清单

1. 登录 npmjs.org → Access Tokens → 生成 **Granular Access Token**（类型：Automation；权限：至少可发布目标包；若包尚未发布，选择覆盖该包名或后续收紧）
2. GitHub 仓库 `heihei0299/pi-session-anylize` → Settings → Secrets and variables → Actions → **New repository secret**：Name = `NPM_TOKEN`，Value = 上一步的 token
3. 完成后回复本 ticket：是否已配置 + token 类型（**不要**贴 token 值）

## 为什么需要

- 05（发布 workflow 规格）与 07（首次真实发布验证）依赖此凭据；未配置时 Actions `npm publish` 会认证失败
- token 只存于 GitHub secret，不写入仓库任何文件

## Answer

### 完成记录（2026-08-06）

- **状态**：用户已手动完成
  - ① npmjs.org Granular Access Token（Automation 类型）已生成
  - ② GitHub repo `heihei0299/pi-session-anylize` → Settings → Secrets and variables → Actions 已配置 `NPM_TOKEN`
- **凭据位置**：仅存于 GitHub repository secret `NPM_TOKEN`（Actions 运行时注入），不写入仓库任何文件；token 值未记录
- **验证方式**：将在 07（首次真实发布）由 Actions `npm publish` 实际验证；本机无 gh CLI，未预先列出 secret
- **影响**：05（workflow 规格）可直接采用 `NPM_TOKEN` 认证；07（首次发布）前置条件已满足
