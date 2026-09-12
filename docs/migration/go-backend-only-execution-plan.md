# Go-only 后端迁移执行文档

> Branch: `migration/go-backend-only-20260912`  
> 规范入口：`.scratch/go-only-backend-migration/spec.md`  
> Ticket tracker：`.scratch/go-only-backend-migration/issues/`

## 决策

- 当前 token-analyzer 只负责 Pi / Codex token usage。
- OpenCode 先抽离到仓库根下独立 `opencode-analyzer/`，保留现有能力，为未来整目录拆仓上线做准备。
- Go 是 token-analyzer 唯一生产后端。
- normalized SQLite ledger 是唯一 usage 事实中心。
- Query 与 Refresh 分离。
- Watch 只触发 Refresh 并查询 snapshot。
- TypeScript 只作为迁移期 oracle，最终删除。

## 最终 5 Tickets

```text
01 OpenCode standalone extraction
              |
              v
02 canonical Pi/Codex contract
              |
              v
03 unified Go ledger + Query
              |
              v
04 unified runtime
   Refresh / Query / Watch
     Go server / WebUI
              |
              v
05 retire TS + release/docs
```

### Frontier

已完成：

- **01 — 将 OpenCode 抽离为 standalone `opencode-analyzer/` 项目**

当前可立即执行：

- **02 — 建立 canonical Pi/Codex contract**

后续按依赖顺序执行 02 → 03 → 04 → 05。

## Ticket 摘要

### 01 — OpenCode standalone extraction

把现有 OpenCode RPC、Seroval、credential、sync、storage、CLI/API/WebUI audit 等能力完整迁入仓库根下独立 `opencode-analyzer/`。保留功能，但切断与 token-analyzer 私有实现和运行时的双向依赖。

### 02 — Canonical contract

建立 synthetic fixtures + golden expected，冻结 Pi/Codex 已接受的四载体、fork/dedup/incremental、time、cache、diagnostics 等行为，为后续架构重构提供统一验收基线。

### 03 — Unified Go ledger + Query Engine

Pi、Codex、All 一次性收敛到：

```text
source Refresh
      ↓
normalized SQLite ledger
      ↓
single Go Query Engine
```

移除 Pi 文件扫描统计、Codex SessionFileData 回绕以及独立 All merge 统计路径。

### 04 — Unified runtime

一次性收口：

```text
Refresh / Query
Watch
Go HTTP server
WebUI
```

Query 只读；Watch = change → refresh → query；Go binary 独立提供 API 和单一 WebUI 源。

### 05 — Contract TypeScript + release/docs

在 Go 已完整覆盖生产行为后：

- 删除 TS/Node backend；
- 删除迁移期 parity oracle；
- 删除旧 SessionData 生产统计路径与双 WebUI；
- 统一 module/repository/release naming；
- 更新 README / CONTEXT / ADR；
- canonical golden tests 成为长期行为契约。

## 迁移红线

- 第一票先完成 OpenCode 产品边界拆分，不在后续 Go-only 重构中继续携带 OpenCode 耦合。
- 不在 canonical contract 建立前大规模替换 Pi/Codex 统计路径。
- 不为了迁移改变已接受 usage 口径。
- 不让 Query 承担 Sync。
- 不在 Watch 复制 parser/token math。
- 不长期保留双后端。
- token-analyzer 不重新依赖 `opencode-analyzer/`。
- OpenCode 不进入 token-analyzer normalized ledger、source union、API 或 WebUI。
- 不提交真实 session、rollout、DB、cookie、token、.env。
- build/test/lint/typecheck 按仓库规则仅在明确授权后执行。

## Definition of Done

- [ ] OpenCode 已完整迁入独立 `opencode-analyzer/` 项目边界，现有功能保留且可未来整目录拆仓。
- [ ] token-analyzer 与 `opencode-analyzer/` 无双向运行时/private-code 耦合。
- [ ] Go 是 token-analyzer 唯一生产后端。
- [ ] Pi/Codex 都通过 normalized ledger。
- [ ] Pi/Codex/All 所有统计窗口只通过单一 Query Engine。
- [ ] Query 无写副作用。
- [ ] Watch 无独立 usage aggregation。
- [ ] 单一 WebUI 源由 Go binary 提供。
- [ ] TypeScript backend 已删除。
- [ ] Node 不再是运行 token-analyzer 的必要条件。
- [ ] canonical fixtures 独立保护最终 Go 行为。
- [ ] module/repository/release/docs 与最终产品边界一致。
