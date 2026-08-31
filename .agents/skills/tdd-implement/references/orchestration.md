# 多 issue 编排（按依赖分层并行）

本文件仅在多 issue 编排模式下生效；单 issue / 单 spec 走 [stages.md](stages.md) 单线流程，不经过本文件。

多 issue 触发条件：`.scratch/<feature>/issues/` 下存在多个 issue 文件且至少部分含 `Blocked by` 依赖声明。

## 目录

- [A0. 依赖图构建](#a0-依赖图构建)
- [A1. 拓扑分层](#a1-拓扑分层)
- [A2. 分层调度](#a2-分层调度)
- [A3. 子代理契约（单 issue 单代理）](#a3-子代理契约单-issue-单代理)
- [A4. 全量收敛](#a4-全量收敛)
- [A5. 回退与冲突](#a5-回退与冲突)
- [出口条件](#出口条件)
- [边界](#边界)

---

### A0. 依赖图构建

1. 扫描 `.scratch/<feature>/issues/` 下全部 `NN-<slug>.md`，逐文件解析 `Blocked by` 行：
   - `Blocked by: None` / `Blocked by: （无` / 无此行 → 无依赖（frontier）
   - `Blocked by: 01, 02` / `Blocked by: 01（…）` → 依赖 `01`、`02` 对应的 issue 文件（按编号前缀匹配）
   - 无法解析的行 → 视为无依赖，并在编排总结中注明告警
2. 以 issue 编号为节点、`Blocked by` 为有向边构建 DAG；若检测到环，立即报错并列出环上节点，不进入调度。
3. 读取 `spec.md`（若存在）作为各子代理的共享上下文；同时读取 `CONTEXT.md` 与 `docs/adr/` 供一致性校验。

### A1. 拓扑分层

对 DAG 做 Kahn 分层（BFS 拓扑）：

```
L1 = 全部入度为 0 的节点（可立即开始）
L2 = 移除 L1 后入度为 0 的节点
…
Ln = 最后一层
```

每层内节点互无依赖，可并行；层间有依赖，必须串行。分层结果在编排开始前一次性展示给用户确认（合规交互点），确认后才派发。

### A2. 分层调度

```
for each 层 Li in L1..Ln:
  并行派发：为 Li 中每个 issue 启动一个子代理（single 模式，禁止 parallel tasks 数组）
  等待：阻塞直到 Li 全部子代理返回回执卡片
  验收：编排器按 A3 验收清单逐 issue 验收（只认回执卡片的关键信息 + 抽检验证，不消费全量日志）
  层收敛验证：验收全通过进入全量验证（完成条件 4 项，全部通过才进下一层，任一失败按 A5 回退）：①该层全部 issue 验收通过 ②相关测试套件通过（全量仅在 A4） ③`git status` 卫生（仅删本次临时产物，正向；护栏：禁止 `git reset --hard`/`git checkout .`/`git clean -fd`/`git stash push --include-untracked`）④历史校验 `git merge-base --is-ancestor $BASE_HEAD HEAD` 通过；验收不通过或相关/卫生/历史任一失败按 A5 回退重派该 issue
全部层层收敛通过后进入 A4 全量收敛
```

- **派发纪律**：与阶段⑤双轴审查一致——逐个 `subagent` 派发，禁止 `parallel tasks` 数组（同因：中文报告截断）。
- **等待语义**：层内任一子代理失败不取消同层其他子代理；待层内全部返回后统一按 A5 处理。
- **回合连续性**：编排器在层间不结束回合——一层收敛后立即派发下一层，直到全部层完成或外部阻塞；预告下一层后立即执行。
- **Git 历史保护（正向：仅追加；护栏：禁改写）**：编排器在分层调度前记录 `BASE_HEAD=$(git rev-parse HEAD)`，每层收敛后校验 `git merge-base --is-ancestor $BASE_HEAD HEAD`，失败即经 `git reflog` 恢复；为达 `git status` 干净仅删本次产生的 `[DEBUG-...]`临时产物（正向），护栏：禁止 `git reset --hard`/`git checkout .`/`git clean -fd`/`git stash push --include-untracked`/`git push --force` 等（需显式确认）。

### A3. 子代理契约（单 issue 单代理）

每个子代理是一个**完整的 tdd-implement 单 issue 执行单元**，输入与产出严格界定：

> 编排层为 Feature 层，按 `Blocked by` 分层；每子代理各自治完成完整 tdd-implement 流程，产出独立 commit；禁止跨 issue 改动；输出约束为回执卡片，不透传全量过程日志；主代理验收保证无跨 issue 改动与逐 issue 验收，打回重派直至验收通过才计入层收敛。

- **输入**：
  - `spec.md`（feature 级共享 spec，若无则以该 issue 正文为准）
  - 分配的单个 `NN-<slug>.md`（唯一 issue 输入）
  - `CONTEXT.md` + `docs/adr/`（术语与决策一致性）
- **执行**：严格走 tdd-implement ①→⑦全流程——①理解需求（读 spec + issue）→ ②确认 seams（该 issue 范围内）→ ③红-绿循环（每 cycle 后 typecheck + 相关测试）→ ④相关测试套件（仅该 issue 相关 + typecheck，不跑全量；全量由编排器在 A2 层收敛/A4 统一执行，单 issue 单线模式仍跑全量）→ ⑤双轴 review → ⑥commit-check 门禁 + commit → ⑦文档对齐（仅该 issue 相关描述）+ `Status: resolved` + `## 实施总结` 落盘 + 目录卫生。TDD 语义以 [tdd 技能](.agents/skills/tdd/SKILL.md) 为唯一事实源，不在子代理内重写。
- **产出**：
  - 独立 commit（message 含 issue 编号，如 `feat(<feature>): <issue title> (#NN)`）
  - 该 issue 文件 `Status: resolved` + 底部 `## 实施总结`
  - 该 issue 范围内的测试全绿 + typecheck 通过
- **禁止**：跨 issue 改动；修改其他 issue 文件；跳过 ⑤/⑥ 直接 commit。

#### 输出约束（子代理只返回回执卡片）

子代理不向编排器透传全量过程日志（各 seam 的红-绿细节、typecheck 原始输出、双轴 review 全文、完整测试日志）。只返回一张**回执卡片**（结构化关键信息，中文，≤ 30 行）：

```
[回执] #NN <issue 标题>
- 提交：<commit hash> — <message>
- seams：<清单>
- 测试：相关测试 <数量> 项全绿 / 失败清单（全量由编排器层收敛/A4 验证）
- typecheck：通过 / 失败原因
- review：Standards <通过/问题> / Spec <通过/问题>
- 验收：checkbox <m/n 全绿，缺口说明>
- 文档：<更新文件 / 无需更新>
- 遗留：<如有>
```

卡片字段缺一不可；缺失字段视为验收不通过。详细过程与证据留在子代理的 commit 与 issue 文件中，编排器按需抽检而非全量消费。

#### 主代理验收（编排器逐 issue 验收）

编排器收到回执后逐 issue 验收，不盲信子代理自检：

1. **落盘校验**：`git log --oneline` 含该 commit 且 message 含 `#NN`；issue 文件 `Status: resolved` 且底部 `## 实施总结` 已落盘。**完成条件：5 项检查表，全部通过才计入层收敛，任一不过即打回重派**（详见 `SKILL.md` 主代理验收）。
2. **抽检验证**：抽跑该 issue 相关测试（或 `tsc --noEmit` 抽检），不重跑全量套件；抽检失败即打回。
3. **改动边界**：`git diff <base>..HEAD --name-only` 核对无跨 issue 文件改动；有跨改视为不通过。
4. **卫生**：`git status` 无 `[DEBUG-...]` 残留与未跟踪临时文件。
5. **提交关联**：`git log` message 含 `#NN` 且与落盘 commit 一致；缺失或不一致视为不通过。

任一项不通过 → 打回重派该子代理（仅该 issue），层内其他已通过不受影响；验收通过才计入层收敛。验收结论随层收敛一并输出。

子代理内部的回合连续性、任务分解、Todo 规定、Git 历史保护与单线模式完全一致（见 [stages.md 阶段③ 3e/3f/3h](stages.md#阶段-③tdd-开发循环) 与 Git 安全前置）。子代理同样在入口记录 `BASE_HEAD` 并在每阶段出口校验 `git merge-base --is-ancestor $BASE_HEAD HEAD`，禁止 `git reset --hard`/`git checkout .`/`git clean -fd`/`git stash push --include-untracked` 等。

### A4. 全量收敛

全部层逐 issue 验收通过后，编排器执行：

1. **全量测试套件**：跑仓库完整测试套件（所有 issue 执行完毕后的唯一全量，阶段④与 A2 已改为相关，仅此一次）
2. **历史校验**：执行 `git merge-base --is-ancestor $BASE_HEAD HEAD`，若为 false 说明编排过程中历史被改写，立即经 `git reflog` 恢复后重跑收敛。
3. **目录卫生**：`git status` 确认无 `[DEBUG-...]` 残留、无未跟踪临时文件；有残留则仅删本次临时产物后重检，禁止 `git reset --hard`/`git checkout .`/`git clean -fd`/`git stash push --include-untracked` 等。
4. **汇总总结**：在会话输出汇总各 issue 的回执卡片关键信息（提交 hash / seams / 验收 checkbox / 测试结果 / 文档对齐）；不另写汇总文件，不透传子代理全量日志（各 issue 的 `## 实施总结` 已落盘，详查落盘文件）。

### A5. 回退与冲突

- **子代理内回退**：按 [stages.md 回退路由](stages.md#回退路由) 在子代理内闭环（typecheck 失败 → ③、测试失败 → ③、review 不通过 → ③/②/①）。
- **层收敛失败**：层内任一子代理未达到 `resolved`（测试失败 / review 不通过 / commit-check 门禁失败）→ 该 issue 保持原 `Status`，编排器在层等待结束后报告失败清单，不自动进入下一层；待修复后重派该层失败节点。
- **全量收敛失败**：A4 全量测试失败 → 定位到失败测试归属的 issue，回到其所在层重派对应子代理。
- **文件冲突**：同层子代理若触及同一文件，后完成者 rebase 解决冲突后重跑 typecheck + 相关测试；跨层天然串行无冲突。冲突解决禁止使用 `git reset --hard`/`git checkout .`/`git clean -fd`/`git stash push --include-untracked` 丢弃对方提交，rebase 后必校验 `git merge-base --is-ancestor $BASE_HEAD HEAD` 且 `git log --oneline` 含全部层提交；冲突检测以 `git` 合并结果为准，编排器不做静态预判。
- **环依赖**：A0 检测到环即报错终止，不派发任何子代理。

### 出口条件

- 全部 issue `Status: resolved` + 各自 `## 实施总结` 已落盘
- 全量测试套件通过
- 工作区干净（`git status` 无残留）

### 边界

- 单 issue / 单 spec 不走本文件
- 子代理不跨 issue 改动；编排器不替子代理写实现代码
- 汇总总结只在对话输出，不落盘额外汇总文件
- TDD 语义以 [tdd 技能](.agents/skills/tdd/SKILL.md) 为唯一事实源，不在本文件重写

