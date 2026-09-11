# Go-only 后端迁移执行文档

> Branch: `migration/go-backend-only-20260912`  
> 规范入口：`.scratch/go-only-backend-migration/spec.md`  
> Ticket tracker：`.scratch/go-only-backend-migration/issues/`

## 决策

- 当前项目只负责 Pi / Codex token usage。
- OpenCode 不迁移进 Go 核心；现有 OpenCode 集成从 token-analyzer 解耦。
- Go 是唯一生产后端。
- normalized SQLite ledger 是唯一 usage 事实中心。
- Query 与 Refresh 分离。
- Watch 只触发 Refresh 并查询 snapshot。
- TypeScript 只作为迁移期 oracle，最终删除。

## 可执行 Ticket 图

```text
01 extract OpenCode       02 canonical contract
        |
        v
02 canonical contract
        |
        +----> 03 Pi Go ledger
        |
        +----> 04 Codex/All ledger
                    \                /
                     \              /
                       03 + 04
                          |
                          v
                  05 Refresh/Query/Watch
                          |
                          +------ 02
                          |
                          v
                  06 single Go server/WebUI
                          |
                          v
                  07 retire TS + release/docs
```

### Frontier

可立即执行：

- **01** 将 OpenCode 抽离为独立 `opencode-analyzer/` 项目

完成 01 后执行 02；完成 02 后，03 与 04 可并行。

## Ticket 摘要

### 01 — OpenCode standalone extraction
把现有 OpenCode 能力完整迁入仓库根下独立 `opencode-analyzer/`，保留功能并建立未来可整目录拆仓的边界。

### 02 — Canonical contract
建立 synthetic fixtures + golden expected，冻结 Pi/Codex 已接受行为。

### 03 — Pi Go ledger/query
Pi 所有窗口统一走 Refresh → ledger → Query，旧文件扫描 aggregate 退出生产。

### 04 — Codex/All ledger-native query
Codex/All 直接查询 ledger，移除 SessionFileData 回绕。

### 05 — Refresh/Query/Watch lifecycle
Query 只读；Refresh 负责写；Watch 只 change → refresh → query。

### 06 — Single Go server/WebUI
单 Go binary、单 WebUI 源、Pi/Codex/All 完整受支持能力，无 OpenCode UI/API。

### 07 — Contract TS + release/docs
删除 TS backend 与旧统计路径，统一 module/release/docs/ADR/CONTEXT。

## 迁移红线

- 不在 canonical contract 建立前大规模替换统计路径。
- 不改变已接受 Pi/Codex usage 口径来“适配迁移”。
- 不让 Query 再承担 Sync。
- 不在 Watch 复制 parser/token math。
- 不保留长期双后端。
- 不把 OpenCode 重新加入 source union、HTTP API 或 WebUI。
- 不提交真实 session、rollout、DB、cookie、token、.env。
- build/test/lint/typecheck 按仓库规则仅在明确授权后执行。

## Definition of Done

- [ ] Go 是唯一生产后端。
- [ ] Pi/Codex 都通过 normalized ledger。
- [ ] 所有统计窗口只通过单一 Query Engine。
- [ ] Query 无写副作用。
- [ ] Watch 无独立 usage aggregation。
- [ ] 单一 WebUI 源由 Go binary 提供。
- [ ] TypeScript backend 已删除。
- [ ] OpenCode 已迁入独立 `opencode-analyzer/` 项目边界，token-analyzer 与其无运行时耦合。
- [ ] Node 不再是运行 token-analyzer 的必要条件。
- [ ] canonical fixtures 独立保护最终 Go 行为。
- [ ] module/repository/release/docs 与最终边界一致。
