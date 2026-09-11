# 11: 收口 Go-only 命名、发布与领域文档

**What to build:** 用户从 README、安装命令、release assets、module metadata 和领域文档中只看到一个一致的 token-analyzer Go 产品；旧仓库拼写与 Go/npm 双版本概念退出，CONTEXT 与 ADR 准确描述最终的 ledger / Refresh / Query / Watch 架构。

**Blocked by:** 10: Contract TypeScript 后端并移除 Node 生产运行时.

**Status:** ready-for-agent

- [ ] repository/module/import/release metadata 统一使用当前项目名称，并明确处理任何 breaking module path 影响。
- [ ] release 流程只发布 Go-only 产品所需产物，binary 命名不再需要 Go/TS edition 区分。
- [ ] 安装与运行文档以 Go binary / release 为唯一生产路径。
- [ ] 领域术语表更新为 normalized ledger 唯一事实中心、Go-only backend、Refresh/Query 分离与 Watch 新语义。
- [ ] 既有存储 ADR 的后果描述更新为最终单运行时现实；如 Refresh/Query lifecycle 需要长期约束，则记录新的架构决策。
- [ ] 项目开发指南中的结构、验证命令与运行方式不再引用已删除 TypeScript 后端。
- [ ] 最终删除测试成立：无 TypeScript backend、无旧 SessionData 扫描聚合生产路径、无 Watch 独立统计、无 WebUI 双副本。
