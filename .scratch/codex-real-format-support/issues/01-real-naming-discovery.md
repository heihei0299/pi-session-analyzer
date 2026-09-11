# 01: 真实命名可发现（Codex rollout 发现契约）

**What to build:** 用户在本机真实 Codex home（`rollout-<秒级时间戳>-<thread id>` 命名，位于按年/月/日分目录下，可含 revert 的 `_<rollout id>` 形态、`.jsonl.zst` 压缩表示，以及 `archived_sessions` 归档根）上运行 `--source codex`，或在 WebUI 切到 Codex 时，能看到真实会话与 totals / sessions / meta，而不是空窗口加一片「跳过非 canonical」噪声。本 ticket 只保证「能发现、能读到」，token 数字口径由 02 修正。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] 发现阶段按显式 grammar 解析 canonical 文件名（前缀 + 秒级时间戳 + thread id，可选 `_<rollout id>`，可选压缩后缀），不再要求时间戳之后紧跟字面量 `Z`；带 `Z` 的历史形态仍被接受，非 canonical 文件仍被跳过并累计诊断。
- [x] 真实同形 synthetic fixture 落地，并成为后续 ticket 的验收基线：plain 与压缩 sibling、`sessions` 与 `archived_sessions`、按日分目录、revert 形态、非 canonical 干扰文件。
- [x] 在真实同形 fixture 上，`--source codex` 的 totals / sessions / meta 非空，sessions 行带 source，Codex 成本仍为 `unpriced`。
- [x] plain 与压缩 sibling 只计一次；表示切换（plain ↔ zstd）后的重扫保持幂等，不双算、不删除历史账本行。
- [x] 既有幂等回归（重扫 / 追加 / 截断 / 替换 / 半行 / 坏行不阻塞合法行）在真实同形 fixture 上仍为绿。
- [x] 窄单测覆盖命名接受集合：真实命名、revert 形态、按日目录、压缩后缀、带 `Z` 的旧形态被接受；非 canonical 命名被拒绝并计入诊断。
- [x] `meta.warnings` 能说明被跳过的文件，空 Codex 目录只产生警告、不作为服务错误。

## Implementation summary

- 命名判定从正则猜测改为显式 grammar 解析：前缀 + 秒级时间戳（`2006-01-02T15-04-05`，秒后容忍历史形态的可选 `Z`）+ thread id，可选 `_<rollout id>`（至多一个 `_` 分段），可选 `.zst`；时间戳按日历校验，非法时间戳、缺 id、未支持的压缩后缀一律判非 canonical。
- 「rollout 产物」判定与命名判定分离：`rollout-` 前缀或 `.jsonl`/`.jsonl.zst` 后缀的文件即使不被接受也必须以 skip 诊断现形，避免上游更改命名或压缩表示时静默漏算。
- Codex home 存在但缺少 `sessions`/`archived_sessions` 时给出警告（不再是静默空窗口）。
- 验收基线 fixture 改为真实同形：真实命名、按年/月/日分目录、`archived_sessions` 归档根、revert 形态、以及一份非 canonical 干扰文件 `notes.jsonl`；全部 id 为占位值。
- 幂等回归（半行、追加、截断、替换、plain ↔ zstd 表示切换）迁移到真实同形路径；新增最高 seam 的端到端用例（源查询适配器跑真实同形 fixture 的 totals / sessions / meta）。

## Comments

- 2026-09-09 一轮 code-review（Standards / Spec 两轴）后的修正：
  - **Spec 轴发现真实数据泄漏**：最初的 fixture 文件名直接用了本机一个真实 thread id（可由 UUIDv7 前缀解出真实时间）。已全部替换为占位 id `00000000-0000-7000-8000-00000000000N`，并在 fixture 说明中标注禁止从真实 Codex home 复制 id。
  - **事实修正**：文件名里的时间戳是上游**本地墙体时间**（例如本机某文件名为 `…T16-04-20…`，其 UUIDv7 前缀解出 `08:04:20Z`），不是 UTC。grammar 只校验形态、不解释该时刻，故实现不受影响；ticket 与代码注释的表述已更正。
  - 采纳的其他修正：`parseRolloutName` → `classifyRolloutName`（它分类而非解析）、测试字段 `tracked` → `expectSkipDiagnostic`、非法 `_` 分段上限、未知压缩后缀计入 skip 诊断、空 rollout 根目录告警。
  - 未采纳：Standards 轴指出 `query_test.go` 与 `fixture_test.go` 的计数断言重复、以及 e2e 断言 `Requests == 3` 属「数字口径」越界——保留，因为该断言校验的是「一个物理 rollout 一行、三个有 usage 的 rollout 都可见」，不是 token 口径（口径由 02 负责）。
- 2026-09-09 验证（本机真实 Codex home，`--source codex`）：发现 **74/74** 个物理 rollout（修复前 0），导入 1109 条 Codex usage event，31 个会话出现在 sessions 窗口，全部 `unpriced`，无「跳过非 canonical」噪声；另有 43 个只有 `token_count` 快照、无 durable usage record 的 rollout 仍未计入且无专门诊断——即 03 的范围。修复前本机汇总 `totalTokens` 285,975,614（应为约 1.457 亿），即 02 的范围。
- 2026-09-09 验证命令：`go test ./...`（全绿）、`go vet ./...`（clean）、`go build ./cmd/token-analyzer`（通过）、`gofmt -l internal/codex internal/query`（empty）。
