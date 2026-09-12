# 07: 收尾 standalone OpenCode contract、CI 与文档一致性

**What to build:** 完成 `opencode-analyzer/` 作为独立项目边界的最后收口：让 standalone Pi audit 的 semantic dedup 与 token-analyzer canonical 语义一致、让独立 Go module 真正进入 CI，并清理 tracker / README / AGENTS 等迁移后文档状态。

**Blocked by:** 06: 收尾 storage lifecycle 与 runtime 变更检测.

**Status:** ready-for-agent

- [ ] `opencode-analyzer/internal/piaudit` 保持完全独立，不 import token-analyzer `internal/*`，但 semantic identity 的 canonical field selection 与 token-analyzer Pi identity 语义一致。
- [ ] no-entry-id semantic dedup 只由 canonical billing/message fields 决定；新增与计费无关的 message metadata 不得制造新的 semantic request。
- [ ] standalone audit contract 增加 regression：两条记录 canonical fields 相同、仅 irrelevant metadata 不同，最终只能计一次。
- [ ] 继续覆盖 assistant / toolResult / compaction / branch_summary、billable/cost/failed gate、fork copied-history 去重、requestId replacement、月份/时间范围与 totalTokens 语义。
- [ ] OpenCode fixture 保持自包含，不通过相对路径复用 token-analyzer testdata，确保未来整目录拆仓时测试仍可直接运行。
- [ ] 主仓 CI 增加独立 `opencode-analyzer` job/step，在该 module 内执行 `GOMAXPROCS=2 go test -p 1 ./...`；root `go test ./...` 不能作为 standalone module 已验证的替代。
- [ ] CI 失败应能明确区分 token-analyzer 与 opencode-analyzer，避免一个独立项目的测试结果被主模块输出掩盖。
- [ ] 复查 `.scratch/go-only-backend-migration/issues/03-*`、`04-*`、`05-*`：已验证的 acceptance 项改为 `[x]`；无法证明的项不得因 `Status: resolved` 被伪装为完成。
- [ ] 修复 `AGENTS.md` 中 `\仅当关键歧义...` 的文本错误。
- [ ] README 的 Pi cost 描述与真实实现一致：上报 `usage.cost.total` 优先，缺失时允许 `model_pricing` fallback；两者都不可用才是零/未定价语义。
- [ ] 删除或更新已失效的迁移/审计说明，避免 README 把历史 remediation 文档描述成当前执行入口。
- [ ] 最终确认 `opencode-analyzer/` 可整目录迁出：自身 module、tests、README、storage/API/UI 不依赖 token-analyzer runtime 或私有源码。

## Acceptance focus

```text
token-analyzer CI
  -> root Go module tests

opencode-analyzer CI
  -> standalone module tests
  -> standalone Pi audit canonical contract

repository docs/tracker
  -> status / checklist / runtime behavior一致
```

完成后，OpenCode Analyzer 的独立边界不仅在目录结构上成立，也在 semantic contract 与 CI 上成立；仓库中的 tracker、README、AGENTS 与最终实现状态一致。
