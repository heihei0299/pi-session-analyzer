# 03: OpenCode CLI 命令行工具（sync / export）

**What to build:** 在现有主 CLI（`src/cli.ts`）中增加 `opencode` 子命令组，支持开发者通过终端触发 OpenCode 数据同步（`opencode sync`）以及导出本地已存储的历史与用量数据（`opencode export`）。支持从环境变量（`OPENCODE_AUTH`、`OPENCODE_WORKSPACE_ID`、`.env` 文件）读取凭据，或通过 CLI 参数覆盖。

**Blocked by:** 02: OpenCode 本地数据分层持久化与增量同步仓

**Status:** resolved

- [x] 在 `cli.ts` 注册 `opencode sync` 子命令，支持参数 `--auth <cookie>`, `--workspace <id>`, `--full`, `--limit <n>`, `--data-dir <dir>`
- [x] 在 `cli.ts` 注册 `opencode export` 子命令，支持参数 `--format <json|csv>`, `--output <file>`, `--month <YYYY-MM>`
- [x] 实现环境变量及 `.env` 凭证加载器，优先级为：CLI 参数 > 环境变量 / `.env`
- [x] 为 CLI 操作提供结构化、友好的终端控制台输出（显示同步进度、抓取页数、新增条数、错误排查提示）
- [x] 编写 CLI 参数解析与命令执行的单元/集成测试

## 实施总结

- 新增 `src/opencode/credentials.ts` 实现 `resolveCredentials` 与 `loadCredentials`（优先级 CLI > `process.env` > `.env`，手写 `.env` 解析去引号/注释，缺失抛“缺少认证信息，请设置 OPENCODE_AUTH 环境变量或传 --auth”）。
- 新增 `src/opencode/cli.ts` 实现 `runOpencodeSync`（调用 `OpenCodeClient` + `OpenCodeStorage.sync`，支持 `--full`/`--limit`，`--workspace` 缺失时自动 `getWorkspaces()`，进度输出含“同步完成/抓取 Z 页/新增 X 条/耗时 Yms”，401 透传为“凭证过期/cookie 过期”友好提示）与 `runOpencodeExport`（支持 `--format json|csv`、`--output` 文件落盘、`--month YYYY-MM` 前缀过滤，格式/月份校验）。
- 扩展 `src/cli.ts`：`parseArgs` 识别首位 `opencode` 子命令（`sync`/`export`）及各自旗标、未知参数报错；`HELP_TEXT` 新增 OpenCode 段落；`runCli` 分发至 `runOpencodeSync`/`runOpencodeExport`；`validateArgs` 增加 opencode 模式隔离。
- 测试 `test/03-opencode-cli.test.ts` 34 用例覆盖 T1-T5（参数解析、优先级合并、`.env` 读取、sync 401/自动发现/limit 透传、export json/csv/按月/落盘、runCli 集成 + 帮助文本），`typecheck` 通过。

Commit: 4588297 feat(opencode-sync): OpenCode CLI commands (#03) (BASE_HEAD=f7566f1)
