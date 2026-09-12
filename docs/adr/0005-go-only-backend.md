# ADR-0005 — Go-only 后端收尾（删除 TypeScript 生产实现）

- **状态**: Accepted（2026-09-12，Go-only 迁移 05）
- **影响组件**: `go.mod`（module 统一为 `github.com/heihei0299/token-analyzer`）、`internal/query`（唯一统计 seam）、`internal/refresh`、`internal/server`（单 WebUI 源码）、`testdata/canonical`（长期契约）、`.github/workflows`（Go-only CI/发版）、`README.md`、`CONTEXT.md`、`opencode-analyzer/`（独立边界）
- **前置**: ADR-0001（fork 去重）、ADR-0002（totalTokens/cacheRate）、ADR-0003（存储对齐）、ADR-0004（input/cacheRead 语义）；04 已收口 Refresh/Query/Watch/Server/WebUI 运行时

## 背景

迁移完成前同一领域存在两套生产后端：TypeScript（CLI/API/server/db/session/watch）与 Go（ledger + Query Engine），以及迁移期 TS parity oracle、旧 SessionData 文件扫描/内存聚合、Codex SessionFileData 回绕、Watch 独立统计、WebUI 双副本。Go canonical acceptance（Pi 四载体/fork/双账本/增量 + Codex durable/zstd/diagnostics，字段级 golden）已覆盖全部用户可见行为，继续保留 TS 只会分叉口径并强制用户理解两个版本。

## 决策

- **Go 是唯一生产后端**：删除 `src/`、`test/*.test.ts`、`test/helpers.ts`、`test/parity_test.go`、`package.json`、`package-lock.json`、`tsconfig*.json` 与 npm `publish.yml`；运行 CLI/API/WebUI 不需要 Node/npm。
- **统计唯一性**：normalized SQLite ledger 为唯一事实中心；`internal/query` 为 totals/sessions/requests/groups/period/detail/meta 唯一生产统计 seam；Query 只读快照（`OpenReadOnly + query_only`），不做 discovery/parse/sync/写库。
- **职责收口**：`sessiondata` 只提供查询 DTO 与共享显示名/cwd/周期/排序 helper；Pi 重命名通过 `internal/pi` header-only locator 定位文件，不再触发旧 JSONL usage parser 或文件缓存。
- **Refresh/Watch 语义**：Refresh（`internal/refresh` + Pi/Codex adapter）为唯一写入路径，串行化，失败保快照并经 meta 暴露；Watch 只做 change → refresh → query，无独立 token/cost 聚合。
- **WebUI 单一源码**：`internal/server/webui.html` 为唯一人工维护源（Go embed 直引），无 copy/sync；`meta.sources` 恒为 `["pi","codex"]`。
- **契约**：`testdata/canonical` + Go golden tests 为长期回归（替代跨 runtime parity）；fixtures 全为合成数据。
- **命名统一**：module/repository/import/release 统一为当前项目名 `token-analyzer`（`go.mod: github.com/heihei0299/token-analyzer`）；旧拼写 `pi-session-anylize` 为 breaking change 直接修正；`opencode-analyzer/` 保持独立 module `github.com/heihei0299/opencode-analyzer`，可整目录迁出。
- **发版**：只发 Go 二进制（`release.yml` + `make release`），普通构建产物为 `dist/token-analyzer`，平台 release artifact 保留平台后缀；不再区分 Go/npm edition，不再发 npm 包。
- **OpenCode 边界**：`opencode-analyzer/internal/piaudit` 自己维护本地 Pi audit 语义和 synthetic fixture；该目录可整目录迁出，token-analyzer 不依赖其 runtime/API/UI/storage/credential。

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
