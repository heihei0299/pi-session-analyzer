# 会话详细界面：子代理重命名与合并展示

`ready-for-agent`

## Problem Statement

当前 WebUI 没有**单会话详情**能力，且对子代理会话的呈现存在两处可用性/一致性问题：

1. **术语不一致**：pi 侧子代理（sub-agent）会话在 token-analyzer 中被标记为“任务”（`isTask`），徽标「任务」与汇总「含任务」与 pi 的“子代理”心智模型不符，搜索/沟通成本高。
2. **消耗分散**：子代理的 token/请求分散在独立的会话行與请求行中（`.../<父id>/tasks/<子id>.jsonl`），用户在排查单次会话成本时需手动把多个行加总，无法在**主会话视角**看到“本次会话到底花了多少（含子代理）”与请求时序。
3. **缺少详情入口**：现有 tab（总览/会话明细/请求明细/会话管理/OpenCode 对账）均为列表/聚合视图，无单会话钻取界面展示其时间、cwd、模型、token 构成与请求时间线。

数据事实：子代理会话存为 `<父sessionId>/tasks/<子id>.jsonl`，header 含 `parentSession: "<父id>"`（已在线上会话实测验证，例 `.../2026-08-30T21-26-21_01a05490.../tasks/..._01a054a3...jsonl` → `parentSession=01a05490...`）。`analyzeFile` 已解析该字段，`isTask = file.includes("/tasks/")`。

约束：列表层面的统计口径（totals/sessions/requests 的独立行）是**对账基线**，不可因详情合并而全局去重/隐藏子代理行，否则破坏与网关/导出的一致性与可追溯性。

## Solution

在 WebUI 增加**会话详细抽屉**，并完成两项变更：

1. **重命名（仅 UI 文案）**：将用户可见的“任务”改为“子代理”（徽标、汇总提示、tooltip），数据层字段 `isTask` 保持不变（注释补充“子代理 (isTask)”）。
2. **合并展示（仅详情视图）**：在详情抽屉内将归属该主会话的全部子代理**视图合并**——请求时间线按 `timestamp asc` 混排，汇总指标按 `主+Σ子代理` 求和（重算 `cacheRate`），列表/总览/导出保持独立行不变。

实现为前后端各一处 seam：后端新增 `GET /api/sessions/:id/detail`（聚合计算在 `SessionData` 深模块），前端新增抽屉组件（点击会话行进入）。

## User Stories

1. 作为用户，我点击会话明细中某会话的 `displayName`/`sessionId`，可滑出该会话的详情抽屉，看到其头部信息、Token 构成条与请求时间线，以便快速钻取单会话成本。
2. 作为用户，我在详情中默认看到**合并子代理后的汇总**（含子代理的 tokens/请求数/成本），并可通过开关“合并子代理（默认开）”切回仅主会话，以便对比。
3. 作为用户，我在详情的请求时间线中可区分每一行来自“主会话”还是“子代理（短 ID）”，子代理行有“子代理”徽标与淡色背景，以便还原真实时序。
4. 作为用户，我看到会话明细徽标与汇总提示已改为“子代理”/“含子代理”，与 pi 心智一致。
5. 作为用户，我在会话列表/请求列表中仍能看到子代理独立行（未被隐藏），以便对账与排障。
6. 作为用户，我在详情头部可点击 `displayName` 重命名该会话（复用现有重命名校验：活跃会话 409、非法名 400、同名 409），成功后抽屉与列表同步刷新。
7. 作为用户，当某会话无子代理时，详情不展示合并开关与“含 N 个子代理”提示，仅展示主会话自身数据。
8. 作为用户，当详情的 `sessionId` 不存在时，看到“会话不存在或已删除”空态而非白屏。

## Implementation Decisions

### 1. 术语迁移范围（Q1 结论）

- **仅 UI 文案**：替换用户可见文案 `任务 → 子代理`，`含任务`/`含子代理任务` → `含子代理`。
- 涉及点（webui.html）：
  - 会话/请求明细徽标：`<span>子代理</span>`（原“任务”在 `renderDetailTable` 的 `07m` 分支）
  - 会话明细汇总 `session-summary` 的 `含子代理` 提示（原 `有Task`）
  - tooltip/aria：`title="子代理会话"`。
- 数据层**不变**：`SessionFileData.isTask`、`SessionRowEnriched.isTask`、`api` 响应字段 `isTask` 均保留原名，注释改为 `/** 是否为子代理会话（路径含 /tasks/，历史称 isTask） */`。
- 测试描述同步改为“子代理”。

### 2. 详情抽屉交互（Q2/Q9 结论）

- **入口**：点击会话明细/请求明细行的 `displayName` 或 `sessionId`（整行点击不触发，避免与重命名冲突）。
- **形态**：右侧侧滑抽屉（drawer，宽度 720px，移动端全屏），遮罩点击/Esc/右上角×关闭；首版**不支持** URL 深链 `?detail=`（后续再加）。
- **来源页**：`tab-sessions` 与 `tab-requests` 均可触发；`tab-manage` 保持现有重命名行为，不触发抽屉。
- **重命名**：抽屉头部 `displayName` 可点击进入编辑态（复用 `startSessionRename` 逻辑与样式 `rename-input`），调用 `POST /api/sessions/rename`，成功后刷新抽屉数据与 `refreshSessionTable`/`fetchRows`。
- **空态**：
  - 无子代理：隐藏“合并子代理”开关与“含 N 个子代理”提示，展示“无子代理”。
  - 404：抽屉内展示 `会话不存在或已删除（{sessionId}）` + 关闭按钮。
  - 空请求（合法会话但 items 为空）：展示“暂无计入口径请求”。

### 3. 详情内容分区（Q6 结论）

抽屉自上而下 4 区：

1. **头部**：`displayName`（可编辑）、`sessionId`（等宽+复制按钮）、`timestamp`（`fmtTimestamp`）、`cwd`（`cwdNorm` + 原始 cwd tooltip）、`model`（单模型/混合 `mixed`）、`requests` 计数。
2. **Token 构成条**：复用总览 `tape` 样式，按合并后（或仅主）`input/cacheRead/output` 比例展示三段，hover 显示数值与百分比；下方 `tape-numbers` 展示 `fmtCompact` 三值。
3. **汇总卡**：6 指标 `totalTokens/requests/input+cacheRead/output/cacheRate/cost`（与总览卡一致）；小字“其中主会话 X tokens · 子代理 Y tokens（N 个）”仅当合并开且 N>0 时展示，细分仅 `totalTokens/requests/cost` 三项（Q11 结论 A）。
4. **请求时间线表**：列 = `timestamp/model/input/output/cache/reasoning/totalTokens/cost/来源`（来源列新增），行按 `timestamp asc`（Q10 结论 A，全量无分页）。`input` 列展示总输入（input+cacheRead，ticket 24 口径）；`cache` 列 = cacheRead+cacheWrite；子代理行淡背景（`#FFF6D6` 类）+ 单元格徽标 `子代理 <短ID 8>`。

> 首版不含：原始消息原文、`reasoning` 详情、按子代理分组视图。

### 4. 归属判定（Q4 结论）

- `SessionData.analyzeFile` 新增解析 `parentSessionId?: string`（header `parentSession` 非空字符串时取值；若缺失则 `undefined`）。
- `SessionFileData` 新增可选字段 `parentSessionId?: string`（与 `isTask` 并存）。
- 合并归属：`子.parentSessionId === 父.sessionId` 即归属该父；**不以**文件路径 `isTask` 作为归属依据（仅作徽标）。
- 孤儿：`parentSessionId` 指向不存在会话 → 不合并，仅在列表独立展示。

### 5. 合并口径（Q3 结论）

- **默认合并**：抽屉开关默认 `开`（每次打开重置为开，不记忆）。
- **汇总合并**：`mergedTotals = finalize( Σ主items + Σ子items )`，对 `Totals` 各累加字段（input/output/cacheRead/cacheWrite/reasoning/cost）求和后 `finalizeTotals` 重算 `cacheRate` 与 `totalTokens`（`totalTokens = input+cacheRead+output`，ADR-0002）。
- **请求数**：`requests = 主items.length + Σ子items.length`。
- **模型**：若主与子涉及多模型 → `mixed`；单模型一致则保留该 model。
- **时间线**：`requests = [...主requests, ...子requests].sort((a,b)=>a.timestamp.localeCompare(b.timestamp))`，每行附加 `source: "main" | "child"` 与 `sourceId: sessionId`。
- **切换**：开关关 → 仅展示主会话 `session` 与 `mainRequests`，汇总卡展示 `mainTotals`；小字隐藏。
- 列表/总览/API `totals/sessions/requests` **不合并**，子代理仍独立行（Q5 结论）。

### 6. 后端设计（Q7 结论：B 后端详情端点）

**新增端点**：`GET /api/sessions/:id/detail`（或 `GET /api/sessions/detail?sessionId=...` 二选一，优先 REST `:id`，与 `POST /api/sessions/rename` 风格一致；若路由冲突则用 query 形态）。

- **参数**：路径 `sessionId` 必填（UUID 尾缀）。
- **查询**：`filter` 复用 `filterFromParams` 的 `since/until/model/cwd`？否——详情为单会话钻取，**不接受**时间/模型过滤（避免歧义），仅按会话 ID 定位。
- **实现位置**：`src/session-data.ts` 新增 `SessionData.queryDetail(dir, sessionId)` 或 `detailFromFiles(files, sessionId)`；`src/api.ts` 新增 `handleApi` 分支。
- **逻辑**：
  1. `allFiles = await readSessionFilesCached(dir)`（复用文件级缓存）
  2. `parent = allFiles.find(f=>f.sessionId===sessionId)`；若无 → 404 `{ error:"Not Found", detail:"会话不存在: ..." }`
  3. `children = allFiles.filter(f=>f.parentSessionId===sessionId)`（按 parentSessionId 归属）
  4. `mainTotals = totalsFromFiles([parent])`；`mergedTotals = totalsFromFiles([parent, ...children])`
  5. `mainRequests = requestRowsFromFiles([parent])`；`allRequests = requestRowsFromFiles([parent, ...children])` + 附加 `source` 标记后按 timestamp 排序
  6. `sessionEnriched = { ...sessionRow, fileName, displayName: displayNameOf(...), cwdNorm, isTask }`（复用 `query` 的 enriched 逻辑）
  7. 返回体：
     ```json
     {
       "session": { "sessionId","timestamp","cwd","cwdNorm","fileName","displayName","model","isTask", ...totals },
       "children": [ { "sessionId","fileName","displayName","timestamp","cwdNorm","isTask", ...totals } ],
       "totals": { "main": TotalsObject, "merged": TotalsObject, "childrenCount": N },
       "requests": [ { ...RequestRow, "displayName","source":"main|child","sourceSessionId" } ],
       "meta": { "hasChildren": true/false }
     }
     ```
- **序列化**：复用 `serialize.ts` 的 `totalsToObject`/`sessionToObject`/`requestToObject`，新增字段 `displayName/cwdNorm/fileName/source` 直接透传。
- **错误**：404（不存在）、400（sessionId 缺失/非法）、500（读取异常）。
- **缓存**：复用 `dirCache`；详情不引入额外缓存层。

**备选路由**：若 `/:id/detail` 与静态资源冲突，降级为 `GET /api/sessions/detail?sessionId=<id>`。

### 7. 前端实现

- **样式**：抽屉 `.session-detail-drawer`（fixed right 0，宽 720px，背景 `var(--enamel)`，阴影 `0 0 40px rgba(0,0,0,.5)`，动画 `translateX` 0.28s `cubic-bezier(.16,1,.3,1)`），遮罩 `.drawer-mask`（`rgba(0,0,0,.45)`）。
- **状态**：`detailState = { open: bool, sessionId: string|null, data: DetailResponse|null, merge: true }`。
- **加载**：`fetchDetail(sessionId)` → `GET /api/sessions/${id}/detail`，loading 态“加载中…”。
- **渲染**：
  - 头部：复用 `fmtTimestamp`/`escapeHtml`/`displayName`，`displayName` 点击进入 `startDetailRename`（复用 `sanitizeName`/`ACTIVE_MS` 校验）。
  - Tape：复用 `renderCards` 的 tape 逻辑，数据取 `merge ? merged : main`。
  - 汇总卡：6 卡片 `totalTokens/requests/input/output/cacheRate/cost` 取 `merge ? merged : main`，小字按 Q11。
  - 请求表：`detail-table-wrap` 复用表样式，列在 `REQUEST_COLS` 基础上追加 `source` 列；行按 `timestamp asc` 已排好，无分页控件（Q10 结论）。
- **开关**：`checkbox` “合并子代理” 默认 checked，`change` 时重渲染（不重新 fetch）。
- **事件绑定**：在 `bindDetailInteractions` 中为 `#session-body`/`#request-body` 的 `displayName/sessionId` 单元格委托 `click` 触发抽屉；`bindSessionManage` 保持不变（manage 页不触发抽屉）。
- **刷新**：详情打开期间 `poll` 不自动刷新详情（避免抖动）；关闭后再参与常规 `poll`。

### 8. 排序/分页（Q10 结论）

- 详情请求时间线**不分页**、**固定按 timestamp asc**（还原真实时序），无排序控件。
- 若单会话合并后请求数 >200（极端），前端仍全量展示（可滚动，`max-height: 60vh` + `overflow:auto`），后续再补分页。

### 9. 异常与校验

| 场景 | 行为 |
|---|---|
| `sessionId` 不存在 | 抽屉展示空态“会话不存在或已删除” + 关闭按钮；API 404 |
| 孤儿子代理 | 不并入任何父，列表独立展示 |
| 活跃会话重命名 | 409 “会话活跃中，稍后再试”（复用 `ACTIVE_MS`） |
| 非法显示名 | 400 “显示名非法（去除非法字符后为空）” |
| 无请求会话 | 汇总 0 值展示 `UNPRICED`/`0%`，时间线空态提示 |

### 10. 文档与术语

- `CONTEXT.md` 在“数据域”增补：**子代理会话** = `isTask` 会话（路径含 `/tasks/`，header 含 `parentSession`），其消耗在详情视图合并到主会话，主列表保持独立。
- `docs/adr/` 若需可增 `0003-session-detail-subagent-merge.md`（可选，非必须）。
- `serialize.ts`/`aggregate.ts` 无需变更。

## Testing Decisions

- **唯一 seam**：`SessionData.queryDetail`（或 `detailFromFiles`）+ `handleApi` 的 `/api/sessions/:id/detail` 分支 + 前端抽屉渲染（手工验收）。
- **单元/集成**（`--test`）：
  1. `isTask` 识别：含 `/tasks/` 与不含的会话 `isTask` 正确
  2. `parentSessionId` 解析：header 含 `parentSession` 时正确提取，缺失时 `undefined`
  3. 归属：`children = filter(parentSessionId===父id)`，孤儿不归入
  4. 合并 totals：`merged = main+children 求和后 finalize`，`cacheRate = cacheRead/(input+cacheRead)` 重算
  5. 请求合并：`requests` 按 timestamp asc 排序，`source` 标记正确
  6. 开关语义：`merge=true` 返回 merged，`merge=false` 等价于 main（前端切换不重新 fetch 亦可由后端 `?merge=` 支持，可选）
  7. 404：不存在 id 返回 404
  8. 重命名：详情重命名复用已有校验（活跃/非法/同名）
- **前端手工验收**（`webui.html` 无自动化 seam，已有约束）：
  - 会话明细/请求明细点击 `displayName`/`sessionId` 滑出抽屉，展示四区
  - 徽标“子代理”与汇总“含子代理”生效
  - 有子代理会话：开关默认开，汇总小字“其中主…·子代理…（N 个）”正确；关开关后仅主会话
  - 无子代理会话：开关隐藏
  - 子代理行淡背景+“子代理”徽标+短 ID
  - Tape 比例随合并开关联动
  - 重命名成功后抽屉与列表同步
  - 遮罩/Esc/× 关闭

## Scope & Out of Scope

- **In**：UI 重命名、详情抽屉、视图合并、后端详情端点、重命名复用。
- **Out**：URL 深链 `?detail=`、按子代理分组表、原始消息原文展示、列表/总览/导出的全局去重隐藏、全量子代理聚合到网关、fileName 字段重命名。

## Decisions Log

| 日期 | 决策 |
|---|---|
| 2026-09-01 | Grilling Round1 Q1-Q5 全采纳推荐：仅 UI 重命名、侧滑抽屉、仅视图合并、`parentSession` 主归属、列表保持独立 |
| 2026-09-01 | Round2 Q6-Q9 全采纳：四区精简、后端详情端点、文案三处+tooltip、displayName/sessionId 触发+默认合并 |
| 2026-09-01 | Round3 Q10-Q12 全采纳：全量 asc 无分页、总量级细分、抽屉内支持重命名 |

## Risks

- 子代理数量极端多（>50）时抽屉请求表滚动高度大 → 以 `max-height` 滚动容器缓解，后续再分页。
- `parentSession` 缺失的旧子代理（历史数据）无法归属 → 接受为独立行，不合并。
- 活跃会话重命名 409 在抽屉内需友好提示（Toast/错误行）。

## Implementation Notes

- 后端 `SessionData` 新增字段 `parentSessionId` 为可选，旧数据无该字段时保持 `undefined`，不破坏现有缓存快照。
- 前端抽屉复用现有 `escapeHtml`/`fmtTimestamp`/`fmtCompact`/`fmtCost`/`fmtRate` 与表样式，避免新样式体系。
- 抽屉不参与 `autoRefresh` 的 `poll` 轮询，避免抽屉内数据闪动。

## 实施状态

- 待实现（tdd-implement 按 Testing Decisions 逐条落地）
