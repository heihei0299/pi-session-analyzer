# Codex rollout 数据源适配 Spec

**状态**：ready-for-agent
**日期**：2026-09-08
**数据域**：token-analyzer 多数据源 token usage
**实现目标**：Go-only；保留现有 Pi 默认行为

## Destination

让 `token-analyzer` 读取持久化 Codex CLI rollout，并将可靠的 token usage 接入现有 SQLite、CLI、HTTP API 与 WebUI。Pi 与 Codex 默认分源统计，用户显式选择 `all` 才合计。

本 spec 只定义行为和边界，不包含实现代码。

## Scope

### In scope

- 持久化 Codex rollout 文件；
- `sessions` 与 `archived_sessions` 下的 plain JSONL 和 `.jsonl.zst`；
- Codex `TokenUsageRecord.usage` token 统计；
- `totals` 与 `sessions` 窗口；
- SQLite normalized ledger、增量同步和幂等去重；
- Go CLI、HTTP API、现有 WebUI 的 source selector；
- Go focused tests、回归测试和 release cross-build 验收。

### Out of scope

- `codex exec --json` 管道和 `--ephemeral` 无文件运行记录；
- Codex `requests` 窗口；
- ChatGPT/Codex 订阅实际扣费、账单 API 和 cost 估算；
- TypeScript parser 与 TypeScript parity；
- token 优化、模型推荐和提示词改造。

## User decisions

- source：`pi|codex|all`，默认 `pi`；
- Codex 目录：`--codex-dir > CODEX_HOME > ~/.codex`；`--dir` 永远保持 Pi 目录语义；
- Pi/Codex 默认分开，`all` 显式合计；
- v1 只保证 `totals` + `sessions`；
- Codex cost 使用 unavailable/unpriced，不把 unavailable 当作真实 `$0`；
- Codex 只实现 Go；允许共享 WebUI HTML 的最小修改；
- 不隐式合并 parent/child/fork/revert；保存关系 metadata；
- 不支持 Codex requests 查询，Codex 或 `all` 请求应明确报错；
- 空目录、坏文件和未知 event 返回部分结果并提供 diagnostics，不静默吞掉问题。

## Input contract

### Discovery

- 递归扫描 Codex home 下的 `sessions` 和 `archived_sessions`；
- 识别 canonical rollout 文件名：
  - `rollout-<UTC timestamp>-<thread-id>.jsonl`；
  - `rollout-<UTC timestamp>-<thread-id>_<rollout-id>.jsonl`；
  - 上述 plain filename 的 `.zst` sibling；
- plain 与 compressed sibling 是同一 logical rollout；两者同时存在时只选择 plain；
- 文件名只用于发现、排序和 physical rollout 辅助 identity，session/thread 归属以 `session_meta` 为准；
- 非 canonical 文件跳过并累计 diagnostic。

### Rollout line

每行是带顶层 `timestamp` 的 JSON object，可带 `ordinal`，其余字段按 `type` 解码。缺少 timestamp、无法解码的 item 或坏行不能产生 usage。

必须识别：

- `session_meta`；
- `token_usage_record`；
- `turn_context`；
- 关系和诊断所需的 `event_msg`、`compacted`、`inter_agent_communication` 等。

### Session metadata

保存以下信息（能读取时）：

- `session_id`、当前 thread `id`；
- `cwd`、`originator`、`cli_version`、`model_provider`；
- `parent_thread_id`、`forked_from_id`、fork ordinal；
- `history_base`、`subagent_history_start_ordinal`；
- `thread_source`、agent role/path/nickname。

同一 stable thread 可能对应多个 physical rollout；不得把 path 直接当作 stable session ID。

## Usage contract

### Canonical usage

- 一条 `TokenUsageRecord.usage` 是一个 Codex response usage event，只计一次；
- `response_id` 与 source 组成 logical identity；
- `turn_token_usage`、`thread_token_usage`、`TokenCount` 累计 snapshot 只作校验/诊断，不再次进入账本；
- 没有可靠 `TokenUsageRecord.usage` 的响应不推算 token，不产生 Codex usage row；
- `TokenUsage` 映射：
  - `input_tokens` → input；
  - `cached_input_tokens` → cacheRead；
  - `cache_write_input_tokens` → cacheWrite；
  - `output_tokens` → output；
  - `reasoning_output_tokens` → reasoning；
  - `totalTokens` 按现有 ADR-0002 规则计算为 input + cacheRead + output。

### Model/provider attribution

- 有明确 `turn_id` 关联的 turn model 优先；
- 缺失时回退 `SessionMeta.model_provider` 可表达的 session-level 信息；
- 再缺失使用 `unknown`；
- `ModelReroute` 只保存诊断，不把 reroute target 强行归属到 response；
- model 不可知不影响 token 计入。

### Cost

- `total_cost_usd` 使用现有 unavailable sentinel `"0"`；
- `pricing_model` 使用 `unpriced`；
- UI/JSON/API 必须能区分 unpriced 与真实零花费。

## Ledger and synchronization

### Existing tables

复用：

- `proxy_request_logs`：normalized Codex usage rows，`data_source=codex`；
- `session_usage_dedup`：`source + response_id` 幂等 identity 和冲突诊断；
- `session_log_sync`：physical path 的 revision、line/byte cursor 和同步状态；
- 现有 rollup/query 表和聚合逻辑。

不新建 Codex 专用 ledger，不复制 SessionData、Totals 或查询实现。

### Revision behavior

- physical file 的 size、mtime、tail fingerprint 或可读性状态变化时全量重扫；
- 旧账本不删除，重复 response 由 identity 去重；
- plain↔zstd path 切换视为 physical path 变化，重扫新表示；
- plain 与 compressed 同时存在时 plain 优先；
- 半行不推进 cursor。

### Malformed and conflict behavior

- 只提交完整换行记录；
- 坏行和未知 event 跳过并累计 warnings/diagnostics；
- 单个坏行不阻止其他合法行导入；
- 同一 response ID：
  - 首条完整 usage 优先；
  - 首条无有效 usage 时可接受后续完整 usage；
  - 两条完整 payload 冲突时保留首条并记录 conflict；
- usage inserts、dedup 结果和该 physical file cursor 在同一事务提交；
- 同步失败不得推进已失败位置的 cursor。

### Time

- usage 时间使用 rollout envelope timestamp；
- 无效时回退 session metadata timestamp；
- 两者都无效时记录仍可保留，但不参与时间范围过滤。

## Go implementation seam

新增独立 `internal/codex` package，职责最小化为：

1. discovery：解析 Codex home、递归发现 rollout、plain/zstd sibling 去重；
2. parser：读取 envelope、metadata、`TokenUsageRecord` 和 model attribution 所需上下文；
3. sync：revision/cursor、事务导入、dedup、diagnostics；
4. adapter：把 Codex source 接入现有 CLI/API/server 查询入口。

复用：`internal/db`、`internal/domain`、`internal/timerange`、现有序列化和 server/query。暂不抽象通用 source interface，不改 `internal/pi` 内部协议。

### zstd dependency

使用 `github.com/klauspost/compress/zstd` 的 Go 1.23 兼容版本线（研究快照：`v1.18.0`）。不使用 CGO wrapper、系统 `zstd` 命令或 seekable zstd。

### WebUI

只对共享 `src/webui.html` 做最小修改：

- 增加 `Pi / Codex / All` source selector；
- 复用现有 totals、groups、sessions 页面；
- Codex cost 显示 unavailable；
- Codex requests Tab/查询显示明确 unsupported；
- 不新增独立 Codex Tab。

## Query contract

### CLI

现有命令保持兼容，增加：

```text
token-analyzer [totals|sessions|requests] [--source pi|codex|all] [--dir <pi-dir>] [--codex-dir <codex-home>]
```

- 默认 `--source pi`；
- `--source codex` 使用 Codex directory；
- `--source all` 同步并合计 Pi/Codex；
- `requests` 配合 `codex` 或 `all` 明确报错；
- source 参数错误、目录不可访问和 unsupported view 提供稳定可诊断错误。

### HTTP API

现有 `/api/totals`、`/api/sessions`、`/api/groups`、`/api/period`、`/api/meta` 支持 source filter；`/api/requests` 对 `codex/all` 返回明确 unsupported error。

- `sessions` 行增加 `source`；
- `meta` 返回实际参与查询的 sources 和 warnings/diagnostics；
- `all` 的 totals 仍返回现有结构，不新增 `piTotals/codexTotals/combinedTotals` 三套嵌套字段；
- 空 Codex 目录可返回空结果和 warnings，不把它当服务器错误。

## Acceptance matrix

使用 synthetic、脱敏 fixture，不提交真实 session、数据库或 secrets。

### Parser/import

- canonical plain rollout 可发现；
- archived rollout 可发现；
- `.jsonl.zst` 可读取；
- plain/compressed sibling 只导入一次；
- metadata identity、cwd、provider、parent/fork 字段正确；
- `TokenUsageRecord.usage` 字段映射正确；
- 缺 usage 不产生 row；
- model 明确关联、session fallback、unknown 三种路径正确；
- reroute 不错误改写 response model。

### Dedup/sync

- 同一 response 重扫不增加 token；
- plain↔zstd path 切换不增加 token；
- append 只处理新完整行；
- 半行在下一次完整后才提交；
- truncate/replace 触发全量重扫但不删旧账；
- 坏行/未知 event 有 diagnostics 且不阻塞合法行；
- 冲突 response 保留首条并有 conflict；
- usage 和 cursor 原子提交。

### Query/UI

- 默认 Pi 查询无行为回归；
- Pi/Codex/All source filter 正确；
- all totals 显式合计且 sessions 带 source；
- Codex requests 和 all requests 明确拒绝；
- unpriced cost 不显示为真实零花费；
- 空目录和部分失败可见但不误报服务失败。

### Commands

```sh
go test -v ./...
go build ./cmd/token-analyzer
make release
```

CI/发布前必须覆盖现有 Linux amd64/arm64、macOS amd64/arm64、Windows amd64 release targets。TypeScript 不新增 Codex 实现；既有 TypeScript tests 只用于确认 Pi 行为未回归。

## References

- 领域术语：[`CONTEXT.md`](../../CONTEXT.md)
- 决策地图：[`map.md`](map.md)
- Codex 上游格式 research：[`research/01-codex-rollout-format.md`](research/01-codex-rollout-format.md)
- Codex usage research：[`research/02-codex-usage-semantics.md`](research/02-codex-usage-semantics.md)
- Codex identity research：[`research/03-codex-session-identity.md`](research/03-codex-session-identity.md)
- Codex model research：[`research/04-codex-model-attribution.md`](research/04-codex-model-attribution.md)
- Go zstd research：[`research/05-go-zstd-compatibility.md`](research/05-go-zstd-compatibility.md)
