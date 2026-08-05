# Token Analyzer WebUI — 功能规格（Spec）

**状态**: ready-for-agent（本 spec 由 wayfinder effort `token-analyzer-webui` 综合全部已决 ticket 产出，供后续 session 按此实现 webui；本 effort 不写实现代码）

**前置**: 依赖 token-analyzer CLI 的既有实现（`src/analyze.ts` / `aggregate.ts` / `serialize.ts` / `watch.ts` / `cli.ts`），webui 只做展示层与会话管理，不重写统计逻辑。数据域与统计口径 A 见 `.scratch/token-analyzer/spec.md`。

---

## Problem Statement

token-analyzer CLI 已能输出 token 消耗的表格/JSON/CSV 统计，但只有终端形态：需要浏览器可视化的统计面板（汇总卡片、分组表、明细表、时间趋势、自动刷新），并希望**按项目（cwd）组织会话、重命名会话**（当前会话文件名是 `<时间戳>_<UUID>.jsonl`，不可读，无法管理）。用户需要：一条 `serve` 命令启动本地 Web 服务，浏览器查看/筛选统计、按项目浏览会话、重命名会话。

## Solution

在现有 CLI 增加 `serve` 子命令，启动零依赖 HTTP 服务器（Node 原生 `http` + 单 HTML 内联前端），浏览器访问展示统计面板与会话管理：

- **四个视图（tab）**：总览（汇总卡片 + 分组表）/ 会话明细 / 请求明细 / 会话管理（按 cwd 项目分组）
- **交互**：时间范围预设按钮组、自动刷新下拉、导出 JSON/CSV、明细表分页排序、会话重命名
- **技术栈**：零运行时依赖、Node 24 type-stripping 直跑 `.ts`、原生 HTML/CSS/JS 单文件内联、无构建步骤
- **UI 形态参考**：pi-switch 统计页（`http://127.0.0.1:43110/` 的 📊 面板：汇总卡片 + 分组表 + 明细表 + 时间范围按钮 + 自动刷新下拉 + 导出按钮）

### 运行形态（ticket 01）

- 命令：`node src/cli.ts serve [--port <n>] [--host <h>] [--dir <path>]`（serve 为子命令，与窗口参数同层；serve 模式下窗口参数非法，校验拒绝）
- 默认端口 **50080**、默认 host **127.0.0.1**（仅本机）、`--dir` 继承 CLI 默认 `~/.pi/agent/sessions/`
- 不引入 `--interval`（自动刷新间隔由前端控件控制）
- 静态资源：**单 HTML 内联**（CSS/JS 全部内嵌），服务端只 serve 一个 index.html
- 生命周期：端口占用 → `EADDRINUSE` 友好提示（「端口 X 已被占用，可用 --port 更换」）；启动打印访问 URL；Ctrl+C 优雅退出

### HTTP API（ticket 02）

多端点细粒度，裸 JSON 响应，复用既有 `serialize.ts` 字段结构（驼峰命名，cacheRate 为 0-1 小数）：

| 端点 | 参数 | 响应 |
|---|---|---|
| `GET /api/totals` | model/cwd/since/until | `Totals` 对象 |
| `GET /api/sessions` | model/cwd/since/until | `{ window, rows: SessionRow[] }`，每行含 fileName（ticket 06 补充） |
| `GET /api/requests` | model/cwd/since/until | `{ window, rows: RequestRow[] }` |
| `GET /api/groups` | by=model\|cwd\|model,cwd + 筛选 | `{ window, by, rows: GroupRow[] }` |
| `GET /api/period` | period=day\|week\|month + 筛选 | `{ window, period, rows: PeriodRow[] }` |
| `GET /api/meta` | 无 | `{ dir, sessionCount, dataRange: { since, until } }` |
| `POST /api/sessions/rename` | body `{ sessionId, name }` | 成功 `{ ok, fileName }`；见会话管理 |

- 筛选参数 `model` / `cwd` / `since` / `until` **直接映射 CLI 语义**（since/until 闭区间、cwd 规范化比较、按会话时间戳归属），复用既有 `filterFiles`
- 错误处理：统一 JSON 错误体 `{ error, detail }` + HTTP 状态码（400 参数非法 / 404 未知路径或会话不存在 / 409 活跃或重名 / 500 数据目录不可读或无合法会话）
- 服务端每次请求全量 `readSessionFiles(dir)` 后按参数过滤/分组/汇总（212 会话秒级处理，无常驻状态）

### 前端 UI 布局（ticket 03）

单页 SPA，四个 tab：**总览 / 会话明细 / 请求明细 / 会话管理**。

- **总览**：8 张汇总指标卡片（请求数 / 输入 / 输出(含推理) / 缓存(cacheRead+cacheWrite) / 推理 / 缓存率 / 总 token / 花费）；数值格式化 ≥10⁴ 用 k/M 缩写；花费全 0 显示「费率未配置（免费/未定价）」；分组表按模型/cwd 切换（列：分组键/请求数/输入/输出/缓存率/总token/花费）；「导出 JSON / 导出 CSV」按钮（前端 fetch 组装下载，不引入服务端下载端点）
- **会话明细**：列 = 会话ID/时间/cwd/模型/请求数/输入/输出/缓存率/总token/花费
- **请求明细**：列 = 会话ID/时间/模型/输入/输出/缓存/推理/缓存率/总token/花费；默认按时间倒序
- 明细表分页下拉（20/50/100），点击列头排序
- **时间范围控件**：按钮组 今天 / 7天 / 30天 / 全部 / 自定义（默认「全部」）；「自定义」展开两个 datetime-local（since/until）映射 API 参数
- **自动刷新**（ticket 04）：顶部「自动刷新 ▾」下拉（Off/5s/30s/5min，默认 Off）+ 手动「刷新」按钮
- **视觉**：深色主题（仿参考页 zinc 暗色系），顶部标题 + 数据目录/会话数/数据范围状态行（数据源 `/api/meta`）
- 布局示意（原型）：

```
┌──────────────────────────────────────────────────────────┐
│ Token Analyzer WebUI         [总览][会话明细][请求明细][会话管理] │
├──────────────────────────────────────────────────────────┤
│ [今天][7天][30天][全部][自定义…]      [自动刷新▾][刷新]   │
│ 数据目录: ~/.pi/agent/sessions/ · 会话数: 212 · 范围: … │
├──────────────────────────────────────────────────────────┤
│ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐           │
│ │请求数│ │ 输入 │ │ 输出 │ │ 缓存 │ │ 推理 │  …卡片行…  │
│ └──────┘ └──────┘ └──────┘ └──────┘ └──────┘           │
│ [缓存率] [总 token] [花费]（零花费标「费率未配置」）      │
├──────────────────────────────────────────────────────────┤
│ 分组表：按模型 ▾ | 按 cwd ▾        [导出 JSON][导出 CSV] │
├──────────────────────────────────────────────────────────┤
│ （明细/会话管理 tab）表格 + 分页 + 排序 / 项目分组重命名  │
└──────────────────────────────────────────────────────────┘
```

### 自动刷新机制（ticket 04）

- **后端**：全量重算——每次请求重新读取+聚合，**不**复用 `watch.ts` 的 `IncrementalReader`（增量状态与多端点设计冲突）
- **间隔**：Off / 5s / 30s / 5min，默认 Off；前端 `setInterval` 轮询当前可见 tab 所需端点，切换 tab 重建定时器
- **筛选组合**：自动刷新时当前筛选**照常生效**（全量重算天然支持；与 CLI `--watch` 不支持筛选形成对照，实现勿误对齐）
- **浏览器侧**：轮询期间保持旧数据渲染不闪烁，新数据到达一次性替换；有变化时状态行显示「已更新 HH:MM:SS」

### 会话管理 tab（ticket 06）

- 新增第四个 tab：**会话管理**——按 cwd 项目分组展示全部会话（每组标题 = 规范化 cwd；组内每行 = 显示名/时间/模型/请求数/总token；组可折叠；行内「重命名」按钮）
- **重命名机制**：改文件名 `<时间戳>_<UUID>.jsonl` → `<显示名>_<UUID>.jsonl`（**保留尾 UUID 与 header id 一致**，pi 兼容——仓库已有非标准命名先例）；不改写文件内容（不动 header/cwd 归属，统计口径不受影响）
- 显示名规范化：去除路径分隔符与非法文件名字符（`/` `\` `:` `*` `?` `"` `<` `>` `|` 及首尾空白），空名拒绝（400）
- **仅非活跃会话可重命名**：文件 mtime 距今 > 5 分钟视为非活跃；活跃会话（≤5min，pi 正在写入）→ 409「会话活跃中，稍后再试」
- **重名冲突**：目标目录内同名文件已存在 → 409「同名文件已存在」，不做自动加序号
- 显示名派生：`/api/sessions` 每行新增 `fileName` 字段（原始文件名；显示名 = 去尾 `_<UUID>.jsonl` 的前缀，无前缀则显示原始时间戳名）

## User Stories

1. 作为用户，我想用 `node src/cli.ts serve` 启动本地 Web 服务，以便在浏览器查看 token 统计。
2. 作为用户，我想在浏览器打开默认 URL（http://127.0.0.1:50080/）即看到统计面板，以便无需记忆命令参数。
3. 作为用户，我想在总览看到汇总指标卡片（请求数/输入/输出/缓存/推理/缓存率/总token/花费），以便一眼掌握总消耗。
4. 作为用户，我想按模型或按项目（cwd）切换分组表，以便对比各维度消耗。
5. 作为用户，我想查看会话级明细表，以便定位单个会话的花费。
6. 作为用户，我想查看单请求级明细表（默认时间倒序），以便排查异常高消耗请求。
7. 作为用户，我想用 今天/7天/30天/全部/自定义 预设筛选时间范围，以便聚焦特定时段。
8. 作为用户，我想用自定义 since/until 精确限定时间范围，以便统计任意区间。
9. 作为用户，我想按模型/项目进一步筛选统计，以便聚焦特定维度。
10. 作为用户，我想开启自动刷新（5s/30s/5min），以便实时观察正在运行的 pi 进程消耗。
11. 作为用户，我想在自动刷新时看到「已更新」提示，以便确认数据是新的。
12. 作为用户，我想导出当前筛选范围的 JSON/CSV，以便接入自己的报表。
13. 作为用户，我想在会话管理 tab 按项目分组浏览全部会话，以便按项目组织管理。
14. 作为用户，我想给会话重命名（改为可读的显示名），以便日后识别会话内容。
15. 作为用户，我想重命名时得到明确的错误提示（非法名/活跃中/重名冲突），以便知道为何失败。
16. 作为用户，我希望重命名不改变统计数字（sessionId/cwd 不变），以便统计口径稳定。
17. 作为用户，我希望端口被占用时得到友好提示，以便用 --port 更换后重试。
18. 作为用户，我希望前端在数据加载/请求失败时显示友好错误（非白屏），以便知道服务状态。
19. 作为用户，我希望页面为深色主题，以便夜间使用不刺眼。
20. 作为用户，我希望服务仅监听 127.0.0.1，以便本地数据不被局域网访问。

## Implementation Decisions

- **模块组织**：新增 HTTP 服务器模块（serve 子命令入口、路由分发、静态资源服务）+ API 处理模块（端点 → 复用 analyze 层函数 → 复用 serialize 字段序列化）+ 单 HTML 内联前端资源（模板字符串或独立文件由服务端读取，实现自定）；`cli.ts` 增加 serve 子命令解析与路由，校验 serve 模式不接受窗口参数
- **serve 参数**：`--port`（默认 50080）、`--host`（默认 127.0.0.1）、`--dir`（继承默认）；不引入 `--interval`
- **路由分发**：同一 `http` 服务器内——`/` 与 `/index.html` 返回单 HTML；`/api/*` 走端点处理；其余 404；MIME：HTML `text/html; charset=utf-8`、JSON `application/json`
- **API 实现**：每次请求全量 `readSessionFiles(dir)` → `filterFiles`（model/cwd/since/until）→ 按端点派生（totalsFromFiles / sessionRowsFromFiles / requestRowsFromFiles / groupRowsFromFiles / periodRowsFromFiles）；响应字段复用 `serialize.ts` 的 `*ToObject` 转换；`/api/sessions` 行扩展 `fileName` 字段
- **重命名实现**：`POST /api/sessions/rename`——扫描数据目录定位 header id == sessionId 的文件 → 校验显示名合法 → 校验文件 mtime 非活跃（>5min）→ 校验目标 `/<显示名>_<UUID>.jsonl` 不存在 → `fs.rename`；返回新 fileName；失败按统一错误体
- **前端实现**：单 HTML 内联 CSS/JS，fetch 调用 API；tab 切换、筛选按钮、分页/排序、自动刷新定时器、重命名交互均为原生 JS；无框架无构建
- **数值格式化**：≥10⁴ 用 k/M 缩写（与参考页一致），花费保留原精度展示
- **零花费标注**：totals/分组/明细中 cost 全 0 显示「费率未配置（免费/未定价）」（沿用 CLI 展示口径）
- **统计口径**：完全沿用 token-analyzer 口径 A，webui 不改 analyze 层任何口径

## Testing Decisions

- **测试目标**：只测**外部行为**——「给定会话 JSONL fixture + 启动 serve → HTTP 响应（HTML/JSON/状态码）与重命名副作用是否符合 spec」，不测内部实现细节
- **测试 seam**：沿用既有「JSONL fixture → 输出断言」模式（`makeFixture`/`removeFixture` 建临时数据目录）；webui 的断言点是 **HTTP 层**——启动 serve（随机端口）后 fetch 端点断言 JSON 响应与状态码；重命名断言文件系统副作用（文件名变化、UUID 保留、冲突/活跃拒绝）
- **覆盖清单**（每个决策一个用例）：
  1. serve 启动：`/` 返回 200 text/html，含标题；未知路径 404 统一错误体
  2. `/api/totals` 返回 Totals 字段与 CLI totals 一致（同 fixture）
  3. `/api/sessions` 每行含 fileName；`/api/requests` 字段结构
  4. `/api/groups?by=model|cwd` 分组行与 CLI `--by` 一致
  5. `/api/period?period=day|week|month` 周期行与 CLI `--period` 一致
  6. `/api/meta` 返回 dir/sessionCount/dataRange
  7. 筛选参数：since/until/model/cwd 过滤结果与 CLI 一致；非法 since → 400
  8. 错误处理：无合法会话目录 → 500；未知 API 路径 → 404 统一错误体
  9. 重命名成功：`<时间戳>_<UUID>.jsonl` → `<显示名>_<UUID>.jsonl`，UUID 保留，header 未改
  10. 重命名拒绝：非法显示名 → 400；会话不存在 → 404；mtime ≤ 5min（活跃）→ 409；目标重名 → 409
  11. 重命名后统计不变：rename 前后 `/api/totals` 数字一致
  12. 显示名派生：无前缀文件名（标准时间戳名）显示原始时间戳名
- **前端交互**（tab/筛选/分页/自动刷新）以手工验收为主（无构建链、原生单 HTML），spec 不强制自动化前端测试；如实现方愿引入则可用浏览器级测试，但不作为必须

## Out of Scope

- **实现代码**——本 effort 只产出 spec，实现由后续 effort 承担
- **非统计功能**（供应商/代理/包管理/备份/设置/诊断）——只参考 pi-switch 统计页形态
- **前端框架与构建链**（React/Vite/webpack）——零依赖原生方案已定
- **鉴权/认证**——默认仅 127.0.0.1 本机访问，不做登录
- **token 优化建议**——只做计量，不做优化
- **统计口径改动**——webui 只做展示层，不改 analyze 层任何口径
- **会话删除/归档/移动目录**——本 effort 只做重命名；删除等破坏性操作不在范围
- **watch.ts 增量复用**——自动刷新走全量重算，不接增量状态

## Further Notes

- **与 CLI --watch 的差异**：CLI `--watch` 不支持 `--model/--cwd/--since/--until`（增量边界语义不清）；webui 自动刷新因全量重算而**支持筛选**，实现时勿误对齐 CLI 行为
- **重命名与 pi 的兼容性**：仓库已有非标准命名文件先例（`ni_<uuid>.jsonl`、`<中文名>_<uuid>.jsonl`），判定为合法会话；重命名仅改文件名前缀、保留尾 UUID，pi 会话识别（header id）不受影响
- **活跃会话阈值**：5 分钟为 spec 固定常量（实现可提为可配置，但默认必须 5min）
- **数据量参考**（2026-08 采样）：216 文件、212 合法会话、9022 条 usage 消息；全量重算秒级完成，5s 最低轮询档下开销可接受
- **术语**：见仓库 `CONTEXT.md`；本 spec 引用的「ticket 0N」指 `.scratch/token-analyzer-webui/issues/` 下同名文件
