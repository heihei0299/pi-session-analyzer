# 01: 真实命名可发现（Codex rollout 发现契约）

**What to build:** 用户在本机真实 Codex home（`rollout-<秒级 UTC 时间戳>-<thread id>` 命名，位于按年/月/日分目录下，可含 revert 的 `_<rollout id>` 形态、`.jsonl.zst` 压缩表示，以及 `archived_sessions` 归档根）上运行 `--source codex`，或在 WebUI 切到 Codex 时，能看到真实会话与 totals / sessions / meta，而不是空窗口加一片「跳过非 canonical」噪声。本 ticket 只保证「能发现、能读到」，token 数字口径由 02 修正。

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [ ] 发现阶段按显式 grammar 解析 canonical 文件名（前缀 + 秒级 UTC 时间戳 + thread id，可选 `_<rollout id>`，可选压缩后缀），不再要求时间戳之后紧跟字面量 `Z`；带 `Z` 的历史形态仍被接受，非 canonical 文件仍被跳过并累计诊断。
- [ ] 真实同形 synthetic fixture 落地，并成为后续 ticket 的验收基线：plain 与压缩 sibling、`sessions` 与 `archived_sessions`、按日分目录、revert 形态、非 canonical 干扰文件。
- [ ] 在真实同形 fixture 上，`--source codex` 的 totals / sessions / meta 非空，sessions 行带 source，Codex 成本仍为 `unpriced`。
- [ ] plain 与压缩 sibling 只计一次；表示切换（plain ↔ zstd）后的重扫保持幂等，不双算、不删除历史账本行。
- [ ] 既有幂等回归（重扫 / 追加 / 截断 / 替换 / 半行 / 坏行不阻塞合法行）在真实同形 fixture 上仍为绿。
- [ ] 窄单测覆盖命名接受集合：真实命名、revert 形态、按日目录、压缩后缀、带 `Z` 的旧形态被接受；非 canonical 命名被拒绝并计入诊断。
- [ ] `meta.warnings` 能说明被跳过的文件，空 Codex 目录只产生警告、不作为服务错误。
