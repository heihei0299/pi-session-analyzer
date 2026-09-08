---
name: commit-check
description: "检查 matt-skills 当前 staged commit 的范围、敏感信息和 commit message，并按改动路径执行对应同步检查。"
disable-model-invocation: true
---

# Commit Check

用户显式调用 `/commit-check` 后运行本技能。它是提交前的 staged commit gate：检查将要提交的内容是否属于当前逻辑变更、是否包含敏感信息、以及 commit message 是否可追溯。

本技能只检查，不负责 staging，不执行 `git commit`，也不要求整个工作区干净。通过后报告 `ready to commit`，由调用方执行提交。

## 三项核心 gate

### ① Staged scope

- 读取 `git diff --cached --name-status` 和 `git diff --cached`。
- staged diff 必须非空，并且只包含当前用户请求的逻辑变更。
- 列出 staged 文件和关键 diff，无法确认范围时报告疑点并阻塞。
- 调用方负责 `git add`；本技能不自动 stage、unstage 或清理文件。
- 工作区可以保留其它未暂存修改；不以 `git status` 全干净作为出口条件。

### ② Sensitive scan

运行确定性扫描脚本，不手写 grep：

```bash
bash .agents/skills/commit-check/scripts/scan-sensitive.sh --staged-only
```

- 结构化 secret assignment 和 private key block → **fail**。
- 普通 `api_key`、`secret`、`token`、`password`、`.env` 等关键词 → **warning**，由调用方人工确认。
- 扫描只针对 staged diff；不因未暂存内容阻塞本次 commit。

### ③ Commit message

- 使用 `<type>(<scope>): <subject>` 基本格式；`type` 使用 `feat`、`fix`、`docs`、`chore`、`refactor`、`test` 等仓库约定值。
- subject 描述变更结果，不描述操作过程。
- body 可选；只有确实需要时补充动机、影响范围或验收证据。
- 一个 commit 只表达一个逻辑变更；多主题拆分提交。
- 本技能检查并给出 message 结论，但不代替调用方执行 commit。

## matt-skills 路径适配

以下 staged 路径触发 matt-skills 专属检查；普通源码或测试 commit 不触发这些额外检查：

```text
README.md
AGENTS.md
CONTEXT.md
docs/agents/**
template/**
.agents/skills/**
config/**
scripts/build-template.js
.opencode/commands/**
.pi/prompts/**
```

相关路径变更时：

- README、公开行为、命令、配置或流程描述变化 → 检查对应文档与实现一致；
- `AGENTS.md`、`CONTEXT.md`、`docs/agents/`、`template/`、技能或镜像变化 → 运行相关模板/契约测试，至少覆盖 `test/template-sync.test.js`；
- `commit-check` 自身变化 → 运行 `test/commit-check.test.js` 和 `test/commit-check-scan.test.js`；
- `tdd-implement` 变化 → 运行 `test/tdd-implement-stages.test.js`；
- `config/`、构建脚本、opencode command 或 pi prompt 变化 → 运行对应 CLI、模板或命令测试；
- 只检查本次 staged 路径相关的内容，不通读全部 README、docs 或模板。

这些是 matt-skills 的条件化仓库检查，不改变上面的三项核心 gate。

## Git history pointer

遵循 `CONTEXT.md` 和 `docs/agents/` 中的 Git History Preservation 规则。本技能只在当前会话存在 `BASE_HEAD` 时执行必要祖先校验：

```bash
git merge-base --is-ancestor "$BASE_HEAD" HEAD
```

完整的禁止命令、恢复和 stash 规则只在仓库级文档维护，不在本技能重复展开。

## 执行顺序与出口

1. 检查 staged scope，确认 staged diff 非空且属于当前逻辑变更。
2. 执行 `scan-sensitive.sh --staged-only`。
3. 检查 commit message。
4. 根据 staged 路径执行必要的 matt-skills 条件化检查。
5. 输出 staged 文件、三项 gate 结果、warning、条件化检查结果和 `ready to commit` 或具体阻塞项。

发现阻塞项时停止并报告；不自动修复、不自动 stage、不自动 commit。

## 不负责的内容

- 不做完整代码审查；审查语义由对应审查流程负责。
- 不执行测试先行、typecheck、build、真实运行或 tracker 收尾；这些属于对应实现流程。
- 不成为任何实现流程的自动子步骤；仅按 staged 路径提供本仓库提交 gate。
