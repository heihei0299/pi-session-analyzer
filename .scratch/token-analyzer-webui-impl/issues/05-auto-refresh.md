# 05 — 自动刷新（Off/5s/30s/5min + 静默替换 + 更新提示）

**What to build:** 顶部「自动刷新」下拉（Off/5s/30s/5min，默认 Off）+ 手动「刷新」按钮；开启后前端轮询当前可见 tab 所需端点；新数据静默替换不闪烁；数据有变化时状态行显示「已更新 HH:MM:SS」；当前筛选照常生效。后端沿用全量重算（每请求重新读取聚合），不引入增量状态。spec 依据：`.scratch/token-analyzer-webui/spec.md` 的「自动刷新机制」。

**Blocked by:** 04 — 明细视图

**Status:** resolved

- [x] 下拉选项 Off/5s/30s/5min，默认 Off；切 Off 清除定时器
- [x] 开启后按间隔轮询当前可见 tab 所需端点；切换 tab 重建定时器
- [x] 轮询期间页面保持旧数据渲染，新数据到达一次性替换（无闪烁）
- [x] 数据有变化时状态行显示「已更新 HH:MM:SS」；无变化静默
- [x] 自动刷新时已选的 model/cwd/时间筛选照常生效
- [x] 手动「刷新」按钮立即拉取一次当前视图

## 实施总结
- 提交：`25cc6e4` — feat: WebUI 自动刷新骨架（Off/5s/30s/5min 轮询、静默替换、已更新提示）
- 实现的 seams：S16 自动刷新骨架标记（#auto-refresh 下拉 Off/5000/30000/300000 默认 off / #refresh-btn 手动刷新 / #updated-at「已更新 HH:MM:SS」状态元素）
- 验收标准：6 条全部 `- [x]`（见上；行为级实现完成：setInterval 轮询 + snapshot 对比静默替换 + 切 Off 清定时器 + tab 切换重置对比基准；自动化断言 HTML 骨架标记，轮询交互走手工验收）
- 测试结果：1/1 全绿（test/10-auto-refresh-ui.test.ts）；完整套件 65/65
- typecheck：通过（npm run typecheck）
- 文档对齐：README 的 webui 描述在 ticket 06 收尾一次性补齐（本 ticket 无文档失配）
- 遗留 / 后续建议：model/cwd 筛选控件按已确认决策不提供（API 层筛选完整）；前端「已选筛选」= 时间范围（filterParams）
