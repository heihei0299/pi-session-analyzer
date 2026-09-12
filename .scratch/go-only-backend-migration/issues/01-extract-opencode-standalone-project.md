# 01: 将 OpenCode 抽离为 standalone opencode-analyzer 项目

**What to build:** 把当前嵌在 token-analyzer 内的 OpenCode 对账能力完整迁入仓库根下独立 `opencode-analyzer/` 项目目录。现有 OpenCode 功能要保留，但它必须拥有自己的运行、依赖、存储、API/UI、测试和文档边界；未来把该目录整体迁到新仓库并独立上线时，不需要再重构 token-analyzer。

**Blocked by:** None (can start immediately).

**Status:** resolved

- [x] 仓库根下建立独立 `opencode-analyzer/` 项目目录；当前 token-analyzer 根目录结构不整体搬迁，避免无意义的大规模路径改动。
- [x] 现有 OpenCode RPC / Seroval / workspace discovery / credential-cookie / sync / pagination 能力迁入 `opencode-analyzer/`，而不是删除。
- [x] OpenCode 本地 storage / cursor / lock / export / audit 能力迁入 `opencode-analyzer/` 并拥有自己的数据目录和配置边界。
- [x] OpenCode 专用 CLI、HTTP API 与 WebUI audit 能力由 `opencode-analyzer/` 自己提供；token-analyzer 不再暴露这些命令、路由或页面。
- [x] `opencode-analyzer/` 拥有自己的 README、入口、依赖声明、测试入口和运行说明，能够被视为一个独立项目目录。
- [x] `opencode-analyzer/` 不 import 或依赖 token-analyzer 的 `internal/*`；不得把 token-analyzer ledger/query/server 当作运行前提。
- [x] token-analyzer 不反向 import、启动或代理 `opencode-analyzer/`；两者同仓仅是代码托管关系。
- [x] token-analyzer 不读取/保存 OpenCode cookie 或 credential，不提供 `source=opencode`，OpenCode 不进入 `source=all`、normalized ledger 或 `meta.sources`。
- [x] OpenCode 现有用户可见能力在迁入独立目录后保持可用；本 ticket 不要求把它重写为 Go，也不要求与 token-analyzer 使用相同技术栈。
- [x] 将 `opencode-analyzer/` 整个目录移出仓库后，token-analyzer 的 Pi/Codex canonical contract 不发生变化。
- [x] 反向验证：`opencode-analyzer/` 的自身运行与测试不需要 token-analyzer runtime 或私有源码。
- [x] 独立 Git 仓库、正式 package/release、域名、CI/CD 和线上部署不属于本 ticket，留给未来 OpenCode 项目上线阶段。

## Answer

- 新建独立 Go module `opencode-analyzer/`，提供自己的 CLI、RPC/Seroval client、credential loader、增量 storage/cursor/lock/export、Pi audit、HTTP API、内嵌 WebUI、README、LICENSE 与测试。
- 从 token-analyzer 的 Go/TypeScript CLI、HTTP API、WebUI、CI smoke command 中移除 OpenCode runtime surface；新增双向依赖边界回归测试。
- 子代理双轴审查发现的布局发现、损坏 header、筛选 CSV 覆盖 canonical 文件及 HTML fallback 问题均已修复。
- 受限并发验证通过：`GOMAXPROCS=2 go test -p 1 -v ./...`、`cd opencode-analyzer && GOMAXPROCS=2 go test -p 1 -v ./...`、`env -u TOKEN_ANALYZER_DB TZ=Asia/Shanghai pnpm exec node --test --test-concurrency=1 'test/**/*.test.ts'`、`pnpm exec tsc --noEmit`。
