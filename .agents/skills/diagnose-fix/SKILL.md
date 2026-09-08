---
name: diagnose-fix
description: "Complete diagnosis→fix→regression channel for bugs: diagnose, then fix via a TDD red-green loop with a hard gate (no fix code before a failing regression test). Use when the user says diagnose/debug/fix this, or reports something broken/throwing/failing/slow — prefer this over diagnosing-bugs when a fix is wanted, not just a diagnosis."
---

# Diagnose Fix

本 skill 只连接诊断、TDD 修复和回归三个阶段。诊断细节以 [`diagnosing-bugs`](.agents/skills/diagnosing-bugs/SKILL.md) 为事实源，红绿语义以 [`tdd`](.agents/skills/tdd/SKILL.md) 为事实源；不复制两个上游的完整步骤。

## 流程

### ① 诊断

委托 `diagnosing-bugs` 完成 Phase 1–4：建立能捕捉用户症状的 tight feedback loop，实际复现并最小化，再形成可证伪的假设并按单变量探针验证。

出口：反馈回路已实际变红，且最小复现已确认；此阶段不写修复代码。

### ② TDD 修复

1. 在正确的公共 seam 上提出回归测试边界，并遵循 `tdd` 要求获得用户确认；只确认本次 seam，不建立重型 seam ledger。
2. 加载 `tdd`，先把最小复现转成失败回归测试并实际看到 Red。
3. 只写让该测试 Green 的最小修复，并遵循 `tdd` 的红绿循环。

**硬门槛：**不存在已实际观察到的失败回归测试时，不得写任何修复代码。不存在正确 seam 时，本身就是 finding；记录架构阻塞，不绕过测试直接修改。

出口：回归测试先 Red，最小修复后 Green；不进入 `tdd-implement` 的长流程。

### ③ 回归收尾

按 `diagnosing-bugs` 的收尾要求重跑阶段 ① 的原始、未最小化反馈回路，确认用户症状消失；清理 `[DEBUG-...]` 探针和一次性 harness，并记录最终验证的假设。

出口：原始症状消失、回归测试 Green、临时诊断产物已清理。

## 回合连续性

诊断 → seam 确认 → Red → 修复 → Green → 原始回路 → 清理在一个回合内连续推进。只在用户必须确认 seam、发现无 seam finding、外部环境阻塞或整个阶段出口时暂停；预告下一步后立即执行。

## 引用

- 诊断：[`diagnosing-bugs`](.agents/skills/diagnosing-bugs/SKILL.md)
- 修复：[`tdd`](.agents/skills/tdd/SKILL.md)
- 负向边界：[`references/anti-patterns.md`](references/anti-patterns.md)
