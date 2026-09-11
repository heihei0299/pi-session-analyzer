# 03: 源能力声明与出口诚实性（Go、npm/TS 与文档）

**What to build:** 用户在任何一个出口都不会遇到「做不到的能力被假装可用」：WebUI 与 npm 版面板只在后端真的声明了该数据源时才提供源选择；Codex/All 下的请求级窗口、会话详情与重命名给出明确原因而不是 404 或写错目录；npm 版 CLI 收到 Codex 参数时明确报错并指向 Go 版本；README 的安装方式与能力矩阵和实现一致。

**Blocked by:** 01: 真实命名可发现（Codex rollout 发现契约）

**Status:** resolved

- [x] `meta.sources` 成为「后端能提供哪些数据源」的唯一声明源，共享 WebUI 不再自行假设；未声明的源不出现在源选择器里。
- [x] Codex/All 下请求级窗口、会话详情、重命名入口统一禁用，并给出可读原因（例如「Codex/All 不支持 requests 查询」）。
- [x] 服务端对 Codex/All 的会话详情与重命名返回明确的 unsupported 错误（不作为 404，也绝不写入 Pi 目录）；Pi 下的详情与重命名行为不变。
- [x] TS 后端不再静默忽略 `source`：未声明能力时面板不提供该源；TS CLI 收到 `--source` / `--codex-dir` 时明确报错并提示改用 Go 版本，而不是静默忽略或返回错源数据。
- [x] README 明确两种安装方式的能力差异（哪些能力仅 Go 版本可用），并与 `/api/meta` 的声明一致。
- [x] 服务端集成测试覆盖：source 切换、requests 拒绝、Codex/All 的详情与重命名 unsupported、`meta.sources` 声明、警告透传、空目录不返回服务错误。
- [x] Pi 默认路径（`--source pi`、默认目录、WebUI 默认视图）无行为回归。

## Implementation summary

- `internal/query` 新增 `SupportedSources()`：Go 后端声明 `["pi","codex"]`，所有带 `meta` 的查询出口统一覆写为后端能力声明，不再从本次参与文件反推源。
- 共享 WebUI 按 `/api/meta.sources` 渲染源选择器；Codex/All 下 requests tab、详情入口、会话管理重命名和抽屉标题重命名统一禁用并给出可读原因。切换到非 Pi 时会离开 requests tab 并关闭已打开抽屉。
- Go server：详情与重命名对 Codex/All 返回 400 `Unsupported`（不返回 404、不写 Pi 目录）；Pi 路径保持原行为。WebUI 详情/重命名请求显式携带 `source=pi`，因此即使 `serve --source codex` 启动，选择 Pi 后详情/重命名仍正常工作。
- TS/npm 后端：`/api/meta` 返回 `sources:["pi"]`；所有数据端点对未声明的 `source=codex/all` 返回 400 `Unsupported` 并提示改用 Go 版本；详情、重命名先校验 source，避免落成 404 或误写 Pi 目录。
- TS CLI：`parseArgs` 对 `--source` / `--codex-dir`（含 `--flag=value` 形态，含 `serve` 模式）抛出明确错误并提示 Go 版本；帮助文本补充 npm/TS 仅 Pi。
- README 增加安装方式能力矩阵，并更新用法、数据源、Web 面板与 HTTP API 说明，使文档与 `/api/meta.sources`、实际拒绝行为一致。

## Comments

- 2026-09-11 实现与验证：
  - `go test ./...` 全绿（在设置临时 `TOKEN_ANALYZER_DB` 隔离默认库后；含新增 `TestServerSourceSwitchingCapabilitiesAndEmptyDir`：source 切换、requests 拒绝、meta.sources、空 Codex 目录警告；既有 `TestServerRejectsDetailAndRenameForNonPiSources` 覆盖详情/重命名 unsupported 与不写 Pi 目录，并新增 `--source codex` + 显式 `source=pi` 的详情回归）。
  - `go vet ./...` clean。
  - `npm run typecheck` clean；`npm test` 318/318 通过（含新增 `test/39-source-capability-honesty.test.ts`：TS meta.sources、source 拒绝、rename 不写盘、CLI 参数拒绝）。
  - `make sync-webui` 已同步 `src/webui.html` 与 Go embed 副本；`internal/server/webui_sync_test.go` 验证二者一致。
  - Pi 回归：既有 Go/TS 测试全绿；默认 `source=pi`、默认目录、WebUI 默认视图与 details/rename 行为未改变。
