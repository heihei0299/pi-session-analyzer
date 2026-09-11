# 05 — 导出中状态

**What to build:** 点击导出（JSON 或 CSV）后，两个导出按钮立即切换为「导出中…」并禁用，直到导出完成恢复；导出失败时恢复按钮并经错误横幅提示。全量导出数据量较大（约 28MB）时用户有明确的进行中反馈，不会重复点击。

**Blocked by:** None — can start immediately

**Status:** resolved

- [x] 导出触发瞬间两个按钮变「导出中…」且 disabled
- [x] 导出完成后按钮恢复可点
- [x] 导出失败时按钮恢复且错误横幅显示失败原因
- [x] 导出文件内容与修复前一致（totals/sessions/requests 三段、随当前筛选）

## 实施说明

- `exportData` 增加 `setBusy` 辅助闭包：触发时立即将 `#export-json` 与 `#export-csv` 设为 `disabled = true` 且文案切换为「导出中…」。
- 接口请求与文件下载包在 `try...finally` 块中，成功完成或异常失败时在 `finally` 中统一恢复按钮文案与可点状态。
- 异常时通过既有 `showError("导出失败: " + e.message)` 输出错误横幅。
- 自动化测试见 `test/43-webui-fixes-04-05.test.ts`。
