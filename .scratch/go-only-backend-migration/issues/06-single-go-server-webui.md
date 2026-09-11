# 06: 用单一 Go server 交付完整 Pi/Codex WebUI

**What to build:** 用户启动一个 Go binary 即可使用 Pi、Codex、All 的完整受支持 WebUI；UI 只有一个人工维护源，全部 API 由 Go 提供，没有 Node server、内嵌 OpenCode 页面或双副本同步；OpenCode UI 仅存在于独立 `opencode-analyzer/` 项目。

**Blocked by:** 02: 将 OpenCode 抽离为 standalone opencode-analyzer 项目; 03: Pi 全窗口切到 Go ledger + Query Engine; 04: Codex / All 切到同一个 ledger-native Query Engine; 05: 收口 Refresh / Query / Watch 生命周期.

**Status:** ready-for-agent

- [ ] Go binary 无需 Node server 即可提供首页与全部 Pi/Codex 生产 API。
- [ ] 仓库只保留一个人工维护 WebUI 源，不需要 copy/sync 两份 HTML。
- [ ] source selector 只声明 Pi/Codex/All 的真实能力。
- [ ] Pi detail/subagent/rename 保持现有行为。
- [ ] Codex/All 不支持的 requests/detail/rename 入口明确禁用并解释原因。
- [ ] WebUI 自动刷新只读取 snapshot，不因每个请求触发 source sync。
- [ ] token-analyzer WebUI 与 HTTP API 不包含 OpenCode 专用交互或路由；相关能力只属于 `opencode-analyzer/`。
- [ ] 没有 Node runtime 的环境中，Go binary 可以独立 serve 可用面板。
