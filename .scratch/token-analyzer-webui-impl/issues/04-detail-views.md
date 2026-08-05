# 04 — 明细视图（会话明细 + 请求明细 tab，分页排序）

**What to build:** 在前端骨架（03 的 tab 结构）上补齐「会话明细」与「请求明细」两个 tab：各自列定义按 spec，分页下拉（20/50/100），点击列头排序，请求明细默认按时间倒序。spec 依据：`.scratch/token-analyzer-webui/spec.md` 的「前端 UI 布局」明细表部分。

**Blocked by:** 03 — 总览视图

**Status:** resolved

- [x] 会话明细 tab：列 = 会话ID/时间/cwd/模型/请求数/输入/输出/缓存率/总token/花费（数据源 /api/sessions）
- [x] 请求明细 tab：列 = 会话ID/时间/模型/输入/输出/缓存/推理/缓存率/总token/花费，默认时间倒序（数据源 /api/requests）
- [x] 两表均支持分页下拉 20/50/100 行
- [x] 点击列头可升降序排序
- [x] tab 切换即时生效（每次切换拉取对应端点，无整页刷新）

## 实施总结
- 提交：`7410987` — feat: WebUI 明细视图骨架（会话/请求表头、分页 20/50/100、列头排序、请求默认时间倒序）
- 实现的 seams：S15 明细 tab 骨架标记（会话表头 10 列 / 请求表头 10 列（spec 列清单）/ 分页 20/50/100 / th.sortable+arrow 排序标记 / request-head data-sort=timestamp+data-dir=desc 默认倒序 / fetchRows("sessions"|"requests") tab 切换端点）
- 验收标准：5 条全部 `- [x]`（见上；行为级实现完成，自动化断言 HTML 骨架标记，分页/排序/切换交互走手工验收）
- 测试结果：1/1 全绿（test/09-detail-views-ui.test.ts）；完整套件 64/64
- typecheck：通过（npm run typecheck）
- 文档对齐：README 的 webui 描述在 ticket 06 收尾一次性补齐（本 ticket 无文档失配）
- 遗留 / 后续建议：验收清单「请求明细表头 11 列」与 spec/ticket 正文列清单（会话ID/时间/模型/输入/输出/缓存/推理/缓存率/总token/花费 = 10 项）不一致，判定为清单笔误，按正文 10 列实现
