# 11 — 统计口径（哪些 usage 计入总量）

Type: grilling
Status: resolved
Blocked by: 01

## Question

总消耗量与会话级消耗量窗口的统计口径是什么？

候选口径：
- A: 与 pi 自身一致——只认 `type=message && role=assistant` 的 usage（toolResult / compaction / branch_summary 均忽略）
- B: 全量——含 toolResult / compaction / branch_summary 的 usage（真实消耗全覆盖，但数字与 pi 自身展示口径可能不一致）
- C: 混合（请说明）

## 背景

map `Not yet specified` 中的 fog，由「单次请求窗口定义」（ticket 06）决议后毕业：06 已确定单请求窗口 = assistant 消息 usage（toolResult 不计入），但总/会话窗口是否含 toolResult / compaction / branch_summary 的 usage 尚未决策。

ticket 01 的既有证据：
- `compaction` / `branch_summary` entry 顶层可选携带 usage，pi 文档注释 "included in session token and cost totals"（`docs/session-format.md`）
- 但 pi 自身会话统计只认 `type=message && role=assistant` 的 usage，其余被忽略（`pi-agent-core/dist/harness/session/jsonl-storage.js:233-240`）
- `toolResult` 消息可选携带 usage（工具嵌套 LLM 工作）

「与 pi 自身一致」与「全量」在 compaction/toolResult 场景下数字可能不同，需决策。

## Resolution

经 `/grilling` 会话与用户确认后，将结论写入本文件 `## Answer` 段落，`Status: resolved`，并在 map `Decisions so far` 追加一行。

## Answer

**决策：口径 A** —— 总消耗量与会话级消耗量窗口只认 `type=message && role=assistant` 且携带 usage 的消息；toolResult / compaction / branch_summary 的 usage 一律不计入。

- 三个窗口口径一致：总消耗量 = 会话级消耗量 = 单请求窗口（ticket 06）之和，全部锚定 assistant 消息 usage——单请求窗口是会话窗口的细粒度展开，会话窗口是总窗口的分组，无额外计入来源
- **toolResult 不计入**：与 ticket 06 一致（其消耗归属即本 ticket）；当前数据中 toolResult 10,069 行全部无 usage（2026-08 采样），计入与否当前无实际差异
- **compaction / branch_summary 不计入**：pi 文档注释称其 usage "included in session token and cost totals"（`docs/session-format.md`），但本工具统计口径独立定义，且当前数据中 compaction 8 行、branch_summary 0 行全部无 usage；spec 需记录此差异（与 pi 内部口径可能不一致）
- 依据：ticket 06（同锚）、ticket 01（usage 语义）、pi 自身会话统计口径（`jsonl-storage.js:233-240` 只认 assistant）
- 当前数据下 A/B 数字完全一致（toolResult / compaction / branch_summary 均无 usage）；未来若这些类型开始携带 usage，本工具按 A 忽略——spec 中明确写入此规则即可，无需运行时判断
