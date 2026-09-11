# 02: 从 token-analyzer 移除 OpenCode 耦合

**What to build:** token-analyzer 不再承担 OpenCode 云端对账客户端职责；用户使用当前项目时不需要 OpenCode cookie、workspace、RPC、Seroval、同步、本地 storage 或 audit UI，Pi/Codex usage 功能不受影响。

**Blocked by:** None (can start immediately).

**Status:** ready-for-agent

- [ ] CLI 不再暴露 OpenCode 专用 sync/export/audit 命令或参数。
- [ ] HTTP API 不再暴露 OpenCode 专用同步、成本、usage history 或 audit 端点。
- [ ] WebUI 不再包含 OpenCode 对账 tab、同步按钮或 OpenCode 专属状态。
- [ ] 项目不再读取、保存或要求 OpenCode cookie/credential/workspace 配置。
- [ ] OpenCode RPC/Seroval client、storage、cursor、lock 等专用生产实现从当前项目移除。
- [ ] OpenCode 不进入 normalized ledger、meta.sources 或 source=all。
- [ ] 删除 OpenCode 集成后，Pi/Codex canonical 行为不发生变化。
- [ ] 文档明确 OpenCode 已不属于本项目；未来若需要，应由独立工具/插件承载。
