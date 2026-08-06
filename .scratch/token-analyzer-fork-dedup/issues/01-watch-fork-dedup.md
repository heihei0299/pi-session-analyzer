# 01 — watch 模式 fork 去重

**What to build:** `--watch` 实时监控与静态 CLI / webui 同口径——fork 会话的复制历史（`message.timestamp < forkTs`）不再计入实时 totals。增量读取器首次读取会话文件时解析 header（`parentSession` + timestamp → forkTs），剔除复制历史；forkTs 随文件跟踪状态持久化，文件替换/重读路径复用同一 forkTs（不把复制历史重新计入）；fork 后新增消息与正常追加路径不受影响。`--watch` 输出与静态 CLI totals 在含 fork 会话的数据集上逐字段一致。ADR-0001 与 README 同步 watch 去重决策。

**Blocked by:** None — can start immediately。

**Status:** resolved

- [x] `--watch` 首读 fork 会话（header 含 parentSession）时，复制历史消息（ts < forkTs）不计入 totals
- [x] fork 后新增消息（ts >= forkTs）正常计入
- [x] 已跟踪文件发生替换/重读时，复用初始 forkTs，复制历史不重复计入
- [x] 正常追加路径不受 forkTs 影响（新增行照常累计）
- [x] 新增 watch 单步测试（复用既有单步驱动模式）：fork fixture 首读断言不计复制历史
- [x] 同一 fork fixture 数据集：`--watch` 输出 == 静态 CLI totals（fork 场景一致性）
- [x] 既有测试全绿（109 用例）
- [x] 领域文档同步：ADR-0001 补 watch 决策；README 统计口径节注明 watch 也去重

## 实施总结
- 提交：`19f2af4` — feat: --watch fork 会话去重——首读解析 forkTs 剔除复制历史、重读复用（issue 01-watch-fork-dedup）
- 实现的 seams：T1 首读 fork 会话剔除复制历史 / T2 fork 后追加正常计入 / T3 替换重读复用 forkTs / T4 watch==CLI 逐字段一致 / T5 ts==forkTs 边界保留（test/24-watch-fork-dedup.test.ts，复用 05 单步驱动模式）
- 实现要点：`src/watch.ts` — FileState 持久化 forkTs（number|null）；readEntriesFrom 支持 knownForkTs 复用（重读路径传 state.forkTs）与 offset===0 时 resolveForkTs 解析 header；过滤语义与 analyzeFile 一致（parseUtcTimestamp UTC 解析、ts<forkTs 剔除、无效 ts 保守保留）；追加路径不解析不过滤；extractUsage→extractEntry 返回 usage+timestamp
- 验收标准：逐条 `- [x]`（上表 8 条全绿）
- 测试结果：全绿 114 用例（109 既有 + 5 新增）；变异验证：禁用过滤 → 4 红，还原 → 4 绿（测试可捕捉回归）
- typecheck：通过（tsc --noEmit 无错误）
- Code Review：Standards 轴（extractEntry 改名已修；共享 helper 抽取不采纳——需动未提交 analyze.ts，T4 已钉死口径）；Spec 轴无缺口
- 文档对齐：ADR-0001 补 watch 决策与边界约束、README 统计口径节与开头段注明 watch 也去重——已更新工作区文件；因 README/docs/adr 含未提交的 fork 核心/webui-fixes 描述，未纳入本 commit，建议随既有批次提交
- 遗留 / 后续建议：① analyze.ts fork 核心（12 行）、test/23、.scratch/token-analyzer-fork-dedup/spec.md 与 README/docs/adr 为既有未提交工作，建议单独 commit；② 替换路径复用 forkTs 边界（替换成不同会话沿用旧 forkTs）为 issue 明文决策，ADR 已补记；③ README 中 webui-fixes 描述（分页/时间筛选/109 用例）随 webui-fixes 批次提交
