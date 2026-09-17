# 08: 清理 Query capability 的不可达实现

**What to build:** 删除已经确认没有调用方的 Query 实现和无效局部状态，同时保持 Codex/All 不支持 requests 的公开 capability 契约不变。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] 未调用的 Codex request-row builder 和无效局部状态被删除。
- [x] Codex/All requests 仍在 Validate、CLI、HTTP 和 WebUI 边界返回明确 unsupported，不回落到隐式空结果。
- [x] Pi requests、sessions、groups、period、detail 和 All aggregate 的现有 QueryResult 结构不改变。
- [x] canonical Query contract 和 unsupported capability 回归测试继续通过。
- [x] 不建立第二套 Query/aggregation 路径，不引入 factory、ORM、framework 或 runtime dependency。
- [x] 变更保持局部，删除后没有当前仓库内部调用方或编译依赖残留。

## Comments

- Source spec: `.scratch/token-analyzer-maintainability-review/spec.md`
- Triage: ready-for-agent

---

## Completion note

- 修改摘要：删除不可达的 `buildCodexRequestRows` 与无效 `byID` 状态；Codex/All requests 的 WebUI export 改为显式 unsupported，保留 Validate/CLI/HTTP/Query capability 契约。
- 验证：`TOKEN_ANALYZER_DB= go test ./internal/query ./internal/server`，40 个测试通过；`rg` 确认无残留 builder、状态和空结果回退。
- Review：完整 Standards/Spec 双轴 Review 已通过；WebUI unsupported 行为已增量复核关闭。
- Commit：`ce116df refactor(query): remove unreachable request builder`。
- 未解决边界问题：无。
