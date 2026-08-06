# 05 — 导出中状态

**What to build:** 点击导出（JSON 或 CSV）后，两个导出按钮立即切换为「导出中…」并禁用，直到导出完成恢复；导出失败时恢复按钮并经错误横幅提示。全量导出数据量较大（约 28MB）时用户有明确的进行中反馈，不会重复点击。

**Blocked by:** None — can start immediately

**Status:** ready-for-agent

- [ ] 导出触发瞬间两个按钮变「导出中…」且 disabled
- [ ] 导出完成后按钮恢复可点
- [ ] 导出失败时按钮恢复且错误横幅显示失败原因
- [ ] 导出文件内容与修复前一致（totals/sessions/requests 三段、随当前筛选）
