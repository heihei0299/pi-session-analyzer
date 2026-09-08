---
name: tdd-implement
description: "完成已确认的 spec/ticket 的 test-first/TDD 交付闭环。"
disable-model-invocation: true
---

# TDD Implement

`seam` + `red-green` 是本技能的领衔词。它把一个 spec 或 task issue 编排成四个交付阶段；TDD 的红-绿语义、测试质量和 mock 边界以 [tdd 技能](.agents/skills/tdd/SKILL.md) 为唯一事实源，本技能只定义交付编排。

本技能是 **Long-Horizon Skill**：阶段按顺序连续执行，并自带 **Turn Continuity** 与 **Chunking**。术语见 `CONTEXT.md`，技能设计规则见 `docs/agents/skill-design.md`。

## 入口与分支

- **单 issue**：单个 `.scratch/<feature>/spec.md`、等价 spec 或 `Type: task` issue，按下方四个 Steps 完成一个交付闭环。
- **多 issue**：`.scratch/<feature>/issues/` 下存在多个 `Type: task` 文件时，先读取 [orchestration.md](references/orchestration.md)，按 `Blocked by` 构建 DAG、Kahn 分层，再由主代理按层串行完成各 issue。
- `Type: research`、`prototype`、`grilling` 分流到对应技能，不进入本技能。

多 issue 的 A0-A5 是编排控制活动，不是额外的产品交付阶段：依赖图、分层、串行调度、层收敛、全量收敛和回退/冲突处理的详规只在 [orchestration.md](references/orchestration.md) 中维护。

## 四阶段 Steps

按序执行；每步达到可验证出口条件后立即进入下一步。每步开始前读取 [stages.md](references/stages.md) 中对应定义。

| Step | 做什么 | 出口条件 |
|---|---|---|
| ① **Contract** | 读取入口，提取 Acceptance Criteria，建立 Scope Ledger、Preflight、验证矩阵和 Behavior/Seam 边界 | 需求无待决歧义，验证命令已确定；知道做什么、从哪里验证、什么不做 |
| ② **Red-Green** | 以 Behavior 为粒度执行有效 Red → 最小 Green → formatter/typecheck → 最小相关测试 | 所有 Behaviors 均有有效 Red、实现全绿，formatter/typecheck 和最小相关测试通过 |
| ③ **Verify** | 运行当前 issue 影响范围测试、必要 build、要求的真实运行验证；执行一次 Standards + Spec Review | 最终 diff 的相关证据通过，真实运行验证完成（如要求），无 blocking finding |
| ④ **Deliver** | 对齐 docs/README，执行敏感信息扫描，检查 staged diff、commit message 和必要的 Git history，创建独立 commit，更新 issue/progress.md | commit 已创建，Acceptance Criteria 全部通过，Tracker 与工作区反映真实完成状态 |

## 运行时纪律

- 四个阶段都从入口连续执行到自身出口：预告下一步后立即执行；进度输出并入工具调用序列，输出后继续执行。只有合规交互点、明确的外部阻塞或阶段出口条件结束当前回合。
- 一个 seam 是公共可观察边界；一个 Behavior 是一个红-绿 cycle；一个 seam 可以包含多个 Behaviors。Seam/Behavior 的细节和 Todo 粒度见 [stages.md](references/stages.md)。
- 当前 issue 的范围、Acceptance Criteria、Out of Scope、测试/typecheck/build/真实运行证据和最终 commit 必须可追溯。Seam 或专项测试绿色不代表 issue 完成；四阶段出口全部满足后才可标记 `resolved`。
- 多 issue 模式中，每个 issue 只提交一个独立 commit；issue 影响范围测试在 Step ③ 执行，全仓测试由 orchestration 的 A4 在全部 issue 完成后执行一次。

## 引用

- TDD 核心规则：[tdd 技能](.agents/skills/tdd/SKILL.md)
- 测试标准：[tdd/tests.md](.agents/skills/tdd/tests.md)
- Mock 指南：[tdd/mocking.md](.agents/skills/tdd/mocking.md)
- 四阶段详规：[stages.md](references/stages.md)
- 多 issue 编排：[orchestration.md](references/orchestration.md)
