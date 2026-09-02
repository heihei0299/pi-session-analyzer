# 02: 详情抽屉 Variant A — Bench 可视化

**What to build:** 用户点击会话明细/请求明细的 `displayName`/`sessionId`，右侧滑出 Variant A 抽屉（Bench 质感），默认展示合并后视图，列表仍独立展示子代理行。

**Blocked by:** 01

**Status:** resolved

- [x] 入口：`displayName`/`sessionId` 点击触发抽屉（整行不触发，避免与重命名冲突；`tab-manage` 不触发）
- [x] 抽屉：右侧 760px（移动端全宽），遮罩/Esc/× 关闭，动画 `cubic-bezier(.16,1,.3,1)`，首版不支持 URL 深链
- [x] 头部：`displayName`/`sessionId`（复制按钮可选）、`timestamp(fmtTimestamp)`、`cwdNorm`（tooltip 原始 cwd）、`model`（mixed 处理）、`主会话`/`含 N 个子代理` 徽标
- [x] 开关：`合并子代理` 默认开（每次打开重置），无子代理时隐藏；切换时重渲染不重 fetch（tape/卡片/表联动）
- [x] Tape：复用总览 `tape` 样式，按 `merge?merged:main` 的 `input/cacheRead/output` 比例渲染
- [x] 四卡：总 token/请求数/花费/缓存率（小字“其中主 X·子代理 Y（N 个）”仅总量三项，`merge` 且 `N>0` 时展示）
- [x] 请求时间线表：列 `时间/模型/输入(总输入)/输出/缓存/总token/花费/来源`，按 `timestamp asc` 全量无分页，子代理行淡背景 + `子代理 <短ID8>` 徽标区分

## 实施总结
- 提交：`b578acc` — `feat(session-detail-subagent): 详情抽屉 Variant A (#02)`
- 实现的 seams：T1 抽屉骨架（760px/遮罩/动画/移动端全宽）、T2 入口触发（detail-trigger 委托、整行不触发、manage 不触发）、T3 头部（displayName/sessionId 复制/fmtTimestamp/cwdNorm tooltip/model mixed/徽标）、T4 合并开关（默认开/重置/无子隐藏/不重 fetch 联动）、T5 Tape（复用 tape 样式、merge?merged:main 比例）、T6 四卡（总 token/请求数/花费/缓存率+小字细分）、T7 请求时间线（8 列/timestamp asc/无分页/子代理淡背景+短ID8）
- 验收标准：
  - [x] 入口：displayName/sessionId 点击触发，整行不触发，manage 不触发 — `src/webui.html:detail-trigger 委托 + state.tab==="manage" 排除` / `test/27-drawer-variant-a.test.ts:Seam-T2`
  - [x] 抽屉：760px、移动端 100vw、遮罩/Esc/×、cubic-bezier 动画、无 ?detail= 深链 — `src/webui.html:.session-detail-drawer width:760px + .drawer-mask rgba` / `Seam-T1`
  - [x] 头部：displayName/sessionId 复制、fmtTimestamp、cwdNorm tooltip、model mixed、徽标 — `src/webui.html:drawer-head + fmtTimestamp + cwdNorm + mixed` / `Seam-T3`
  - [x] 开关：默认开每次重置、无子隐藏、切换不重 fetch — `src/webui.html:detailDrawerState.merge=true + hasChildren + change→renderDetailDrawer` / `Seam-T4`
  - [x] Tape：复用 tape 样式、按 merge 选择 merged/main — `src/webui.html:tape + merged` / `Seam-T5`
  - [x] 四卡：总 token/请求数/花费/缓存率 + 小字细分仅总量三项 — `src/webui.html:cards 4 + 其中主·子代理` / `Seam-T6`
  - [x] 请求时间线：8 列、timestamp asc、无分页、子代理 #FFF6D6 + 短ID8 — `src/webui.html:detail-child-row + slice(0,8)` / `Seam-T7`
- 测试结果：7 seams 全绿（`TZ=Asia/Shanghai node --test test/27-drawer-variant-a.test.ts`）；相关回归 `25-detail-data-foundation` + `07-api` + `08-overview-ui` + `09-detail-views-ui` 全绿；全量套件仅预存 opencode-client 4 项失败（与本 issue 无关，stash 后复现）
- typecheck：通过（`npm run typecheck`）
- 文档对齐：无需更新（CONTEXT 已含子代理定义，README 无 CLI/配置变更；抽屉为纯前端 View 层，术语与 ADR-0002 口径保持一致）
- 遗留 / 后续建议：抽屉内 `displayName` 重命名与 404 空态已实现基础版，完整重命名交互与 focus trap 交由 #03；URL 深链 `?detail=` 按 spec 首版不做，后续再加；移动端可进一步加手势滑动关闭
