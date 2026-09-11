# Go-only 后端迁移架构审计报告

> 审计基线：`main@05edcfc702a5439dc363d41724a5d23732b5cc9a`  
> 更新日期：2026-09-12  
> 范围：Pi/Codex usage 后端架构、统计口径、Query/Refresh/Watch、WebUI、TypeScript 退役与产品边界。  
> 本报告为静态审计；未执行 build / test / lint / typecheck。

## 1. 审计结论

当前最主要问题是**同一 Pi/Codex token usage 领域存在多个事实路径和多个统计执行引擎**：

- Go Pi 仍存在文件扫描 + SessionData 内存聚合；
- TypeScript Pi 已主要走 SQLite + SQL aggregation；
- Go Codex 会先写 SQLite，再转换回会话内存模型聚合；
- Watch 维护独立 totals 增量状态；
- WebUI 存在双副本；
- Query 与 Sync 职责未完全分离。

此外，当前仓库还内嵌 OpenCode RPC/Seroval/凭据/同步/storage/audit/UI。该能力与“分析本机 Pi/Codex token usage”的核心域不同，会扩大安全、依赖、API、UI 与发布边界。

**Recommendation: Strong — Go-only + OpenCode decoupling。**

目标：

```text
Pi --------┐
           ├─> Source Refresh ─> Normalized SQLite Ledger ─> Query Engine
Codex -----┘                                              ├─> CLI
                                                         ├─> HTTP
                                                         ├─> Watch
                                                         └─> WebUI
```

OpenCode 不在该图中。

## 2. 核心架构风险

| 优先级 | 发现 | 影响 |
|---|---|---|
| P0 | Go Pi 普通查询可绕过 normalized ledger | 同一数据可能不同口径 |
| P0 | Go/TS 重复实现 usage 领域规则 | 每次规则变更存在 runtime 漂移 |
| P0 | parity 对部分窗口只比较行数 | 字段级错误可保持测试绿色 |
| P1 | Codex Query 含同步写副作用 | GET 延迟、副作用与并发不可预测 |
| P1 | Codex ledger 后再回绕 SessionFileData | ledger 未成为真正 query seam |
| P1 | Watch 独立计算 totals | 第三套统计语义 |
| P1 | WebUI 双副本 | UI 漂移与构建复杂度 |
| P1 | OpenCode 内嵌当前仓库 | 产品域、安全与运行依赖不必要扩大 |
| P2 | module/repository 旧拼写 | 发布/import/navigation 噪声 |

## 3. 目标边界

### token-analyzer 负责

- Pi session discovery / parse / identity / dedup / incremental refresh；
- Codex rollout discovery / parse / identity / diagnostics / refresh；
- normalized SQLite ledger；
- totals / sessions / requests / groups / period / detail / meta；
- CLI / HTTP / Watch / WebUI。

### token-analyzer 不负责

- OpenCode 登录 cookie/凭据；
- OpenCode workspace discovery；
- OpenCode RPC/Seroval；
- OpenCode 云端 usage/cost 同步；
- OpenCode 本地 storage/cursor/lock；
- OpenCode audit/export；
- OpenCode WebUI 对账页面。

如果未来需要 OpenCode 能力，应独立成工具/插件/repo，再通过明确公开接口与 token-analyzer 交互。

## 4. 最重要的实现收口

### Pi

普通查询必须完成：

```text
Pi source
  -> Refresh
  -> normalized ledger
  -> Query Engine
```

不得继续增强旧 SessionData 文件扫描统计。

### Codex

取消：

```text
rollout -> SQLite -> SessionFileData -> memory aggregate
```

改为：

```text
rollout -> Refresh -> normalized ledger -> Query Engine
```

### Query / Refresh

Query 只读。Refresh 才负责 discovery/parse/dedup/write/diagnostics。

### Watch

Watch 只做：

```text
change -> Refresh -> Query
```

不再理解四载体、cache、pricing、fork 或 token math。

### WebUI

只保留一个人工维护源，由 Go binary 内嵌；不包含 OpenCode 专用页面。

## 5. 测试策略

最高 seam：

```text
synthetic source fixture
  -> Refresh
  -> ledger
  -> Query
  -> CLI/API/WebUI observable result
```

必须用 canonical golden contract 覆盖 Pi 四载体、fork/dedup/incremental/time 与 Codex cache/diagnostics/zstd/revert。

OpenCode 解耦的验收不是 parity，而是：

- 没有 OpenCode 配置仍完整运行；
- CLI/API/UI 不暴露 OpenCode；
- 发布产物不依赖 OpenCode；
- 删除 OpenCode 后 Pi/Codex contract 不变。

## 6. 终局判据

迁移成功不只是“没有 TypeScript”，而是同时满足：

1. usage 统计规则只有一个生产实现；
2. source-specific 语义止于 Refresh adapter；
3. normalized ledger 是唯一事实中心；
4. Query 完全只读；
5. Watch 不计算 usage；
6. WebUI 单源码、Go 内嵌；
7. OpenCode 与 token-analyzer 产品边界彻底分离。
