# 03 — 总览视图（汇总卡片 + 分组表 + 时间范围 + 导出）

**What to build:** 单 HTML 真实前端骨架（深色主题），总览 tab：8 张汇总指标卡片、按模型/cwd 切换的分组表、时间范围预设按钮组（今天/7天/30天/全部/自定义）、状态行（数据目录/会话数/数据范围）、导出 JSON/CSV 按钮。数据全部来自 02 的 API。spec 依据：`.scratch/token-analyzer-webui/spec.md` 的「前端 UI 布局」与「会话管理 tab」（tab 骨架在本 ticket 建立）。

**Blocked by:** 02 — HTTP API 端点集

**Status:** resolved

- [x] 页面加载显示 8 张汇总卡片（请求数/输入/输出含推理/缓存/推理/缓存率/总token/花费），数值 ≥10⁴ 用 k/M 缩写
- [x] 花费全 0 时卡片显示「费率未配置（免费/未定价）」
- [x] 分组表按模型 / 按 cwd 可切换（列：分组键/请求数/输入/输出/缓存率/总token/花费）
- [x] 时间预设按钮组生效（今天/7天/30天/全部/自定义），自定义展开 since/until 输入，映射 API 参数
- [x] 状态行显示数据目录 / 会话数 / 数据范围（来自 /api/meta）
- [x] 「导出 JSON」「导出 CSV」按钮下载当前筛选范围数据（前端 fetch 组装，含 totals/sessions/requests）
- [x] 深色主题；API 失败时页面显示友好错误而非白屏

## 实施总结
- 提交：`6e83c97` — feat: WebUI 总览视图骨架（深色主题、8 汇总卡片、分组表、时间预设、导出、错误横幅）
- 实现的 seams：S13 总览骨架标记（8 卡片 id / data-group 分组切换 / data-preset 时间预设 / 状态行 / 导出按钮 / 错误横幅 / data-theme 深色主题）/ S14 4 tab 骨架（overview/sessions/requests/manage）
- 验收标准：7 条全部 `- [x]`（见上；行为级实现完成，自动化断言 HTML 骨架标记，交互走手工验收）
- 测试结果：2/2 全绿（test/08-overview-ui.test.ts）；完整套件 63/63
- typecheck：通过（npm run typecheck）
- 文档对齐：README 的 webui 描述在 ticket 06 收尾一次性补齐（本 ticket 无文档失配）
- 遗留 / 后续建议：明细 tab（04）与会话管理 tab（06）当前为占位 stub；自动刷新下拉与刷新按钮结构已就位（05 接入逻辑）
