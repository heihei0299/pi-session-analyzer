# Go-only 后端迁移执行计划

> Branch: `migration/go-backend-only-20260912`
>
> Base: `main@05edcfc702a5439dc363d41724a5d23732b5cc9a`
>
> 目标：后端全部切换到 Go；TypeScript 后端退役；保留单一 WebUI。
>
> 本文是实施 checklist。每个 Ticket 必须可以独立 review、回滚和验收。

## 0. Definition of Done

只有全部满足下列条件，才算迁移完成：

- [ ] Go 是唯一生产后端实现。
- [ ] Pi 与 Codex 都通过 normalized SQLite ledger。
- [ ] totals/sessions/requests/groups/period/detail/meta 只有一个生产 query engine。
- [ ] Query 路径不触发 ingestion/write。
- [ ] Watch 不独立计算 usage。
- [ ] WebUI 只有一个人工维护源码。
- [ ] TypeScript CLI/API/server/db/session-data/watch/opencode 后端已删除。
- [ ] Node/npm 不再是运行 token-analyzer 的必要依赖。
- [ ] canonical fixtures 覆盖 Pi 四载体、fork/dedup/incremental 和 Codex cache/diagnostics/zstd/revert。
- [ ] 删除 TS 后，所有 canonical expected fixtures 继续由 Go 测试保护。
- [ ] README、安装、release、module/repository metadata 全部指向 Go-only 产品。
- [ ] 现有 ADR/CONTEXT 与最终实现一致。

---

# Phase 1 — 建立迁移安全网

## GO-00：冻结 canonical behavior

### 目标

在删除任何 TS 生产路径之前，把当前确认过的领域语义固定为可执行 fixtures。

### 工作项

- [ ] 新建统一 fixture 根目录，例如 `testdata/semantic/`。
- [ ] Pi fixture：
  - [ ] assistant
  - [ ] toolResult
  - [ ] compaction
  - [ ] branch_summary
  - [ ] failed/error
  - [ ] aborted
  - [ ] fork copied history
  - [ ] nested fork
  - [ ] task session
  - [ ] requestId duplicate
  - [ ] semantic duplicate
  - [ ] partial line
  - [ ] append
  - [ ] truncate
  - [ ] rewrite
  - [ ] cross-day
  - [ ] mixed-model
- [ ] Codex fixture：
  - [ ] cached input
  - [ ] cached > input anomaly
  - [ ] snapshot-only
  - [ ] diagnostics replay
  - [ ] zstd/plain same physical rollout
  - [ ] revert
  - [ ] archived rollout
  - [ ] invalid/noncanonical rollout diagnostics
- [ ] 每个 fixture 添加 canonical expected JSON：
  - [ ] totals
  - [ ] sessions
  - [ ] requests（支持源）
  - [ ] groups
  - [ ] period
  - [ ] meta/diagnostics
- [ ] 改造 `test/parity_test.go`：
  - [ ] 不再只比较 row count。
  - [ ] canonicalize 后比较完整字段。
  - [ ] 浮点 cost 使用明确容差。
  - [ ] 排序结果使用稳定次级键。

### 验收

- [ ] Go/TS 对 Pi canonical fixtures 输出一致。
- [ ] 当前已接受的 ADR 行为均有 fixture 对应。
- [ ] 任何后续迁移 ticket 都不能修改 expected，除非同时更新 ADR/CONTEXT 并说明行为变化。

### 禁止

- [ ] 不在此 Ticket 改 production architecture。
- [ ] 不删除 TS。

---

# Phase 2 — Go Pi 切入 normalized ledger

## GO-01：Go Pi ingestion 成为正式写入路径

### 目标

让 Go 完整承担 Pi 的 discovery/parse/identity/dedup/incremental/database write。

### 当前问题

Go 普通 Pi 查询仍可绕过 ledger，进入 `SessionData.AnalyzeFile`。

### 工作项

- [ ] 核对 `internal/pi` 与 ADR-0003 四载体规则：
  - [ ] assistant
  - [ ] toolResult
  - [ ] compaction
  - [ ] branch_summary
- [ ] 核对 billable/cost/failed 门控。
- [ ] 核对 request_id / semantic_id。
- [ ] 核对 forkTs。
- [ ] 核对 append/partial-line/rewrite/truncate。
- [ ] 核对 cwd/session metadata 落库。
- [ ] 核对 model pricing。
- [ ] 为 Go 增加 `RefreshPi` 或等价 use case。
- [ ] CLI `sync` 走 Go Pi ingestion。
- [ ] server 启动/刷新机制可以调用同一 ingestion。

### 验收

- [ ] GO-00 全部 Pi fixtures 通过。
- [ ] 重复 Refresh 不增加 ledger 记录。
- [ ] 文件 append 只处理新增完整行。
- [ ] rewrite/truncate 不双算。
- [ ] fork copied history 不进入账本。

### 删除条件

此阶段**不删除** `SessionData.AnalyzeFile`，但标记为 legacy read path，后续 GO-03 删除。

---

# Phase 3 — 单一 Query Engine

## GO-02：建立 ledger-backed Query Engine

### 目标

所有统计窗口只从 normalized ledger 读取。

### 建议 seam

`internal/query` 成为唯一领域查询 module。

### 工作项

- [ ] 定义统一 `query.Filter`：
  - [ ] source
  - [ ] model
  - [ ] cwd
  - [ ] time range
- [ ] 明确 SessionTimeRange / MessageTimeRange 使用位置。
- [ ] 将下列查询迁移为 SQL/ledger-backed：
  - [ ] totals
  - [ ] sessions
  - [ ] requests
  - [ ] groups
  - [ ] period
  - [ ] detail
  - [ ] meta
- [ ] source=pi 只通过 ledger。
- [ ] source=codex 只通过 ledger。
- [ ] source=all 只通过 ledger union/filter。
- [ ] costStatus 在 query/domain 层统一。
- [ ] requests/detail 的 source capability guard 保持明确。
- [ ] 所有排序增加稳定 tie-breaker。
- [ ] pagination 在 DB/query 层完成，不先全量 materialize 再切片。

### 关键约束

Query Engine：

- [ ] 不读取 JSONL。
- [ ] 不读取 zstd rollout。
- [ ] 不调用 discovery。
- [ ] 不写 DB。
- [ ] 不理解 `input_tokens` 等上游 source-specific 字段。

### 验收

- [ ] 相同 DB snapshot 下 Query 结果 deterministic。
- [ ] Query 前后 DB write counters/mtime 无变化。
- [ ] Pi/Codex/All totals 与 canonical expected 一致。
- [ ] sessions totals 与 detail/period/groups 可解释一致。
- [ ] cross-day 按 CONTEXT 的 message/session time semantics 工作。

---

# Phase 4 — 删除 SessionData 旧读路径

## GO-03：移除文件扫描 + 内存聚合生产路径

### 目标

彻底消除第二统计引擎。

### 工作项

从 `internal/sessiondata` 识别并删除/迁移：

- [ ] `CollectJsonlFiles`
- [ ] `AnalyzeFile`
- [ ] `ReadSessionFilesCached`
- [ ] 文件级 cache / singleflight（若仅为旧扫描服务）
- [ ] 基于 `[]SessionFileData` 的生产 QueryFiles 路径
- [ ] Codex `LoadSessionFiles -> QueryFiles` 回绕

保留或迁移仍有价值的领域能力：

- [ ] domain row/result types → `internal/domain` 或 `internal/query`
- [ ] stable display-name 规则
- [ ] filter/view enums（若仍必要）

### 验收

- [ ] production binary 中没有“扫描所有 session JSONL 后直接 aggregate”的查询路径。
- [ ] Codex 查询不再转换为 `SessionFileData` 后内存聚合。
- [ ] 删除旧路径后 GO-00 fixtures 不变。

### 删除测试

应用 deletion test：

> 删除 SessionData 旧 source/read 路径后，复杂度应集中到 source adapter + ledger/query，而不是在 server/CLI 重新出现。

---

# Phase 5 — Refresh 与 Query 分离

## GO-04：引入明确 Refresh lifecycle

### 目标

消除“GET 查询隐式写数据库”。

### 工作项

- [ ] 将当前 Codex `syncAndLoad` 从 Query 中移出。
- [ ] 定义 refresh orchestration：
  - [ ] Refresh Pi
  - [ ] Refresh Codex
  - [ ] collect diagnostics
- [ ] server 启动执行初次 refresh。
- [ ] 后续 refresh 采用：
  - [ ] filesystem change trigger，或
  - [ ] bounded poll interval。
- [ ] 并发 refresh 使用 singleflight/mutex，禁止重复写。
- [ ] HTTP Query 只读。
- [ ] refresh error 不破坏已有 snapshot 查询。
- [ ] meta 暴露 lastRefresh/diagnostics（如有产品价值）。

### 验收

- [ ] 连续调用 totals API 不导致 sync。
- [ ] 多 API 并发读取不会重复 discovery/sync。
- [ ] refresh 失败时仍可读取上一次已提交 snapshot。

---

# Phase 6 — Watch 收口

## GO-05：Watch 只负责变化通知

### 目标

删除 Watch 的独立 token aggregation 规则。

### 工作项

- [ ] Watch 检测变化。
- [ ] 变化后调用对应 source refresh。
- [ ] refresh 完成后调用 Query Engine。
- [ ] 删除 Watch 中对 usage/cost/totals 的直接业务计算。
- [ ] 明确 debounce/coalesce 行为。
- [ ] 文件 rewrite/truncate 继续由 Pi adapter 负责，而不是 Watch。

### 验收

- [ ] watch totals 与同一时刻普通 query totals 完全一致。
- [ ] Watch module 不 import source-specific parser/cost logic。

---

# Phase 7 — HTTP / CLI adapter 瘦身

## GO-06：统一 Go CLI 与 HTTP adapters

### CLI

- [ ] 参数解析只负责构造 use-case request。
- [ ] totals/sessions/requests/groups/period 只调用 Query Engine。
- [ ] sync/refresh 只调用 Refresh use case。
- [ ] render/serialize 不重新计算领域字段。

### HTTP

- [ ] parse params
- [ ] capability validation
- [ ] call Query/Refresh
- [ ] serialize response

### 禁止

- [ ] server 内不得重新实现 source merge。
- [ ] server 内不得解析 session/rollout。
- [ ] CLI 内不得直接 SQL 拼业务查询（统一进 query module）。

### 验收

- [ ] CLI 与 HTTP 对相同 Query Request 返回同一 domain result。
- [ ] JSON schema 在迁移期保持兼容，除非有明确 breaking decision。

---

# Phase 8 — WebUI 单一源码

## GO-07：移除 WebUI 双副本

### 目标

只有一个人工维护 HTML。

### 建议结构

```text
internal/server/web/
  index.html
```

由 Go `embed.FS` 直接嵌入。

### 工作项

- [ ] 选定 canonical WebUI 文件。
- [ ] Go server embed。
- [ ] 删除构建期 copy/sync-webui 机制。
- [ ] 删除另一份副本。
- [ ] WebUI capability 只依赖 Go `meta.sources`。
- [ ] 清理 npm-only backend 提示。

### 验收

- [ ] 仓库只有一个人工维护的 WebUI source。
- [ ] 二进制无需外部静态文件即可 serve。

---

# Phase 9 — OpenCode 后端迁移确认

## GO-08：确认 Go OpenCode 功能覆盖

### 原则

OpenCode 继续作为 benchmark/audit，不强行成为 `source=opencode`。

### 工作项

- [ ] 对照 `src/opencode/*` 与 `internal/opencode/*`：
  - [ ] auth/credentials 行为
  - [ ] workspace discovery
  - [ ] pagination
  - [ ] Seroval
  - [ ] storage
  - [ ] incremental cursor
  - [ ] lock
  - [ ] audit
  - [ ] export
- [ ] 将 TS 独有能力补到 Go。
- [ ] Web API 只调用 Go OpenCode module。

### 验收

- [ ] npm 后端删除前，OpenCode 用户可见功能无缺失。
- [ ] OpenCode 不进入 normalized usage ledger，除非另立 ADR 改变其领域定位。

---

# Phase 10 — 删除 TypeScript 后端

## GO-09：删除 Node production backend

### 前置条件

必须全部满足：

- [ ] GO-00 canonical fixtures 完成。
- [ ] GO-01 ~ GO-08 已完成。
- [ ] Go CLI/API/WebUI/OpenCode 功能覆盖确认。
- [ ] 无生产入口依赖 `src/*.ts`。

### 删除候选

- [ ] `src/aggregate.ts`
- [ ] `src/analyze.ts`
- [ ] `src/api.ts`
- [ ] `src/cli.ts`
- [ ] `src/db.ts`
- [ ] `src/db-aggregation.ts`
- [ ] `src/pi-discovery.ts`
- [ ] `src/pi-identity.ts`
- [ ] `src/pi-parse.ts`
- [ ] `src/pi-sync.ts`
- [ ] `src/render.ts`
- [ ] `src/serialize.ts`
- [ ] `src/server.ts`
- [ ] `src/session-data.ts`
- [ ] `src/source-capabilities.ts`
- [ ] `src/time-range.ts`
- [ ] `src/watch.ts`
- [ ] `src/opencode/*`
- [ ] TS production tests
- [ ] TS build config
- [ ] package scripts used only for backend

### package.json 决策

- [ ] 如果 WebUI 无 JS toolchain：删除 package.json / lockfile / tsconfig。
- [ ] 如果未来前端仍需要 Node build：package.json 仅保留 frontend dev dependencies，不再包含 backend runtime。

### 验收

- [ ] 安装/运行只需要 Go binary。
- [ ] Node 未安装时 CLI/API/WebUI 全功能可运行。
- [ ] canonical fixtures 全由 Go 测试承接。

---

# Phase 11 — 命名与发布收尾

## GO-10：统一 repository / module metadata

### 工作项

- [ ] 评估外部 Go consumers。
- [ ] 若允许 breaking module path：
  - [ ] `go.mod` 更新为 `github.com/heihei0299/pi-session-analyzer`
  - [ ] 全仓 import 更新
- [ ] README 链接更新。
- [ ] package/release metadata 删除旧 `pi-session-anylize`。
- [ ] binary 命名统一：
  - [ ] 推荐唯一 `token-analyzer`
  - [ ] 删除长期 `token-analyzer-go` 区分语义
- [ ] Release workflow 只产 Go binaries。
- [ ] 安装文档改为 Go install / GitHub Release。

### 验收

- [ ] 用户文档中不存在“Go edition / npm edition”双产品概念。
- [ ] GitHub repository、Go module、release assets、README 使用统一命名。

---

# Phase 12 — 文档收口

## GO-11：更新架构文档

- [ ] 更新 `CONTEXT.md`：
  - [ ] normalized ledger 唯一事实源
  - [ ] Go-only backend
  - [ ] Refresh / Query 分离
  - [ ] Watch 语义
- [ ] 更新 ADR-0003 后果描述，确认“不再存在 TS runtime 双轨”。
- [ ] 如 Refresh/Query lifecycle 属于长期决策，新建 ADR。
- [ ] 删除 TS/npm 后端相关操作说明。
- [ ] AGENTS.md 更新 project structure/build commands。

---

# 推荐 PR / Commit 切分

不要做一个巨型 PR。推荐：

1. `test: add canonical migration fixtures`
2. `feat(pi): route Go ingestion through normalized ledger`
3. `refactor(query): make ledger the only query source`
4. `refactor(sessiondata): remove legacy file aggregation path`
5. `refactor(sync): separate refresh from query`
6. `refactor(watch): query ledger after refresh`
7. `refactor(server): use Go query engine exclusively`
8. `refactor(webui): embed single canonical UI`
9. `refactor(opencode): complete Go parity`
10. `chore(ts): remove Node backend`
11. `chore(repo): normalize module and release naming`
12. `docs: document Go-only architecture`

每个 PR 应满足：

- 单一架构目的；
- 有独立回滚点；
- 不混入无关格式化；
- 不提前删除下一阶段需要的 parity oracle。

---

# 实施顺序依赖图

```text
GO-00 fixtures
   |
   v
GO-01 Go Pi ingest
   |
   v
GO-02 single Query Engine
   |
   +-------> GO-03 remove legacy SessionData
   |
   +-------> GO-04 Refresh != Query
                 |
                 v
              GO-05 Watch
                 |
                 v
              GO-06 adapters
                 |
        +--------+---------+
        v                  v
     GO-07 UI          GO-08 OpenCode
        \                  /
         \                /
          v              v
             GO-09 delete TS
                    |
            +-------+-------+
            v               v
         GO-10 naming    GO-11 docs
```

---

# 迁移期间的红线

- [ ] 不在 Go ledger/query 完整前删除 TS oracle。
- [ ] 不为了“统一”改变已接受统计口径。
- [ ] 不把 Query 和 Refresh 再合并。
- [ ] 不在 Watch 重写 usage parser。
- [ ] 不让 HTTP/CLI adapter 直接实现 SQL 业务规则。
- [ ] 不通过新增兼容分支长期保留双后端。
- [ ] 不把 OpenCode 无依据地纳入 source union。
- [ ] 不在同一 PR 同时做大规模 rename + 行为重构。
- [ ] 不提交真实 session log、数据库、token、cookie、.env。

---

# 验证矩阵

> 按仓库现行规则，build/compile/test/lint/typecheck 需要明确授权；实际执行 Ticket 时先取得对应授权。

每个生产行为迁移至少需要：

| 维度 | 验证 |
|---|---|
| Pi ingestion | canonical fixtures + idempotent refresh |
| Codex ingestion | canonical fixtures + diagnostics |
| Query | totals/sessions/groups/period/detail/meta golden |
| All source | Pi + Codex 合计与 costStatus |
| Time | session/message 两套 TimeRange |
| Fork | copied history 去重 |
| Incremental | append/partial/rewrite/truncate |
| HTTP | 与 query domain result 一致 |
| CLI | 与 query domain result 一致 |
| Watch | refresh 后与普通 query 一致 |
| WebUI | capabilities/unsupported actions 正确 |
| OpenCode | sync/export/audit parity |

---

# 最终删除测试

迁移完成后逐项执行 deletion test：

1. 删除全部 TS backend 后，生产功能是否完整？
2. 删除 `SessionData.AnalyzeFile` 后，是否没有其他 module 被迫重新实现 parser？
3. 删除 Watch usage aggregation 后，Watch 是否仍完整工作？
4. 删除 WebUI 副本后，是否没有新的 copy script？
5. Query module 是否可以在没有 Pi/Codex 文件系统访问权限的测试环境下，仅凭 DB fixture 完整测试？

若答案全部为“是”，说明 module 的 seam 已经收口正确。

---

# 第一批可执行工作

建议迁移分支上的第一批只做两件事：

## Batch A — GO-00

- canonical fixture matrix；
- parity 从“长度比较”升级到完整结果比较；
- 不改产品行为。

## Batch B — GO-01

- Go Pi ingestion 覆盖 ADR-0003；
- 增加 ledger-backed Pi query 的最小 vertical slice：先 totals；
- totals 验证稳定后再依次迁 sessions/requests/groups/period/detail/meta。

不要第一批就删除 TS。

**迁移原则：先建立单一事实中心，再删除重复实现。**
