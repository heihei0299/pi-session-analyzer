# 05 — GitHub Actions 发布 workflow 规格

Type: grilling
Status: resolved
Blocked by: 03

## Question

发布 workflow（`.github/workflows/publish.yml`）的规格是什么？

具体子问题：
1. **触发**：`push: tags: ['v*']`（+ 是否加 `workflow_dispatch` 手动触发）
2. **步骤链**：checkout → setup-node（Node 版本 24？是否矩阵）→ `npm ci` → typecheck + test → build（tsc）→ `npm publish`（认证：`NPM_TOKEN` secret → `.npmrc`；是否 `--provenance`——个人账号 + token 认证下 npm 不支持 provenance，需确认）
3. **版本一致性校验**：tag 与 package.json.version 比对（按 03 决策：fail 或同步）
4. **失败与重试**：测试失败即中止；publish 失败是否重试（幂等性：版本已发布则 skip 还是报错）
5. **产物一致性**：发布前 `npm pack --dry-run` 校验 files 白名单内容

## Answer

### 结论摘要（grilling 逐问确认，2026-08-06）

**workflow 规格**（已落地为 `.github/workflows/publish.yml`）：

1. **触发**：仅 `push: tags: ['v*']`（无 workflow_dispatch；失败恢复走新 tag / 同日 prerelease）
2. **步骤链**（单 job，ubuntu-latest，timeout 15min）：checkout → setup-node **24**（registry-url npmjs）→ **版本一致性校验**（`GITHUB_REF_NAME` 去 `v` 前缀 vs `package.json.version`，不一致 fail——03 决策）→ `npm ci` → `npm run typecheck` → `npm test` → `npm run build` → `npm pack --dry-run` 产物校验 → `npm publish`（`NODE_AUTH_TOKEN=${{ secrets.NPM_TOKEN }}`）
3. **provenance**：不启用——npm OIDC 发布要求包已存在 + npmjs trusted publisher 配置，且用户已定 NPM_TOKEN 认证；`permissions` 最小化（contents: read，无 id-token: write）
4. **失败策略**：不重试，任一步 fail 即中止（用户确认）
5. **产物一致性**：发布前 `npm pack --dry-run` 校验 files 白名单内容

### 备注（后续可选，非本 effort 范围）

- 首发成功后可在 npmjs 配置 trusted publisher，将后续发布迁移到 OIDC（免长期 token）
- 07 首次发布前需确认：`package-lock.json` 与 `package.json` 一致性（`npm ci` 会校验）；`npm version` 需 clean working tree
