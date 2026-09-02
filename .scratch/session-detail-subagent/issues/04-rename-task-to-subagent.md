# 04: 术语收敛 — 任务 → 子代理 全链路

**What to build:** 用户在所有可见文案中看到“子代理”而非“任务”，心智与 pi 一致；数据层字段 `isTask` 保持不变以保兼容。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] 会话/请求明细徽标“任务”→“子代理”（`renderDetailTable` 分支、`sessionRowHtml` 若有），汇总提示“含任务/含子代理任务”→“含子代理（N）”
- [x] Tooltip/aria：`title="子代理会话"`，`session-summary` 的 `title` 同步
- [x] 代码注释：`SessionFileData.isTask` 注释改为“是否为子代理会话（路径含 /tasks/，历史称 isTask）”，API 响应注释同
- [x] 测试描述：含 “task” 的用例标题改为“子代理”，断言保持 `isTask` 字段名
- [x] 文档：`CONTEXT.md` 数据域增“子代理会话”定义（`isTask` 会话，`parentSessionId` 指向主会话，详情视图合并到主，主列表保持独立）；API 字段 `isTask` 不改名
## 实施总结
- 提交：`c18f1c2` — `feat(session-detail-subagent): 术语收敛 (#04)`
- 实现的 seams：徽标“子代理”+title \| 汇总“含子代理（N）”+title 同步 \| 注释幂等（isTask 历史称） \| CONTEXT 子代理会话定义 \| API isTask 保持 + 测试描述子代理化
- 验收标准：
  - [x] 会话/请求明细徽标“任务”→“子代理”（`renderDetailTable`，含 title="子代理会话"） — `src/webui.html:945`
  - [x] 汇总提示“含任务/含子代理任务”→“含子代理（N）” — `src/webui.html:968`（`subCount` 计数，N=当前页子代理数）— `src/webui.html:967-969`
  - [x] Tooltip/aria：`title="子代理会话"` — `src/webui.html:945`；`session-summary` title 同步为“含子代理” — `src/webui.html:969`
  - [x] 代码注释：`SessionFileData.isTask` 已为“是否为子代理会话（路径含 /tasks/，历史称 isTask）” — `src/session-data.ts:39`（01 已更新，本次幂等）
  - [x] 测试描述：新增 `test/26-rename-task-to-subagent.test.ts` 用“子代理”命名，断言保持 `isTask` — 5 用例全绿
  - [x] 文档：`CONTEXT.md:13` 新增子代理会话定义（isTask + parentSessionId + 详情合并/列表独立）
- 测试结果：相关 11 用例全绿（`25-detail-data-foundation` 6 + `26-rename` 5）；全量 262 用例中 256 绿、6 失败为 opencode 客户端预存故障（与本 issue 无关，基线 9 失败中 3 已由本次修复）
- typecheck：通过
- 文档对齐：`CONTEXT.md` 已更新；`README.md` 仍含“含任务”旧文案但按本 issue 边界不纳入（仅 CONTEXT 为必改，README 留后续 issue 统一术语）
- 遗留 / 后续建议：`README.md:86,92` 仍有“任务/含任务”描述，建议随后续术语全量收敛一并更新；`dist/` 为构建产物（.gitignore），`npm run build` 后自动同步，无需手改；`sessionRowHtml` 当前无徽标，若后续在会话管理页增子代理徽标需同步为“子代理”

