# 四阶段详细定义

单 `spec` / 单 `task` 与多 `task` 共用下表四阶段。多 issue 的依赖图、Kahn 分层、层收敛、全量收敛和回退/冲突处理见 [orchestration.md](orchestration.md)。TDD 语义以 [tdd 技能](.agents/skills/tdd/SKILL.md) 为唯一事实源，不在此重写。

## 目录

- [① Contract：明确交付契约](#阶段-①-contract明确交付契约)
- [② Red-Green：行为级 TDD](#阶段-②-red-green行为级-tdd)
- [③ Verify：最终验证与审查](#阶段-③-verify最终验证与审查)
- [④ Deliver：提交与 Tracker 收尾](#阶段-④-deliver提交与-tracker-收尾)
- [跨阶段运行纪律](#跨阶段运行纪律)
- [状态统一](#状态统一)
- [回退路由](#回退路由)

---
## 不可省略的质量门禁

无论单 issue 还是多 issue，以下门禁都必须形成证据：

1. 当前 issue 的范围、Acceptance Criteria 和 Out of Scope 明确；
2. 产品实现之前存在有效 Red；
3. 最终 diff 对应的相关测试和 typecheck 通过；
4. ticket 要求真实运行时，真实运行验证已完成；
5. Standards 与 Spec Review 无 blocking finding；
6. README/docs 与实现一致；
7. 每个 issue 形成独立、可追溯的 commit；
8. Tracker 状态与真实完成度一致。

Seam 或专项测试绿色不等于 issue 完成；只有四个阶段全部通过，issue 才能标记为 `resolved`。


## 阶段 ① Contract：明确交付契约

### 入口条件

- 用户提供单个 `spec`、等价 spec 或 `Type: task` issue。
- `research`、`prototype`、`grilling` 等非实现入口已分流。

### 操作

1. 完整读取入口；按需读取 `CONTEXT.md` 的相关术语和与本次 spec、触及符号或失败证据有关的 ADR。
2. 使用仓库规定的代码探索入口。探索结果含完整源码时视为已读，不再次 `read` 同一文件，除非文件发生漂移或只返回调用路径。
3. 逐条提取 Acceptance Criteria，并写出本 issue 的 **Scope Ledger**：

   ```text
   必须实现：当前 issue 要求的行为
   明确不做：后续 tickets 和 Out of Scope
   允许触及：预计受影响的模块、组件、接口
   ```

4. 实现或 Review 中发现的新问题必须归类为：
   - 当前 Behavior 必须修复；
   - 当前 issue 需要新增 Behavior；
   - 后续 ticket；
   - 与当前 feature 无关。

   只有前两类进入当前实现；第 2 类必须补回 Contract 和验证矩阵，第 3、4 类保留记录，不无记录地扩大范围。
5. 完成一次 **Preflight** 并记录真实结果：
   - 当前 `HEAD`、工作区状态和 `BASE_HEAD=$(git rev-parse HEAD)`；
   - 可用的 test、typecheck、build 命令；
   - 可用的 subagent model；
   - 可用的 browser 或 Playwright 路径；
   - 可用的敏感信息扫描脚本；
   - ticket 要求的真实运行验证方式。
6. 建立一次验证矩阵，列出 targeted tests、typecheck、全量测试、必要 build、smoke/package/security check 和真实运行验证，并记录各项的触发条件，后续只复用这份矩阵。
7. 识别公共测试边界和 Behaviors。一个 Seam 是一个公共可观察边界；一个 Behavior 是一个红-绿 cycle；一个 Seam 可以包含多个 Behaviors。每个 Behavior 明确输入、可观察输出、对应 Acceptance Criterion 和验证层级。
8. spec 已确认且未变化的 Seam 直接复用；只有出现需求歧义、验收缺口、范围变化、破坏性操作或互斥方案时才请求用户确认。

### 出口条件

- 能用自己的话复述需求和每条 Acceptance Criterion；
- Scope Ledger 已记录，且明确什么不做；
- 无未澄清歧义；
- 验证矩阵已建立；
- 工具、命令和真实运行路径已确认可用，或已记录为 `blocked/unavailable` 及替代路径；
- `BASE_HEAD` 已记录。

---

## 阶段 ② Red-Green：行为级 TDD

### 入口条件

- Contract 出口条件全部满足。

### 操作

1. 在本阶段入口加载 [tdd 技能](.agents/skills/tdd/SKILL.md) 的相关规则一次；每个 Behavior 只按需读取对应的 `tdd/tests.md` 和 `tdd/mocking.md` reference，不重复阅读全文。
2. 按 Behavior 建立 Todo，而不是按 Seam 建立 Todo。推荐层级：
   - 大任务：整个 issue；
   - 中任务：Seam；
   - Todo：一个 Behavior cycle；
   - Subtodo：`B1-R` 红 → `B1-G` 绿 → `B1-T` typecheck。
3. 每个 Behavior 连续执行：

   ```text
   写一个失败测试
   → 从公共接口确认目标 Behavior 失败
   → 最小实现
   → formatter
   → typecheck
   → 最小相关测试
   → 标记该 Behavior completed
   ```

4. 只有通过公共接口观察到“目标行为尚未实现”的断言失败才是有效 Red。语法错误、缺失 helper/fixture、测试环境启动失败、工具参数错误、timeout 或命令中断都记录为失败类别或 `UNKNOWN`，不能当作有效 Red。
5. 根据语言做即时验证：Go 修改后立即 `gofmt` 和最小 package test；TS/TSX 修改后立即 parser/typecheck 和最小 component test。批量编辑拆成小批，每批恢复绿色后再继续。
6. 每个 Behavior 完成后更新实际 Todo 状态，再进入下一个 Behavior；全部 Behaviors completed 后才离开本阶段。

### 回合连续性与 Chunking

- 每个 Behavior 的 Red → Green → formatter → typecheck → 最小相关测试在一个回合内串行完成；确认全绿后立即进入下一个 Behavior。
- 一个 Seam 全绿只是内部进度，不是阶段出口；阶段出口是所有 Behaviors 红-绿完成且 typecheck 通过。预告下一步后立即执行，直到阶段出口、合规交互点或外部阻塞。
- 进度输出并入工具调用序列，输出后继续执行；不要把“准备下一步”当作回合终点。
- 单次 `write` 超过约 150 行时先写骨架再分批补全；批量 `replace` 超过 5 处时拆批，每批后立即验证。
- `done`、`completed` 等状态只按当前实际推进更新，已完成项永不回退。

### 出口条件

- 所有 Behaviors 都有有效 Red；
- 所有 Behaviors 的最小实现已 Green；
- formatter、typecheck 和最小相关测试通过；
- Todo 清单反映真实状态，全部 Behavior Todo 为 `completed`；
- `BASE_HEAD` 祖先校验通过。

---

## 阶段 ③ Verify：最终验证与审查

### 入口条件

- Red-Green 出口条件满足，当前 diff 稳定。

### 固定顺序

```text
当前 issue 影响范围测试
→ 必要 build
→ 必要真实运行验证
→ 一次 Standards + Spec Review
→ 修复 blocking finding 后的定向复核
```

### 测试与真实运行验证

- 多 issue 模式只运行当前 issue 影响范围内的完整测试；不在每个 issue 重复运行全仓测试。全部 issues 完成后由 orchestration A4 运行一次全仓测试。
- 单 issue 或单 spec 模式运行仓库完整测试。按照 Contract 的验证矩阵执行，不同时运行等价命令。
- ticket 要求真实运行时，优先使用专用 browser 工具，其次使用项目已有 Playwright；HTTP/CLI 只能补充 API 验证，不能替代 WebUI 验证。
- 真实进程验证使用隔离配置和临时端口，保存 PID，记录实际请求结果或页面可见结果，结束时清理进程和临时目录。

### Review

1. 每个 issue 恰好执行一次正式双轴 review：
   - **Standards**：是否符合仓库规则和代码质量要求；
   - **Spec**：是否逐条满足当前 issue 的 Acceptance Criteria。
2. 两个轴独立输出、互不掩盖；每个轴明确限制输出，例如 `≤ 400 words / ≤ 40 行`。
3. findings 分类为：当前 issue blocking、后续 ticket、advisory、out of scope。只处理当前 issue blocking finding；其余记录而不扩大范围。
4. 修复 blocking finding 后只运行受影响测试、typecheck 和 finding 的 delta recheck，不重新启动完整双轴 review。审查结果只在对话输出，不生成 `review-*.md` 等书面报告文件。

### 出口条件

- 最终 diff 对应的相关测试通过；
- 必要 typecheck/build 通过；
- ticket 要求的真实运行验证已完成并记录实际结果；
- 一次 Standards + Spec Review 已完成；
- 无 blocking finding；
- 受影响范围的最后一次证据对应当前 diff。

---

## 阶段 ④ Deliver：提交与 Tracker 收尾

### 入口条件

- Verify 出口条件满足。

### Commit 前门禁

按以下顺序完成并记录事实：

1. 最终逐条检查 Acceptance Criteria；
2. 检查 README/docs/config/package 与实现一致；
3. 复核 Scope Ledger，确认没有未记录的范围扩张；
4. 确认证据对应最后一次代码或测试修改；
5. 检查临时文件、构建产物和未跟踪文件；
6. 对 staged diff 执行敏感信息检查；
7. 执行 `git merge-base --is-ancestor $BASE_HEAD HEAD`；
8. 检查 commit message；
9. 确认暂存区只包含当前 issue；
10. 执行 `git diff --cached`，再创建当前 issue 的独立 commit。

执行敏感信息扫描脚本（`bash .agents/skills/commit-check/scripts/scan-sensitive.sh --staged-only`），检查 staged diff、commit message 和 Git history preservation，全部通过后创建当前 issue 的独立 commit。

### Tracker 收尾

Commit 成功后：

- 逐条勾选 Acceptance Criteria；
- 将 issue 状态改为 `resolved`；
- 追加实施总结；
- 更新 `.scratch/<feature>/progress.md` 的 `Status`、`Commit`、`Review`、`Tests`；
- 记录 commit hash、message、最终测试命令/数量/结果和真实运行结果；
- 清理本次产生的临时进程、目录和一次性文件；
- 确认下一 issue 的 blockers 已解除。

阶段 ④ 开始后不新增产品 Behavior。若实现、测试或文档不完整，回到对应阶段；不要在 Tracker 收尾后继续修改源码，也不要通过额外 docs-only commit 掩盖遗漏。只有四个阶段全部通过，才可把 issue 标记为 `resolved`。

### 出口条件

- commit 已创建且为当前 issue 的独立提交；
- Acceptance Criteria 全部通过；
- issue 状态为 `resolved`（无关联 issue 的直接 spec 则在会话中输出总结）；
- 实施总结和 `progress.md` 已同步；
- 工作区符合预期，无本次临时产物或残留未跟踪文件；
- 文档与实现一致，Git 历史保护校验通过。

---

## 跨阶段运行纪律

### Tool Failure Budget

```text
首次失败
→ 判断失败类别
→ 最多一次有依据的 fallback
→ 仍失败则记录 blocked/unavailable 并停止该路径
```

相同命令或工具参数不原样连续重试；timeout 或中断后缩小到 package、文件或具体 test；model、browser 或 tool 不可用时最多一次 fallback。用户要求停止或 handoff 时立即停止。

### 验证证据失效

任何产品代码或测试文件再次变化，旧的测试、typecheck、build 和 review 证据立即失效；必须重新验证受影响范围。只能使用最后一次修改之后的结果证明当前 diff 已完成。

### Git History Preservation

进入 Contract 时记录 `BASE_HEAD=$(git rev-parse HEAD)`；每个阶段出口和 commit 前都执行：

```bash
git merge-base --is-ancestor $BASE_HEAD HEAD
```

失败时先经 `git reflog` 找回被改写的历史，再继续。为达到工作区干净只删除本次产生的 `[DEBUG-...]`、一次性脚本和临时文件；未经用户确认不使用 `git reset --hard`、`git checkout .`、`git clean -fd`、`git stash push --include-untracked`、`git push --force`、`git rebase -i` 或任何让 `HEAD` 后退的命令。需要 stash 时使用 `--keep-index`，pop 后重新校验。

---

## 状态统一

```text
Todo:     pending | in_progress | completed | blocked
Issue:    ready-for-agent | in_progress | resolved | blocked
Progress: pending | in_progress | done | blocked
```

状态转换：Contract 完成后 Issue/Progress 为 `in_progress`；Red-Green 完成后 Behaviors 为 `completed`，Issue 仍为 `in_progress`；Verify 完成后 Issue 仍为 `in_progress`；Deliver 完成后 Issue 为 `resolved`、Progress 为 `done`。外部阻塞记录为 `blocked`，恢复后回到 `in_progress`。

---

## 回退路由

| 当前阶段 | 回退条件 | 回退目标 |
|---|---|---|
| ① Contract | 需求歧义、验收缺口、范围变化 | → ① 补充契约和验证矩阵 |
| ② Red-Green | 有效 Red、实现、formatter、typecheck 或相关测试失败 | → ② 修复当前 Behavior |
| ③ Verify | 测试、build、真实运行或 review finding 失败 | → ② 修复 Behavior；需求偏差 → ① |
| ④ Deliver | docs、敏感扫描、staged diff、commit message 或 Tracker 信息不完整 | → ①/③ 修复对应证据；仍在 Deliver 前完成 |

多 issue 的层收敛、全量失败、依赖冲突和跨 issue 修改冲突按 [orchestration.md](orchestration.md) A5 回退，不跨 issue 无记录改动。
