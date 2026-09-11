# 08: OpenCode 对账能力完成 Go parity

**What to build:** OpenCode 对账用户可以在不依赖 TypeScript 后端的情况下完成 workspace 发现、增量同步、成本历史、使用明细、导出和 WebUI audit；OpenCode 继续作为外部 benchmark/audit，而不是被混入 Pi/Codex usage source。

**Blocked by:** 01: 冻结 Go-only 迁移的 canonical 行为契约.

**Status:** ready-for-agent

- [ ] Go 后端覆盖现有 OpenCode auth/credential 注入与 workspace 自动发现行为。
- [ ] Go 后端覆盖使用历史分页、增量游标、去重、并发/文件锁与本地存储行为。
- [ ] Go CLI 的 OpenCode sync/export 对用户可见结果与现有行为一致。
- [ ] Go HTTP API 可以提供 WebUI OpenCode audit 所需的数据与同步动作。
- [ ] WebUI 的月度成本、使用历史与 Pi 对账视图在 Go-only 后端下完整可用。
- [ ] OpenCode 数据不写入 Pi/Codex normalized usage ledger，也不进入 source=all。
- [ ] synthetic fixtures/协议样本足以覆盖关键 Seroval/RPC 解析，不依赖真实 cookie 或云端账号进入 CI。
