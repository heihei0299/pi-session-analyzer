# 06 — 构建与打包配置落地

Type: task
Status: resolved
Blocked by: 01, 02

## Question

落地 tsc 构建与 npm 打包配置，产出可 `npm pack` 的发布包（carrying execution——本 effort 直接实现）。

## 工作内容

1. **tsconfig.build.json**（或改造现有 tsconfig）：`outDir: dist`、`declaration: true`、`rootDir: src`，target / module 按 02 结论
2. **package.json**：`name`（按 01）、`bin.token-analyzer → dist/cli.js`、`files` 白名单（dist/ + README + LICENSE）、`engines`（按 02）、`license`、`repository`、`keywords`、`scripts.build`
3. **webui.html 随包分发**：构建时复制进 dist（tsc 不复制非 .ts 文件）；`server.ts` 运行时按 `import.meta.url` 相对定位 dist 内资源（发布后 `serve` 必须可用）
4. **版本与发布脚本**：按 03 约定的 bump 流程（`npm version` 等）
5. **验收**：`npm run build && npm pack --dry-run` 产物完整；`node dist/cli.js` 正常；`npm i -g` 后 `token-analyzer` 与 `token-analyzer serve` 冒烟通过；`npm test` + `npm run typecheck` 仍绿

## Answer

### 执行记录（2026-08-06）

**实现产物**：
- `src/cli.ts`：首行新增 `#!/usr/bin/env node`（bin 可执行必需；tsc 保留首行 shebang）
- `tsconfig.build.json`：extends 根配置 + `noEmit: false` / `outDir: dist` / `rootDir: src` / `declaration: true`；include 仅 src（test 不编译）
- `package.json`：`name=token-analyzer`（01）、`bin → dist/cli.js`、`files: ["dist"]`、`engines.node: ">=18"`（02）、license MIT、repository、keywords；scripts 增 `build`（tsc + cp webui.html）与 `prepack`（发布前自动 build）；移除 `private: true`
- `LICENSE`：MIT（Copyright 2026 heihei0299）
- `.gitignore`：追加 `dist/`（构建产物不入库）

**验收证据**：
1. `npm run typecheck` — 绿
2. `npm run build` — dist 含 8 js + 8 d.ts + webui.html（17 文件）
3. `node dist/cli.js totals --dir <空目录>` — 正常输出 0 数据表格
4. `npm pack` — 20 文件（dist 17 + README/LICENSE/package.json），34.6 kB
5. `npm test` — 91/91 通过
6. `npm i -g ./token-analyzer-0.1.0.tgz` → `token-analyzer totals` 正常；`token-analyzer serve` → HTTP 200，`/api/totals` 返回真实数据（webui.html 资源定位正确）

**备注**：
- version 保持 0.1.0 未动（03 约定由 `npm version` 维护，07 时 bump 为 2026.8.6）
- 全局已安装 token-analyzer@0.1.0（冒烟用）；07 发布后 `npm i -g token-analyzer@2026.8.6` 可更新
