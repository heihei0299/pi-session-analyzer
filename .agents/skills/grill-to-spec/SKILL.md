---
name: grill-to-spec
description: "Router：编排 grill-with-docs → to-spec，只打磨设计与产出文档/spec，不写代码。"
disable-model-invocation: true
---

# Grill to Spec

**grill-with-docs**（grilling + domain-modeling）与 **to-spec** 的编排器。本 skill 只做编排：把设计压力测试成共识，把共识综合成 spec 发布——不写代码，不动源码。

## 职责

| 做 | 不做 |
|----|------|
| 编排 `grill-with-docs` → `to-spec` 完整通道 | 不编写代码、不修改任何源码（含测试） |
| 引导用户从模糊想法 → 结构化 spec | 不拆 tickets（`/to-tickets` 职责） |
| grilling 逐问挑战、打磨设计 | 不调用 `/code-review` |
| 同步产出领域文档（glossary inline；ADR 草稿经用户确认后落盘） | 阶段②仅综合，不新增采访 |
| 综合对话为可执行的 spec 文档并发布 | 不维护已发布的 spec |
| 产出物仅限领域文档与 spec | 实现与修复交给实现类 skill（如 `/tdd-implement`） |

## 流程

```text
① Grill with docs → ② Synthesize to spec
```

① 加载 `/grill-with-docs`：grilling 采访（一次一问、等反馈；决策逐条交由用户定夺）+ domain-modeling 产出 glossary/ADR。出口：用户确认共识达成。

   ① 内 ADR 子流转（与 glossary 的 inline 更新严格区分）：
   1. 触发：仅当 domain-modeling 三条件全满足（难逆转 / 无上下文费解 / 真实权衡）才提议 ADR
   2. 草稿：按 ADR-FORMAT 把完整标题+正文展示给用户审阅，等待反馈
   3. 确认：用户显式说「确认/写入」才落盘；用户拒绝则不写、继续访谈；用户要求修改则改草稿重新确认
   4. 未确认前不得创建或写入 `docs/adr/` 下的任何文件
② 加载 `/to-spec`：探索代码（glossary 词汇贯穿 spec、尊重相关 ADR）→ 确认 seams（既有优先、最高 seam、理想一个）→ 按七节模板编写 spec 草稿 → 展示给用户确认（只展示等决定，不新增采访提问）→ 发布到 `.scratch/<feature-slug>/spec.md` 并标 `ready-for-agent`。出口：spec 已发布。

## 产出物（格式严格对齐下游技能）

本技能产出物仅以下三种，格式以各技能文件为唯一事实源，不在本技能重写：

| 产出物 | 位置 | 格式来源 |
|--------|------|----------|
| Glossary | `CONTEXT.md`（多上下文：`CONTEXT-MAP.md` + 各上下文 `CONTEXT.md`） | [CONTEXT-FORMAT.md](.agents/skills/domain-modeling/CONTEXT-FORMAT.md) |
| ADR | `docs/adr/NNNN-slug.md`（多上下文：系统级在根，上下文级在 `src/<ctx>/docs/adr/`） | [ADR-FORMAT.md](.agents/skills/domain-modeling/ADR-FORMAT.md) |
| Spec | 发布到 issue tracker：`.scratch/<feature-slug>/spec.md` | [to-spec 七节模板](.agents/skills/to-spec/SKILL.md) |

### Glossary 守则
- 懒创建：首个术语解析时才建 `CONTEXT.md`；多上下文时先确认归属，归属不清则询问
- 只是 glossary：零实现细节，不当 spec/scratch pad
- 只收本上下文特有术语，通用编程概念不收
- 定义 WHAT 非 HOW，1-2 句；opinionated，同义词列 `_Avoid_`；术语解析即 inline 更新，不批量

### ADR 守则
- 三条件全满足才提议（难逆转 / 无上下文费解 / 真实权衡）；`docs/adr/` 懒创建
- 格式：标题 + 1-3 句正文；可选节（Status/Considered Options/Consequences）按需，大多数不需要
- 编号：`0001-slug.md` 顺序递增，扫描最高号 +1
- 草稿经用户显式确认后落盘，任何情况无例外（见流程①）

### Spec 守则
- 完整七节模板逐节不缺：Problem Statement / Solution / User Stories / Implementation Decisions / Testing Decisions / Out of Scope / Further Notes
- User Stories：长编号列表，`As an <actor>, I want a <feature>, so that <benefit>` 格式
- Implementation Decisions：不含文件路径/代码片段；例外——原型产出的决策密集片段可 inline，注明来源并裁剪至决策部分
- 全文贯穿 glossary 词汇；尊重所触区域既有 ADR
- seams：既有优先于新建、取最高、理想数量 1，与用户确认
- 发布后标 `ready-for-agent`，triage 状态以 issue 文件顶部 `Status:` 行记录

## 回退

| 触发点 | 条件 | 动作 |
|--------|------|------|
| ② seam 确认 | 用户不同意 seams | → ① 补充 |
| ② 综合时 | 关键信息缺失 | → ① 补采 |
| ② 发布后 | spec 有问题 | → ① 重新循环 |

## 异常终止

| 情况 | 处理 |
|------|------|
| 用户中途放弃 / 无主题 | 终止 |
| tracker 未配置 | 提示 `/setup-matt-pocock-skills`，终止 |
| ① 超过 5 轮无进展 | 建议暂停或缩小范围 |

## 约束

- ① 出口达成后方可进入 ②
- 全程不写代码、不动源码：唯一允许写入的文件是领域文档（`CONTEXT.md`/ADR）与 spec
- ② 探索代码只为确认 seams 与术语——只读不改
- **写入 ADR 必须由用户显式确认，无论任何情况、无任何例外**：三条件全满足、决策看似显然、② 补记，均不豁免。ADR 一旦落盘记录不可撤销（可 supersede，但痕迹永存），全部门槛都在写入之前
- **ADR 与 glossary 不对称**：`CONTEXT.md` 术语可随访谈 inline 更新（domain-modeling 规则），ADR 必须先审草稿、用户确认后才落盘——禁止把 inline 逻辑套用到 ADR

## 引用

- [grill-with-docs](.agents/skills/grill-with-docs/SKILL.md)
- [grilling](.agents/skills/grilling/SKILL.md)
- [domain-modeling](.agents/skills/domain-modeling/SKILL.md)
- [to-spec](.agents/skills/to-spec/SKILL.md)
