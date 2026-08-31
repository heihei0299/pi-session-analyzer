---
name: commit-check
description: "Run the pre-commit gate before any commit: verify docs match the implementation, align README, keep the directory clean, and write a clear commit message. Use whenever the user is about to commit or asks to check anything about the commit — e.g. verifying docs/README are in sync, cleaning up temp files, scanning for secrets/keys/.env in the change, or having you write the commit message. Not for general PR/code review (that's code-review), and not for explaining git/commit conventions (that's a teach task)."
---

# Commit Check

提交前的**门禁检查**：文档一致性 → 保持目录卫生 → 规范 commit message，三项全过才允许 commit。本技能是轻量检查清单，不重写 code-review 的审查语义（[code-review](.agents/skills/code-review/SKILL.md) 是唯一事实源），也不替代任何完整实现流程——它是任何 commit 前的通用门禁，无论改动来自哪个流程。

## 三项检查（全部通过才 commit）

### ① 文档一致性
> 覆盖原 ① 审查文档 的全部检查项与原 ② 对齐 README 的全部检查项

- 本次改动涉及的行为/接口/配置/命令是否有对应文档（README、`docs/`、技能正文）描述
- 文档描述与实现一致：无过期信息、无声称未实现的功能、无遗留的旧接口描述
- 发现不一致 → 先修文档（或更新实现），再进入下一步
- 改动涉及项目结构、分发文件、技能/命令清单时，检查 README 中对应的结构说明、映射表、清单是否同步
- 改动涉及用法/CLI/配置/示例时，检查 README 对应描述与实际一致
- 存在模板镜像/分发副本时，确认源文件与副本同步（如有守护测试，跑一遍确认）
- **特例**：`AGENTS.md` 的 `tdd-implement ↔ implement` 路由行 + 技能文件 + `.gitignore` 的 `.pi/` 忽略，且存在 `AGENTS.md.bak` 时，视为模板同步预期增量，不回滚
### ② 保持目录卫生

- `git status` 确认工作区只含预期改动：无残留未跟踪文件、无临时产物（调试脚本、日志、备份文件、`[DEBUG-...]` 残留）
- 无关文件（一次性脚本、转储、探针、调试日志）直接追加至 `.gitignore`，不执行删除；仅对本次产生的 `[DEBUG-...]` 临时产物做受控清理，禁止为达干净而执行 `git reset --hard`、`git checkout .`、`git clean -fd`、`git stash push --include-untracked`、`git push --force`、`git rebase -i` 等（需显式用户确认；`stash` 如需使用改用 `--keep-index` 并在 `pop` 后校验 `git merge-base --is-ancestor $BASE_HEAD HEAD`）。详见 `CONTEXT.md` Git History Preservation 与 `docs/agents/skill-design.md` Rule 4
- 若本次会话记录了 `BASE_HEAD`，commit 前校验 `git merge-base --is-ancestor $BASE_HEAD HEAD`，失败即经 `git reflog` 恢复后才提交
- 确认没有敏感信息进入改动（密钥、token、`.env`、私钥）——跑 `scripts/scan-sensitive.sh`，不用手写扫描
- 提交后工作区应为干净状态（`git status` 无输出）

### ③ 规范 commit message

- 格式遵循仓库约定（常见：`<type>(<scope>): <subject>`，type 用 feat/fix/docs/chore/refactor/test）
> 模板：`feat(<scope>): <subject>` / `fix(<scope>): <subject>` — 例 `feat(tdd): add seam login`，`fix(ci): gate publish on verify`
- subject 描述变更内容而非过程（不说"我做了什么"，说"改成了什么"）
- 需要时补充 body：动机、影响范围、验收证据（测试结果、同步确认）
- 一次 commit 只含一个逻辑变更；多主题拆多个 commit

## 不做什么

- 不做全量 code review：审查语义以 [code-review](.agents/skills/code-review/SKILL.md) 为唯一事实源，本技能不重写
- 不替代实现流程的收尾：`tdd-implement` 阶段⑦已含文档对齐与目录卫生，本技能只管独立 commit 的门禁
- 不发明扫描规则：敏感信息检测跑 `scripts/scan-sensitive.sh`，不每次重写 grep 模式

## 执行顺序（回合内串行）

1. 跑 ① 文档一致性 → ② 保持目录卫生 → ③ 写 commit message
2. 任一项发现问题：修复后重跑该项，全部通过才 commit
3. commit 后确认 `git status` 干净，工作结束

**回合连续性**：三项检查在一个回合内串行完成，不等用户"继续"；发现问题立即修复并重查，直到三项全过或遇到外部阻塞（权限/授权缺失）。

## 出口条件

- [ ] 文档一致性（文档与 README 均已对齐）
- [ ] 目录卫生（`git status` 干净，无临时产物/敏感信息）
- [ ] commit message 规范（遵循仓库格式）
- 三项全过 → commit

## 引用

- 代码审查语义：[code-review](.agents/skills/code-review/SKILL.md)（唯一事实源，本技能不重写）
- 完整实现流程：[tdd-implement](.agents/skills/tdd-implement/SKILL.md)（含流程内收尾的文档对齐与目录卫生）
