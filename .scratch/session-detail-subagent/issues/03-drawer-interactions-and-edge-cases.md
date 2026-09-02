# 03: 详情交互与异常收尾

**What to build:** 抽屉的交互闭环与空态完备，主会话与子代理在各种边缘下仍表现一致且不闪动。

**Blocked by:** 02

**Status:** resolved

- [x] 头部 `displayName` 点击进入编辑态（复用 `rename-input` 样式与 `sanitizeName`/`ACTIVE_MS` 校验：活跃 409、非法 400、同名 409），成功后刷新抽屉与后列表
- [x] 异常空态：`sessionId` 不存在 → 抽屉内“会话不存在或已删除”+ 关闭按钮（API 404）；孤儿子代理不并入任何父（已在 01 判定）
- [x] 无请求会话：汇总 0 值展示（`UNPRICED`/`0%`），时间线“暂无计入口径请求”；无子代理会话隐藏开关与“含 N”提示
- [x] 滚动与性能：请求表容器 `max-height: 60vh` + `overflow:auto`，极端 >200 行仍可滚动（后续再分页）
- [x] 刷新：抽屉打开期间不参与 `autoRefresh` 的 `poll` 轮询，避免闪动；关闭后恢复
## 实施总结
- 提交：`0b1e395` — `feat(session-detail-subagent): 详情交互与异常收尾 (#03)`；文档：`57eda5c` — `docs: align README with session-detail-subagent drawer (#03)`
- 实现的 seams：
  - T1 Drawer header displayName 重命名：点击标题进入编辑态，复用 rename-input，Enter/Esc/blur 保存，复用 sanitizeName/ACTIVE_MS（409/400），成功后 re-fetch detail + renderDetailDrawer + refreshSessionTable/refreshRequestTable/refreshSessionGroups
  - T2 异常空态：抽屉内“会话不存在或已删除（id）”+ 关闭按钮，API 404 透传；孤儿 parentSessionId 指向不存在会话不合并（filter(parentSessionId===父id)）
  - T3 无请求会话：汇总 0 值展示 UNPRICED/0%/0，时间线“暂无计入口径请求”空态卡，无子代理隐藏开关与含 N 提示（hasChildren 条件）
  - T4 滚动与性能：detail-table-wrap style max-height:60vh + overflow:auto，>200 行全量无分页可滚动
  - T5 刷新：poll 首行 if(detailDrawerState.open) return 跳过，closeDrawer 后重置 open=false 恢复
- 验收标准：
  - [x] 头部 displayName 点击进入编辑态（复用 rename-input/ACTIVE_MS 校验，成功后刷新抽屉与后列表）— src/webui.html:675,675,1505,1541, startDetailRename/saveDetailRename
  - [x] 异常空态 404 + 孤儿隔离 — src/webui.html:1508-1520（404 文案+按钮）、src/session-data.ts:detailFromFiles filter
  - [x] 无请求会话 0 值与空态隐藏 — src/webui.html:1611（暂无计入口径请求）+ 汇总 UNPRICED/fmtRate(0) + hasChildren 条件
  - [x] 滚动 60vh — src/webui.html:1613 style max-height:60vh;overflow:auto
  - [x] 刷新 poll 暂停 — src/webui.html:1085 if(detailDrawerState.open) return
- 测试结果：6/6 全绿（test/28-drawer-interactions-edge.test.ts）；全量 25-28 相关 24/24 全绿，核心 84/84 全绿；opencode 客户端 5 项既有失败与本改动无关（基线已失败）
- typecheck：通过
- 文档对齐：README 已更新（术语 子代理、新增抽屉段落、API detail 端点）；CONTEXT 已由 #04 收敛无需追加
- 遗留 / 后续建议：无（极端 >200 行后续可分页；URL 深链 ?detail= 首版不支持按 spec 暂缓）

