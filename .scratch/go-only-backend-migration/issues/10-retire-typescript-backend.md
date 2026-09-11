# 10: Contract TypeScript 后端并移除 Node 生产运行时

**What to build:** 在所有生产能力已经由 Go vertical slices 覆盖后，用户只剩一个 Go 后端产品；旧 TypeScript CLI/API/server/db/session/watch/OpenCode 生产实现及其专属构建链被删除，而 canonical behavior tests 继续独立保护 Go。

**Blocked by:** 07: Watch 改为变化检测 → Refresh → Query; 09: 用单一 Go server 交付完整 WebUI.

**Status:** ready-for-agent

- [ ] 所有用户可见 CLI、HTTP、WebUI、Watch 与 OpenCode 功能都有 Go acceptance 证明后，才删除对应 TypeScript 生产实现。
- [ ] 删除后不存在第二套 Pi parser、DB aggregation、SessionData、Watch aggregation、server 或 OpenCode backend。
- [ ] TypeScript parity oracle 被 canonical golden tests 替代；长期测试不需要已删除 runtime 才能判断正确性。
- [ ] 运行 token-analyzer CLI/API/WebUI 不再要求 Node/npm。
- [ ] 如果前端没有独立 Node 构建需要，则移除后端遗留的 package/toolchain；如果仍有前端工具链，则只保留前端职责。
- [ ] 删除旧后端后，Go canonical fixtures 的所有 acceptance 结果保持不变。
- [ ] 文档和错误信息中不再把 npm/TS backend 描述为可选生产实现。
