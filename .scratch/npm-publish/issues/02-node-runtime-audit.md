# 02 — Node 运行时 API 审计

Type: research
Status: resolved

## Question

src/ 运行时用到的 Node API 与语法特性要求的最低 Node 版本是多少？tsc 的 target / module 该设什么？

具体子问题：
1. 逐一审计 `src/*.ts`（cli.ts / analyze.ts / aggregate.ts / render.ts / serialize.ts / watch.ts / server.ts / api.ts）用到的 Node 内置模块 API（fs / path / http / os / process / stream / util / crypto 等）与 ES 语法特性
2. 对照 Node 官方文档的版本支持表，找出**最高门槛 API**（引入该 API 的最低 Node 版本）→ 决定 `engines.node` 下限
3. 检查是否有 Node 24-only / 实验性特性（type-stripping 依赖、`--experimental-*`、需 flag 的 API）——注意测试代码（`node:test` 跑 `.ts`）不属于运行时约束，`engines` 只看 src/ 运行时
4. 产出：`engines.node` 建议值、tsc `target` / `module` 建议、潜在不兼容点清单

## 调研指引

- 本地源码审计：src/ 全部文件（运行时）；test/ 仅参考，不设 engines 约束
- 权威来源：Node 官方文档（nodejs.org/api）各 API 的版本历史；tsc 文档（target 与 ES 版本对应）
- 目标：兼容 Node ≥18 的普通消费者（用户已拍板 tsc 编译 dist）
- 输出格式：API 清单（API → 最低 Node 版本）→ 最高门槛 → engines 建议

## Answer

### 结论摘要

- **`engines.node` 建议 `">=18"`**：src/ 运行时全部为 Node 老牌 API，最高门槛仅 `import.meta.url`（Node 12.20+）与 `pathToFileURL`（Node 10.12），远低于 18——用户拍板的 Node ≥18 目标完全可行
- **tsc 沿用现有配置**：`target: ES2022` / `module: NodeNext` / `moduleResolution: NodeNext`（import.meta 属 ES2020，已覆盖）；build 变体仅需在现有基础上加 `outDir: dist`、`declaration: true`、`rootDir: src`、`noEmit: false`
- **`.ts` 后缀相对导入已被覆盖**：`server.ts:9` 的 `from "./api.ts"` 等，由现有 `allowImportingTsExtensions: true` + `rewriteRelativeImportExtensions: true`（TS 5.7+）编译时自动重写为 `.js`，发布 JS 产物无影响
- **无 Node 24-only / 实验性 API**：type-stripping 仅影响开发环境直接跑 `.ts`，不属于消费者约束；`node:test` 仅测试用，不设 engines
- **webui.html 定位**：`server.ts:11` `new URL("./webui.html", import.meta.url)`——编译后 `dist/server.js` 与 `dist/webui.html` 同级即正确；build 脚本需复制该资源

### 审计清单（API → 最低 Node）

| API | 模块 | 最低 Node | 使用文件 |
|-----|------|-----------|----------|
| readdirSync / createReadStream / realpathSync / statSync / readFileSync / openSync / readSync / closeSync / existsSync / renameSync | node:fs | ≤6 | analyze / api / server / watch / cli |
| join / resolve / basename / dirname | node:path | ≤6 | analyze / api / cli |
| createInterface | node:readline | 0.10 | analyze / api / watch |
| createServer（含类型） | node:http | ≤6 | server |
| homedir | node:os | ≤6 | cli |
| pathToFileURL | node:url | 10.12 | cli |
| import.meta.url（ES2020 语法） | — | 12.20 | cli / server |

### 对 06 的落地输入

- `package.json.engines = { "node": ">=18" }`
- `tsconfig.build.json`：继承现有 + `outDir: dist` / `declaration: true` / `rootDir: src` / `noEmit: false`
- build 脚本：`tsc -p tsconfig.build.json && cp src/webui.html dist/`
