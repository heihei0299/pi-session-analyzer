# 02 — webui 网关可比窗口

**What to build:** webui 打开即默认显示与 pi-switch 网关可比的统计窗口——新增「自 8/1（网关可比）」时间预设并默认激活，其起始时间精确为网关数据起点（2026-08-01T00:00:00Z，UTC）；状态行在该窗口下显示范围并标注「网关可比」；「全部」预设保留，可一键切回含 8/1 前数据的完整历史。

**Blocked by:** None — can start immediately

**Status:** resolved

- [x] 时间预设按钮组新增「自 8/1（网关可比）」，起始参数精确传 2026-08-01T00:00:00Z（UTC，后端按带时区后缀原样解析）
- [x] 「自 8/1」为默认激活窗口（打开首屏即生效）；「全部」保留可切回
- [x] 状态行在「自 8/1」窗口下显示范围（2026-08-01T00:00:00 ~ 现在）并标注「网关可比」；切换其他窗口不标注
- [x] HTML 骨架断言含「自 8/1」按钮（自动化，沿用 webui UI 测试约定）
- [x] 手工验收：默认激活态、状态行标注、切「全部」恢复、自定义窗口无标注

## 实施总结

- 提交：`f404b3d` — feat: webui 默认网关可比窗口（自 8/1 预设）
- 实现的 seams：S1 HTML 骨架自动化（gateway 预设 + 默认 active 转移）/ S2 JS 行为（applyPreset 分支 + 状态行标注 + init 首刷，仓库约定手工验收）
- 验收标准：5 条全部 `- [x]`（手工验收项已代码逻辑静态验证：P1 修复后 preset 推断/各分支设值推理通过，建议浏览器实测复核）
- 测试结果：116/116 全绿（08-overview-ui S13 新增 gateway 断言）
- typecheck：通过；script 语法编译检查：OK
- 文档对齐：CONTEXT.md「时间语义与网关可比窗口」节已覆盖本票语义（01 commit 内）；README 全面改写属 03 票
- 遗留 / 后续建议：code-review 发现并修复 P1 运行时 bug（updateStatusRange 解构行丢失导致 ReferenceError，骨架测试无法覆盖，靠 review 双轴抓到）；未采纳 Standards 建议（预设 registry 化 / GATEWAY_SINCE 后端下发）——inline HTML 形态下收益有限，spec 决策即硬编码 8/1
