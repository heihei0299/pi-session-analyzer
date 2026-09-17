# ADR-0005 — Go-only 后端收尾（删除 TypeScript 生产实现）

- **状态**: Accepted（2026-09-12，Go-only 迁移 05）
- **影响组件**: `go.mod`（module 统一为 `github.com/heihei0299/token-analyzer`）、`internal/query`（唯一统计 seam）、`internal/refresh`、`internal/server`（单 WebUI 源码）、`testdata/canonical`（长期契约）、`.github/workflows`（Go-only CI/发版）、`README.md`、`CONTEXT.md`、OpenCode 产品边界（外部项目）
- **前置**: ADR-0001（fork 去重）、ADR-0002（totalTokens/cacheRate）、ADR-0003（存储对齐）、ADR-0004（input/cacheRead 语义）；04 已收口 Refresh/Query/Watch/Server/WebUI 运行时

## 背景

迁移完成前同一领域存在两套生产后端：TypeScript（CLI/API/server/db/session/watch）与 Go（ledger + Query Engine），以及迁移期 TS parity oracle、旧 SessionData 文件扫描/内存聚合、Codex SessionFileData 回绕、Watch 独立统计、WebUI 双副本。Go canonical acceptance（Pi 四载体/fork/双账本/增量 + Codex durable/zstd/diagnostics，字段级 golden）已覆盖全部用户可见行为，继续保留 TS 只会分叉口径并强制用户理解两个版本。

## 决策

- **Go 是唯一生产后端**：删除 `src/`、`test/*.test.ts`、`test/helpers.ts`、`test/parity_test.go`、`package.json`、`package-lock.json`、`tsconfig*.json` 与 npm `publish.yml`；运行 CLI/API/WebUI 不需要 Node/npm。
- **统计唯一性**：normalized SQLite ledger 为唯一事实中心；`internal/query` 为 totals/sessions/requests/groups/period/detail/meta 唯一生产统计 seam；Query 只读快照（`OpenReadOnly + query_only`），不做 discovery/parse/sync/写库。
- **职责收口**：`sessiondata` 只提供查询 DTO 与共享显示名/cwd/周期/排序 helper；Pi 重命名通过 `internal/pi` header-only locator 定位文件，不再触发旧 JSONL usage parser 或文件缓存。
- **存储生命周期**：Refresh 成功后执行 30 天 `RollupAndPrune`；raw → daily rollup → delete 在一个事务内完成，且只针对 token-analyzer 自有行（Pi `app_type='pi' AND data_source='pi_session'`、Codex `app_type='codex' AND data_source='codex'`），共享 DB 不授予其他 proxy 行的所有权；partial-day 查询只使用安全的完整日 rollup，并暴露历史 coverage 边界。
- **Pi root 隔离（后续收口）**：schema v3 新增 `source_root_bindings`，一个 ledger 只绑定一个 Pi root 的真实路径（`Abs/Clean + EvalSymlinks`）。首次 Pi Refresh 建立 binding，切换 root 的 Refresh/Query/QueryDetail 拒绝且不删除历史行；旧 v2 或无 binding 的只读 ledger fail closed；可写 migration 只补齐 schema，若已有 Pi 历史行，首次 Refresh 也必须显式迁移/确认或使用新 ledger，不能自动认领，只有空 ledger 才能首次绑定。不存在或无法解析 symlink target 的 root 不会被当作可绑定的词法路径。
- **Refresh/Watch 语义**：Refresh（`internal/refresh` + Pi/Codex adapter）为唯一写入路径，串行化，失败保快照并经 meta 暴露；Watch 先用 source-specific cheap revision，只有对应源变化才做该源 Refresh，失败保留 acknowledged revision 并重试；无独立 token/cost 聚合。`source=all` 未配置可用 Pi root 时采用明确 Codex-only 语义：不建立 Pi binding、不读取 Pi history；`source=pi` 仍拒绝不可用 root。
- **WebUI 单一源码**：`internal/server/webui.html` 为唯一人工维护源（Go embed 直引），无 copy/sync；`meta.sources` 恒为 `["pi","codex"]`。
- **契约**：`testdata/canonical` + Go golden tests 为长期回归（替代跨 runtime parity）；fixtures 全为合成数据；CI 在 pull request 和 `main` 变更时执行 root token-analyzer Go module 的格式检查、`go vet` 和串行测试，release 只执行 root module 的构建与 smoke check；OpenCode Analyzer 的测试与发布由其外部项目负责。
- **命名统一**：module/repository/import/release 统一为当前项目名 `token-analyzer`（`go.mod: github.com/heihei0299/token-analyzer`）；旧拼写 `pi-session-anylize` 为 breaking change 直接修正；OpenCode Analyzer 使用独立 module `github.com/heihei0299/opencode-analyzer`，已正式迁出本仓库。
- **发版**：Git `v<版本>` tag 是唯一版本来源；`make build/release` 只将 tag 派生值注入 CLI，普通构建产物为 `dist/token-analyzer`，平台 release artifact 保留平台后缀；release workflow 在发布前校验 `--version` 与 tag 一致；不再区分 Go/npm edition，不再发 npm 包。
- **OpenCode 边界**：OpenCode Analyzer 在外部项目中独立维护本地 Pi audit 语义（canonical message fields + complete usage payload）和 synthetic fixture；token-analyzer 不依赖其 runtime/API/UI/storage/credential。

## 后果

**正面**

- 用户只安装运行一个 Go 后端；口径、All 合计、costStatus、capability 声明只有一种真相。
- 测试只需 `GOMAXPROCS=2 go test -p 1 ./...`，无需 Node 工具链。

**负面/约束**

- 删除 npm 分发为 breaking change：依赖 `npm i -g token-analyzer` 的用户需改用 GitHub Release 二进制或 `go install`。
- 历史 TS 测试随删除而移除；行为锁定完全依赖 Go canonical golden，新增口径必须先补 fixture + golden。

## 备选方案（未采纳）

- **保留 TS 只读兼容层**：让 TS 继续提供 Pi 查询以照顾 npm 用户；与“只有一个生产实现”相悖，口径必分叉。
- **保留 parity oracle**：长期双跑 Node vs Go；CI 成本翻倍，且 golden 已能独立锁行为。
