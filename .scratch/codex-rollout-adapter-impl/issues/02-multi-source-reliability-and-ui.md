# 02: 多 source 查询体验与可靠性闭环

**What to build:** 用户可以稳定地使用 `pi`、`codex` 或显式 `all` 查询；重复同步、文件追加、截断、替换、plain/zstd 切换、坏行和冲突 payload 都不会漏算或双算。CLI、HTTP API 和 WebUI 会显示 source、合计结果及可诊断 warnings，并明确拒绝 Codex requests 查询。

**Blocked by:** 01: Go normalized ledger 与 Codex rollout 导入

**Status:** resolved

## Acceptance criteria

- [x] `--source pi|codex|all` 和 `--codex-dir` 行为完成，默认 Pi 行为保持兼容。
- [x] `--codex-dir > CODEX_HOME > ~/.codex` 的目录优先级生效，`--dir` 仍只表示 Pi 目录。
- [x] 重复执行、追加、半行、截断、替换和 revision 变化均符合全量重扫 + response identity 幂等规则。
- [x] plain 与 `.jsonl.zst` 表示切换不会重复计入，也不会修改原始 Codex rollout。
- [x] 坏行、未知 event、空目录和冲突 payload 返回部分结果并产生可读 diagnostics；不会静默吞错。
- [x] `all` totals 正确合计 Pi/Codex，sessions 行带 source，meta 能说明参与统计的 sources。
- [x] Codex 或 `all` 的 requests 查询返回明确 unsupported error，不返回仅 Pi 的伪合计。
- [x] WebUI 复用现有总览、分组和 sessions 页面，提供 `Pi / Codex / All` source selector；Codex cost 显示 unavailable。
- [x] 添加 API/UI 集成测试和增量同步回归测试。

## Implementation summary

- Added WebUI `Pi / Codex / All` source selector with source-aware refresh, metadata inference, and explicit requests disablement.
- Exposed CLI/API diagnostics warnings and preserved current Codex-directory physical rollout isolation across repeated syncs.
- Added partial-result diagnostics coverage for malformed/unknown events and UI/API integration coverage.
