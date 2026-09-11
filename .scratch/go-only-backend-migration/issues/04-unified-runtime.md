# 04: 收口 Refresh / Query / Watch / Go server / WebUI 运行时

**What to build:** 用户启动一个 Go binary 就能使用完整的 Pi/Codex/All token-analyzer。Query 只读取已提交 snapshot；Refresh 统一负责源同步；Watch 只做 change → refresh → query；全部 HTTP API 与 WebUI 由 Go server 提供，并只保留一个人工维护的 WebUI 源。

**Blocked by:** 03: Pi / Codex / All 全部切到统一 Go ledger + Query Engine.

**Status:** ready-for-agent

- [ ] Query 不执行 discovery、parse、sync、游标推进或 DB 写入。
- [ ] Go server 启动时完成明确的初次 Pi/Codex refresh，再对外提供 snapshot 查询。
- [ ] 后续 refresh 走统一 orchestration，并发 refresh 被合并或串行化，多个 GET 不放大为重复同步。
- [ ] refresh 失败保留上一成功 snapshot，并通过 diagnostics/meta 暴露错误。
- [ ] 连续 GET 不改变 ledger 内容或同步游标。
- [ ] Watch 改为 change → refresh → query，不再直接累加 usage/cost/totals。
- [ ] append、partial line、truncate、rewrite、fork、cache、pricing 等 source 规则只存在于 source adapter，不在 Watch 重复实现。
- [ ] Watch totals 与同一时刻普通 Query totals 完全一致。
- [ ] Go binary 无需 Node server 即可提供全部 Pi/Codex 生产 HTTP API 和 WebUI。
- [ ] WebUI 只保留一个人工维护源码，不再依赖两份 HTML 的 copy/sync。
- [ ] source selector 只声明 Pi/Codex/All 的真实 capability。
- [ ] Pi detail/subagent/rename 保持现有行为；Codex/All 不支持的 requests/detail/rename 明确禁用并解释原因。
- [ ] WebUI 自动刷新只读取 snapshot，不因每个前端请求触发 source sync。
- [ ] token-analyzer WebUI/API 不包含 OpenCode 专用交互或路由；OpenCode UI/API 仅属于独立 `opencode-analyzer/`。
- [ ] 没有 Node runtime、没有 `opencode-analyzer/` 目录的环境中，Go binary 仍能独立运行完整 token-analyzer。
