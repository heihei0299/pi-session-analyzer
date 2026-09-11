# Go-only 后端迁移架构审计报告

> 审计基线：`main@05edcfc702a5439dc363d41724a5d23732b5cc9a`
>
> 审计日期：2026-09-12
>
> 范围：后端架构、数据流、统计口径、双实现漂移、查询/同步职责、Watch、WebUI 交付方式、测试与迁移风险。
>
> 本报告为静态审计；未执行 build / test / lint / typecheck。

## 1. 结论

项目当前最主要的架构问题不是代码规模，而是**同一统计领域存在多个事实来源与多套统计执行路径**。

当前同时存在：

1. Go 的 Pi 文件扫描 + `SessionData` 内存聚合路径；
2. TypeScript 的 Pi → SQLite → SQL 聚合路径；
3. Go 的 Codex → SQLite → 再加载为 `SessionFileData` → 内存聚合路径；
4. Watch 独立增量读取并直接维护 Totals；
5. OpenCode 独立 client/storage/audit 数据流。

ADR-0003 已经决定以 normalized SQLite ledger 为主存储，ADR-0004 已经决定跨源 input/cacheRead 语义归一，但实现尚未完全收口到这一架构。

**建议：后端全部切换为 Go，Go 成为唯一 canonical implementation；Pi/Codex 均先归一写入 normalized ledger，所有 totals/sessions/requests/groups/period/detail/meta 查询都只经过一个 Go Query Engine。**

目标不应是“把 TypeScript 翻译成 Go”，而应是借迁移机会删除重复领域逻辑。

---

## 2. 当前架构事实

### 2.1 双后端

Go 主要实现位于：

- `cmd/token-analyzer/main.go`
- `internal/pi/`
- `internal/codex/`
- `internal/db/`
- `internal/query/`
- `internal/sessiondata/`
- `internal/server/`
- `internal/watch/`
- `internal/opencode/`

TypeScript 仍维护对应能力：

- `src/cli.ts`
- `src/api.ts`
- `src/server.ts`
- `src/session-data.ts`
- `src/db.ts`
- `src/db-aggregation.ts`
- `src/pi-discovery.ts`
- `src/pi-parse.ts`
- `src/pi-identity.ts`
- `src/pi-sync.ts`
- `src/watch.ts`
- `src/opencode/*`

两边重复承载了发现、解析、时间语义、序列化、聚合、HTTP/CLI 适配等领域规则。

### 2.2 Pi 查询路径没有统一经过 ledger

Go 的 Pi 普通查询当前大致为：

```text
cmd/token-analyzer
  -> query.Query
  -> source == pi
  -> SessionData.Query
  -> ReadSessionFilesCached
  -> AnalyzeFile
  -> QueryFiles
  -> in-memory aggregation
```

TypeScript 普通查询已经主要通过：

```text
Pi JSONL
  -> syncPiUsage
  -> SQLite
  -> db-aggregation
  -> CLI / API
```

因此 ADR-0003 的“SQLite 直切”在两种运行时之间并未真正形成单一路径。

### 2.3 SessionData 同时承担 source 与 query 两层职责

`internal/sessiondata/sessiondata.go` 当前同时包含：

- 文件递归发现；
- JSONL header/message 解析；
- fork 去重；
- 文件快照缓存；
- cwd 归一；
- Filter；
- totals/sessions/requests/groups/period；
- 排序、分页；
- detail。

这是一个很深的 module，但目前深度来自把两个不同层次揉在一起：

- source ingestion；
- domain query。

迁移中不建议简单拆成大量小文件；应删除 ingestion 职责，保留并演进 query seam。

### 2.4 Codex Query 带同步副作用

当前 `internal/query/query.go` 的 Codex 查询会：

```text
Query
  -> Open DB
  -> DiscoverRollouts
  -> SyncRollouts
  -> LoadSessionFiles
  -> QueryFiles
```

Query interface 因此同时承担 Refresh + Query。

这使 HTTP 请求的读取语义不纯，并让未来的并发、缓存、轮询和性能调优更困难。

### 2.5 Watch 是第三套统计引擎

Go Watch 当前通过独立 `IncrementalReader` 和 `ApplyIncrements` 维护 Totals。

这意味着下列规则必须同时在正常 ingestion 与 watch 中保持一致：

- 四载体计入口径；
- fork；
- dedup；
- cacheRead/cacheWrite；
- reasoning；
- pricing；
- failure records；
- 文件 rewrite/partial-line。

Watch 应仅负责变化检测与 refresh 触发，而不应成为 usage 规则的第二实现。

### 2.6 WebUI 存在源码双副本

当前：

- `src/webui.html`
- `internal/server/webui.html`

大小一致，并已有同步构建步骤避免漂移。

这说明这里已经形成真实 seam，应收敛成一个人工维护的 WebUI source。

### 2.7 Go module / repository 元数据仍使用旧名称

GitHub 仓库已为 `pi-session-analyzer`，但当前仍可见：

- `go.mod`: `github.com/heihei0299/pi-session-anylize`
- `package.json.repository`: 旧地址

这不是迁移阻塞项，但 Go-only 收尾时应统一，避免长期命名漂移。

---

## 3. 风险评级

| 等级 | 发现 | 影响 |
|---|---|---|
| P0 | Go Pi 普通查询绕过 normalized ledger | 同一数据在不同入口可能使用不同计入口径 |
| P0 | Go/TS 重复实现同一领域规则 | 每次规则变化都存在 runtime 漂移风险 |
| P0 | parity test 对很多窗口只比较行数 | 双实现错误可能同时保持“测试绿色” |
| P1 | Codex Query 内含 Sync 写副作用 | HTTP 查询成本和并发行为不可预测 |
| P1 | Watch 独立计算 Totals | 第三套统计语义，维护成本高 |
| P1 | Codex ledger 后又转换回 SessionFileData | normalized ledger 没成为真正 query seam |
| P1 | WebUI 双副本 | UI 修改存在同步漂移 |
| P2 | module/repo 旧命名 | 发布、import、AI/code navigation 噪声 |

---

## 4. 已发生问题说明了什么

近期 Codex 修复中，曾出现上游 `input_tokens` 已包含 cached input，但适配又把 `cached_input_tokens` 叠加进入总量的问题。

结果曾导致：

- totalTokens 大幅虚高；
- cacheRate 明显失真；
- Pi/Codex 同名字段语义不一致。

ADR-0004 已修正规则。

这个事件说明真正需要保护的不是具体 parser，而是：

> **source adapter 必须把源特有语义归一到统一 ledger record；query 层只能面对归一后的字段。**

如果 query 层还需要了解 Codex/Pi 上游字段差异，说明 seam 放错了位置。

---

## 5. 推荐目标架构

```text
Pi Sessions -------------------┐
                              │
                    Pi Source Adapter
             discover / parse / identity
             fork / dedup / incremental
                              │
                              v
                     Normalized Ledger
                         SQLite
                              ^
                              │
                  Codex Source Adapter
             discover / parse / identity
           diagnostics / zstd / incremental
                              ^
                              │
Codex Rollouts ----------------┘

                     Normalized Ledger
                              │
                              v
                       Query Engine
          totals / sessions / requests / groups
              period / detail / meta / audit
                              │
                 ┌────────────┼────────────┐
                 v            v            v
                CLI        HTTP API       Watch
                              │
                              v
                            WebUI
```

### 核心约束

1. Go 是唯一后端语言。
2. SQLite normalized ledger 是唯一事实中心。
3. Source adapter 的差异止于入库之前。
4. Query Engine 不解析 JSONL，不读取 rollout，不计算 source-specific semantics。
5. Query 不执行 Sync。
6. Watch 不计算 token usage。
7. WebUI 只有一个人工维护源文件。
8. OpenCode 暂时保持 benchmark/audit 模块，不强行提升为正式 source。

---

## 6. 推荐 module / seam

建议最终保持少量深模块，而不是引入企业式多层抽象。

```text
cmd/token-analyzer/
internal/
  domain/
  pi/
  codex/
  ledger/
  query/
  timerange/
  render/
  serialize/
  watch/
  server/
  opencode/
```

可在迁移完成后再决定是否把 `pi/`、`codex/` 移至 `source/`。迁移期间不建议仅为目录美观制造大规模 rename diff。

### 6.1 Pi / Codex adapter

每个 adapter 对外只需要少量高 leverage interface，例如：

```go
type Refresher interface {
    Refresh(ctx context.Context) (RefreshResult, error)
}
```

adapter 内部自行处理：

- discovery；
- file revision；
- parse；
- upstream semantics；
- identity；
- dedup；
- diagnostics；
- ledger writes。

### 6.2 Query Engine

推荐成为真正稳定的 domain interface：

```go
type Engine struct {
    ledger *ledger.DB
}

func (e *Engine) Totals(ctx context.Context, f Filter) (Totals, error)
func (e *Engine) Sessions(ctx context.Context, f Filter, p Page) (SessionPage, error)
func (e *Engine) Requests(ctx context.Context, f Filter, p Page) (RequestPage, error)
func (e *Engine) Groups(ctx context.Context, f Filter, by GroupBy) ([]GroupRow, error)
func (e *Engine) Period(ctx context.Context, f Filter, period Period) ([]PeriodRow, error)
func (e *Engine) Detail(ctx context.Context, sessionID string) (SessionDetail, error)
func (e *Engine) Meta(ctx context.Context, source Source) (Meta, error)
```

接口是否最终采用这些签名，应在实现阶段按现有调用面最小化，不要求为了本报告创建抽象。

---

## 7. 迁移期间必须保留的兼容性基线

在删除 TS 前，必须把它当成 migration oracle，而不是继续当长期产品。

需要覆盖的 fixture 至少包括：

### Pi

- assistant；
- toolResult；
- compaction；
- branch_summary；
- failure/aborted；
- fork history；
- nested fork；
- task/parentSession；
- duplicate requestId；
- semantic duplicate；
- partial final line；
- append；
- truncate；
- same-size rewrite；
- model change；
- cross-day session；
- message-level time range；
- session-level time range；
- mixed models；
- pricing missing/present。

### Codex

- normal durable usage；
- cached input；
- cached > input anomaly；
- snapshot-only；
- diagnostics replay；
- plain/zstd physical duplicate；
- revert rollout；
- multi-rollout thread；
- archive/session discovery；
- invalid rollout names。

每个 fixture 应对应 canonical expected JSON，而不是仅断言 row count。

---

## 8. 不推荐的做法

### 不推荐：直接把 TS 文件逐个翻译成 Go

这会保留双引擎时代的重复设计。

### 不推荐：先大规模重命名目录

先收口数据流，再整理目录。否则评审 diff 会被 rename 淹没。

### 不推荐：把 SessionData 拆成很多浅 module

删除错误职责比拆文件更重要。

### 不推荐：为了统一而把 OpenCode 强行做成 source

OpenCode 当前领域词义是外部 benchmark/audit。除非未来真的支持 `--source opencode`，否则保持旁路更清晰。

### 不推荐：一次 PR 同时完成全部删除

应采用可回滚阶段，每一阶段都保留明确验收点。

---

## 9. 最优先行动

第一优先级不是删除 TypeScript，而是完成：

> **Go Pi → normalized SQLite ledger → Go Query Engine**

它完成后，Go 才有资格成为 canonical backend。

第二优先级：

> **Codex 直接从 ledger query，取消 SQLite → SessionFileData → memory aggregation 的回绕。**

第三优先级：

> **Refresh 与 Query 分离，Watch 只触发 Refresh。**

做到这里以后，TypeScript 后端的删除会变成低风险清理，而不是功能迁移。

---

## 10. 审计结论

**Recommendation: Strong — 执行 Go-only backend migration。**

Go-only 与项目当前演进方向一致：

- 已有 Go binary；
- Codex 能力已经偏向 Go；
- SQLite 已进入 Go；
- normalized ledger 已有 ADR；
- 跨源语义已有统一定义；
- npm/TS 已出现能力声明差异。

迁移成功的判据不是“仓库没有 .ts”，而是：

> **任何 usage 统计规则只存在一个生产实现；任何 source-specific 语义只存在于 ingestion adapter；任何展示入口都查询同一个 normalized ledger。**
