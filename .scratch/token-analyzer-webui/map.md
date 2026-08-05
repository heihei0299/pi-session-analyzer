# Token Analyzer WebUI — Map

**Map id**: `token-analyzer-webui` — see `docs/agents/issue-tracker.md` for tracker conventions.

## Destination

在现有 token-analyzer CLI 之上设计一个 **webui** 的可交接 spec（本 effort 只产出 spec，不写实现代码——延续 token-analyzer 模式）：`node src/cli.ts serve` 启动本地 HTTP 服务，浏览器访问展示统计面板——汇总卡片（totals）+ 按模型/cwd 分组表 + 会话/请求明细表 + 时间范围筛选 + 自动刷新 + 导出 JSON/CSV，**UI 形态参考 pi-switch 统计页**（`http://127.0.0.1:43110/` 的 📊 统计面板：汇总卡片行 + 分组表 + 明细表 + 时间范围按钮 + 自动刷新下拉 + 导出按钮）。技术栈延续项目现状：**零运行时依赖**（Node 24 type-stripping 直跑 `.ts`，原生 `http` 模块 + 原生 HTML/CSS/JS，无构建步骤）。复用现有 `src/analyze.ts` / `aggregate.ts` / `serialize.ts` / `watch.ts` 数据层，不重写统计逻辑。

## Notes

- **Domain**: pi 会话数据 token 统计（延续 token-analyzer effort 的统计口径 A，见 `.scratch/token-analyzer/spec.md`；数据源 `~/.pi/agent/sessions/`）
- **技能**: `grilling` / `domain-modeling`（HITL 决策，ticket 类型 grilling）、`prototype`（UI 形态，ticket 类型 prototype）
- **用户偏好**: 中文沟通；高确定性、流程驱动；**只规划**（本 effort 遵循 wayfinder 默认「plan, don't do」——tickets 只解析决策，产出 spec，不写实现代码）；零运行时依赖；UI 参考 pi-switch 统计页
- **会话纪律**: 每 session 最多 resolve 一个 ticket（research 除外）
- **前置资产**: 现有 CLI 实现全部可复用——`analyze.ts`（读取/三窗口/分组/时间汇总/筛选）、`aggregate.ts`（Totals 聚合）、`serialize.ts`（JSON/CSV）、`watch.ts`（IncrementalReader 增量读取器）、`cli.ts`（参数解析与窗口路由）；测试 seam 沿用「JSONL fixture → 输出断言」
- **参考实现**: pi-switch webui 源码在 `/home/shial/Project/pi-switch/webui/`（React/Vite 构建，**技术栈不跟随**，仅参考布局与交互形态；统计面板 `src/components/StatsPanel.tsx`）
- **偏差记录**: 同 token-analyzer——本仓库无 remote，research 发现直接写入 ticket 文件的 `## Answer` 段落

## Decisions so far

<!-- 每解析一个 ticket，在此追加一行：名称 + 链接 + 一句话结论 -->
- [serve 命令与零依赖 HTTP 服务器骨架](issues/01-serve-command-and-server.md) — `serve` 子命令（`node src/cli.ts serve [--port] [--host] [--dir]`），默认端口 50080；单 HTML 内联静态资源；--host 默认 127.0.0.1、--dir 继承 CLI 默认、不引入 --interval（归 04）；EADDRINUSE 友好提示 + 打印访问 URL + Ctrl+C 优雅退出
- [HTTP API 设计](issues/02-http-api-design.md) — 多端点细粒度（/api/totals、/api/sessions、/api/requests、/api/groups?by=、/api/period?period=、/api/meta）；裸 JSON 响应（复用 serialize 字段）+ meta 端点；筛选参数直接映射 CLI（model/cwd/since/until）；统一 JSON 错误体 { error, detail } + 400/404/500
- [前端 UI 布局（参考 pi-switch 统计页）](issues/03-ui-layout-pi-switch.md) — tab 三视图（总览/会话明细/请求明细）；8 张汇总指标卡片（单组）；分组表按模型/cwd 切换；明细表分 tab + 分页(20/50/100) + 列排序；时间范围预设按钮组（今天/7天/30天/全部/自定义）；导出 JSON/CSV 由前端 fetch 组装下载；深色主题零依赖
- [自动刷新机制（对应 CLI --watch）](issues/04-realtime-refresh.md) — 全量重算（不复用 IncrementalReader）；轮询 Off/5s/30s/5min 默认 Off；筛选照常生效（与 CLI --watch 不支持筛选形成对照）；静默替换 + 状态行「已更新」提示
- [会话管理 tab（按项目组织 + 重命名会话）](issues/06-session-management.md) — 新增第 4 个 tab 按 cwd 分组管理会话；重命名 = 改文件名保留尾 UUID（pi 兼容）；仅非活跃会话（mtime>5min）；冲突 409；/api/sessions 增 fileName 字段 + POST /api/sessions/rename
- [综合 spec](issues/05-compose-spec.md) — 已交付 `.scratch/token-analyzer-webui/spec.md`（ready-for-agent）：全部 6 个 ticket 决策综合为可交接 spec，含 20 条 user stories、HTTP 层测试 seam（JSONL fixture → serve 端点断言 + 重命名副作用）、12 个覆盖用例
## Not yet specified

<!-- 全部 fog 已 ticket 化并 resolve；map 已到终点：6 个 ticket 全部 resolved（含综合 spec 05），spec 可交接实现 -->

## Out of scope

- **非统计功能**（供应商/代理/包管理/备份/设置/诊断）——只参考 pi-switch 的统计页形态，不做其他面板
- **前端框架与构建链**（React/Vite/webpack）——用户已定零依赖原生方案
- **鉴权/认证**——本地 127.0.0.1 工具，默认仅本机访问，不做登录
- **token 优化建议**——延续口径：只做计量，不做优化
- **统计口径改动**——webui 只做展示层，不改 analyze 层任何口径
- **实现代码**——本 effort 只产出 spec，实现由后续 effort 承担
