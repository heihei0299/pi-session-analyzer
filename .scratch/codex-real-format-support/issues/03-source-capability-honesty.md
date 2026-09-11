# 03: 源能力声明与出口诚实性（Go、npm/TS 与文档）

**What to build:** 用户在任何一个出口都不会遇到「做不到的能力被假装可用」：WebUI 与 npm 版面板只在后端真的声明了该数据源时才提供源选择；Codex/All 下的请求级窗口、会话详情与重命名给出明确原因而不是 404 或写错目录；npm 版 CLI 收到 Codex 参数时明确报错并指向 Go 版本；README 的安装方式与能力矩阵和实现一致。

**Blocked by:** 01: 真实命名可发现（Codex rollout 发现契约）

**Status:** ready-for-agent

- [ ] `meta.sources` 成为「后端能提供哪些数据源」的唯一声明源，共享 WebUI 不再自行假设；未声明的源不出现在源选择器里。
- [ ] Codex/All 下请求级窗口、会话详情、重命名入口统一禁用，并给出可读原因（例如「Codex/All 不支持 requests 查询」）。
- [ ] 服务端对 Codex/All 的会话详情与重命名返回明确的 unsupported 错误（不作为 404，也绝不写入 Pi 目录）；Pi 下的详情与重命名行为不变。
- [ ] TS 后端不再静默忽略 `source`：未声明能力时面板不提供该源；TS CLI 收到 `--source` / `--codex-dir` 时明确报错并提示改用 Go 版本，而不是静默忽略或返回错源数据。
- [ ] README 明确两种安装方式的能力差异（哪些能力仅 Go 版本可用），并与 `/api/meta` 的声明一致。
- [ ] 服务端集成测试覆盖：source 切换、requests 拒绝、Codex/All 的详情与重命名 unsupported、`meta.sources` 声明、警告透传、空目录不返回服务错误。
- [ ] Pi 默认路径（`--source pi`、默认目录、WebUI 默认视图）无行为回归。
