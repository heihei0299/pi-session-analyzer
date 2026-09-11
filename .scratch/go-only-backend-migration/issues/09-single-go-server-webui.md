# 09: 用单一 Go server 交付完整 WebUI

**What to build:** 用户启动一个 Go binary 就能使用完整 WebUI，包括 Pi、Codex、All 与 OpenCode 已支持的视图和交互；WebUI 只有一个人工维护源，并由 Go server 内嵌提供，不再依赖 TypeScript server 或双副本同步。

**Blocked by:** 03: Pi 会话、请求与详情窗口切到 Go ledger; 04: Pi 分组、周期、时间范围与 meta 切到 Go ledger; 05: Codex 与 All 改为 ledger-native 查询; 06: 将 Refresh 生命周期与 Query 彻底分离; 08: OpenCode 对账能力完成 Go parity.

**Status:** ready-for-agent

- [ ] Go binary 无需外部 Node server 即可提供首页与全部生产 API。
- [ ] 仓库只保留一个人工维护的 WebUI 源，不再需要构建期复制/同步两份 HTML。
- [ ] source selector 只根据 Go backend capabilities 渲染，Pi/Codex/All 的可用性诚实。
- [ ] Pi 会话详情、子代理展示和重命名在 Go server 下保持现有用户行为。
- [ ] Codex/All 不支持的 requests/detail/rename 入口被禁用并给出原因。
- [ ] OpenCode audit tab 只调用 Go HTTP 后端并保持现有用户功能。
- [ ] WebUI 自动刷新读取 snapshot，不因每个前端请求触发 source sync。
- [ ] 单个 Go binary 在没有 Node runtime 的环境中可以 serve 可用面板。
