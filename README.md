# Token Analyzer

Go-only CLI 和本地 Web 面板，用于分析 Pi 与 Codex 会话中的 token 消耗。支持总量、会话、请求三个统计窗口，按模型、目录和时间筛选，并导出 JSON / CSV。单二进制运行，无需 Node.js。

OpenCode 云端用量工具已拆分为 [OpenCode Analyzer](https://github.com/heihei0299/opencode-analyzer)。

## 安装

需要 Go 1.23+：

~~~sh
go install ./cmd/token-analyzer
~~~

在仓库中构建：

~~~sh
make build
make release
~~~

## 使用

~~~sh
token-analyzer totals
token-analyzer sessions --source codex --codex-dir ~/.codex --format json
token-analyzer totals --by model --since 2026-07-01 --until 2026-08-31
token-analyzer totals --watch --interval 1000
token-analyzer serve
~~~

`serve` 默认在 `http://127.0.0.1:50080/` 启动 Web 面板。可用 `--dir` 指定 Pi 会话目录，`--codex-dir` 指定 Codex 目录，`--db` 指定 ledger；Codex 目录也可由 `CODEX_HOME` 设置。

常用选项：

- `--source pi|codex|all` 选择数据源（默认 `pi`）。
- `--format table|json|csv` 选择输出格式。
- `--model`、`--cwd`、`--since`、`--until` 筛选结果。
- `--by model|cwd|model,cwd` 和 `--period day|week|month` 用于总量汇总。

| 数据源 | 支持 |
| --- | --- |
| Pi | 所有统计窗口、Web 面板、会话详情和重命名 |
| Codex | 总量与会话统计；不支持请求窗口、会话详情或重命名 |
| `all` | 合并可用的总量和会话数据；请求窗口和会话管理不可用 |

总 token 按 `input + cacheRead + output` 计算；Pi fork 会话的复制历史会去重。Codex 数据没有美元定价时标记为未定价。Pi 数据库绑定到一个会话根目录；分析不同根目录时请使用独立的 `--db`。

完整统计口径、安全边界和架构决策见 [`docs/audit/`](docs/audit/) 与 [`docs/adr/`](docs/adr/)。
