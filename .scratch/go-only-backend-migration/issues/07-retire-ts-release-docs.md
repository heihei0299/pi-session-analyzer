# 07: 删除 TypeScript 后端并完成 Go-only 发布收尾

**What to build:** 当 Pi/Codex 全部生产能力已由 Go 覆盖后，删除 TypeScript/Node 后端与旧 oracle，统一 module/repository/release/README/CONTEXT/ADR，使用户只看到一个边界清晰的 Go-only token-analyzer；OpenCode 仅作为同仓独立 `opencode-analyzer/` 项目存在。

**Blocked by:** 06: 用单一 Go server 交付完整 Pi/Codex WebUI.

**Status:** ready-for-agent

- [ ] 在 Go acceptance 覆盖全部用户可见能力后，删除 TypeScript CLI/API/server/db/session/watch 等生产实现。
- [ ] 删除旧 SessionData 文件扫描/内存 aggregation 生产路径。
- [ ] canonical golden tests 独立承担长期回归，不依赖已删除 runtime。
- [ ] 运行 CLI/API/WebUI 不要求 Node/npm；若仍保留前端工具链，其职责不得包含 backend runtime。
- [ ] repository/module/import/release metadata 统一使用当前项目名称。
- [ ] release 只发布 Go-only 产品所需产物，文档不再区分 Go/npm edition。
- [ ] CONTEXT/ADR 更新为 ledger 唯一事实中心、Go-only、Refresh/Query 分离、Watch 新语义，以及 OpenCode 已抽离为独立项目边界。
- [ ] 最终删除测试成立：无 TS backend、无旧 SessionData 生产 aggregate、无 Watch 独立统计、无 WebUI 双副本、token-analyzer 无 OpenCode runtime/API/UI 耦合，`opencode-analyzer/` 可整目录迁出。
