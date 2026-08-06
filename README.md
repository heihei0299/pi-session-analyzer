# Token Analyzer

分析 pi 会话数据（`~/.pi/agent/sessions/` 下的 JSONL 文件）token 消耗的 CLI 工具。读取全部合法会话，按统计口径 A 提取消耗数据（含 **fork 会话去重**——fork 复制的历史消息不重复计费），输出总消耗量 / 会话级 / 单请求级三个窗口的指标，支持模型 / cwd 维度拆分、时间维度汇总与筛选、结构化输出（JSON/CSV），并可实时监控正在运行的 pi 进程（`--watch` 同样按 fork 去重口径）；`serve` 子命令启动零依赖本地 Web 面板（总览卡片 / 分组表 / 会话与请求明细 / 会话管理），支持时间范围筛选、**服务端分页排序**、自动刷新、导出 JSON/CSV 与会话重命名。
已发布为 npm 包 **`token-analyzer`**（npmjs.org，日期式版本如 `2026.8.6`）。
功能规格见 [`.scratch/token-analyzer/spec.md`](.scratch/token-analyzer/spec.md)（含实施状态）；实现拆分为 5 个 issue（[`.scratch/token-analyzer-impl/issues/`](.scratch/token-analyzer-impl/issues/)）。WebUI 功能规格见 [`.scratch/token-analyzer-webui/spec.md`](.scratch/token-analyzer-webui/spec.md)，实现拆分为 6 个 issue（[`.scratch/token-analyzer-webui-impl/issues/`](.scratch/token-analyzer-webui-impl/issues/)）。WebUI 审查问题修复见 [`.scratch/token-analyzer-webui-fixes/spec.md`](.scratch/token-analyzer-webui-fixes/spec.md)（8 个 issue）。
领域术语见 [`CONTEXT.md`](CONTEXT.md)，关键决策见 [`docs/adr/`](docs/adr/)（当前：`0001-fork-session-dedup.md`、`0002-total-tokens-gateway-alignment.md`）。
## 安装

```bash
npm i -g token-analyzer    # 从 npm 安装（Node ≥ 18）
```

开发环境（本仓库）：TypeScript + Node 24，零运行时依赖。

```bash
npm install      # 安装 typescript + @types/node（devDependencies）
npm test         # 运行全部测试（114 用例；固定 TZ=Asia/Shanghai 保证时区确定性）
npm run build    # tsc 编译到 dist/ + 复制 webui.html（发布产物）
```

## 用法

```
token-analyzer [totals|sessions|requests] --dir <path> [选项]
```

开发时可用 `node src/cli.ts` 替代 `token-analyzer`（Node 24 type-stripping 直接运行）。

- **窗口**（位置参数，默认 `totals`）：`totals` 总消耗量 / `sessions` 会话级（每会话一行）/ `requests` 单请求级（逐 assistant 消息）
- **数据目录**：`--dir <path>`（默认 `~/.pi/agent/sessions/`）
- **输出格式**：`--format table|json|csv`（默认 `table` 终端表格）
- **筛选**（对所有窗口生效，可组合）：`--model <id>` / `--cwd <path>` / `--since <时间>` / `--until <时间>`
- **分组**（仅 totals 窗口）：`--by model|cwd|model,cwd` 按维度汇总
- **时间汇总**（仅 totals 窗口）：`--period day|week|month` 按周期汇总
- **实时监控**：`--watch [--interval <ms>]` 长驻跟随（默认 1s 轮询）
- **Web 面板**：`serve [--port <n>] [--host <h>] [--dir <path>]` 启动零依赖 HTTP 服务（默认 `127.0.0.1:50080`，仅本机；serve 模式仅支持这三个参数）

### 示例

```bash
# 总消耗量（终端表格）
token-analyzer

# 按模型分组
token-analyzer totals --by model

# 按 cwd 分组（交叉）
token-analyzer totals --by model,cwd --cwd /home/shial/Project/pi-session-anylize

# 会话级窗口 + 模型过滤
token-analyzer sessions --model deepseek-v4-flash

# 按月汇总 + 时间范围
token-analyzer totals --period month --since 2026-07-01 --until 2026-08-31

# JSON 输出（供脚本消费）
token-analyzer totals --by model --format json

# 实时监控
token-analyzer totals --watch --interval 1000

# 启动 Web 面板（浏览器访问 http://127.0.0.1:50080/）
token-analyzer serve
```

## 发布

push `v<版本>` tag 由 GitHub Actions（[`.github/workflows/publish.yml`](.github/workflows/publish.yml)）自动完成 typecheck + test + build + `npm publish`：

```bash
npm version 2026.8.7 && git push && git push --tags
```

- **版本**：日期式 semver（`YYYY.M.D`）；同日再次发布用 prerelease 后缀（`2026.8.6-1`）
- **校验**：tag 与 `package.json.version` 不一致时 workflow 失败（防手滑）
- **凭据**：`NPM_TOKEN`（npmjs Automation token）存于 GitHub Actions secret

## Web 面板（serve）

`token-analyzer serve` 启动本地 Web 服务（零依赖，Node 原生 `http` + 单 HTML 内联前端），浏览器访问 `http://127.0.0.1:50080/`：

- **四个 tab**：总览（8 张汇总卡片 + 按模型/cwd 分组表）/ 会话明细 / 请求明细 / 会话管理（按项目 cwd 分组 + 重命名会话）
- **时间范围**：今天 / 7天 / 30天 / 自 8/1（网关可比，默认） / 全部 / 自定义（date 日期 + 时分下拉，按本地时间解释），作用于总览与明细与导出；默认窗口 = 网关可比（since=`2026-08-01T00:00:00Z` UTC 精确，状态行标注「（网关可比）」）；状态行「范围」随筛选即时显示（未筛选时显示数据范围 min/max）
- **明细服务端分页排序**：会话/请求明细每页 20/50/100 行，点击列头排序——翻页/排序/改页大小重新 fetch（page/size/sortKey/sortDir），不再全量拉取（真实数据 /api/requests 26.7MB → 每页 ~20KB）
- **统计口径（webui）**：时间筛选按**消息 timestamp 消息级**归属（跨天会话的凌晨请求计入当天，与明细一致）；「输入」列显示**总输入**（非缓存 input + 缓存命中 cacheRead，与 pi-switch 网关 Input 对齐）；CLI 与导出保持原始字段
- **自动刷新**：Off / 5s / 30s / 5min（后端每请求全量重算），数据变化时状态行显示「已更新 HH:MM:SS」
- **导出**：JSON（`{ totals, sessions, requests }`）与 CSV（`# totals` / `# sessions` / `# requests` 三段式）下载当前筛选范围
- **会话管理**：按规范化 cwd 分组展示全部会话（组可折叠），点击名称行内编辑重命名——改文件名前缀保留尾 UUID（`<显示名>_<UUID>.jsonl`），仅非活跃会话（mtime > 5min）可改，非法名 400 / 不存在 404 / 活跃与重名 409

HTTP API（`/api/*`，裸 JSON，与 CLI 结构化输出同字段）：`totals` / `sessions` / `requests` / `groups?by=` / `period?period=` / `meta`（筛选参数 `model`/`cwd`/`since`/`until`；明细端点另支持 `page`/`size`/`sortKey`/`sortDir`，响应含 `total`）+ `POST /api/sessions/rename`；错误统一 `{ error, detail }`（400/404/409/500）。

## 统计口径（口径 A）

- **计入口径**：仅 `type=message && role=assistant` 且携带 usage 的消息；toolResult / compaction / branch_summary / user 一律忽略
- **fork 会话去重**：header 含 `parentSession` 的 fork 会话，其复制历史（`message.timestamp < header.timestamp`，fork 创建时间）的 usage 已在父会话统计过，analyzeFile 剔除；fork 后新增消息保留（CLI/webui/--watch 一致，watch 首读解析 forkTs 剔除、替换/重读复用；与 pi-switch 网关统计对齐，8/1 起累计差异 ≈0.6%——8/2、8/4 分毫不差，剩余为覆盖结构，见下「与网关对比」）
- **请求数**：带 usage 的 assistant 消息数；全 0 usage 的失败/中止消息也计入请求数（token 为 0）
- **总 token**：总输入 + 输出 = `input + cacheRead + output`（对齐 pi-switch 网关 total；不含 cacheWrite，ADR-0002 已 Accepted，实现见 `.scratch/token-analyzer-gateway-alignment/`）
- **缓存率**：`cacheRead / (input + cacheRead)`（分母不含 cacheWrite，ADR-0002）；分母为 0 记 0；聚合先求和分子分母再除
- **花费**：直接累加 `usage.cost.total`；全 0 花费标注「费率未配置（免费/未定价）」
- **模型归属**：请求级（每条消息的 `model` 字段）
- **cwd 归属**：会话 header `cwd` 为权威键，规范化（绝对路径、去尾斜杠、符号链接解析）；目录名有损编码不参与归属
- **时间归属**：CLI `--since`/`--until` 按会话 header timestamp 闭区间（含端点）；webui 统计端点按消息 timestamp（跨天差异见 spec/ADR）
- **网关可比窗口**：pi-switch 网关数据起点 = `2026-08-01T00:00:00Z`（UTC）；webui「自 8/1」预设即此起点（默认窗口），切「全部」查看含 8/1 前数据的完整历史
- **时区语义**：webui/CLI 时间参数按本地时区解释（CST 自然日）；网关日志 `ts` 为 UTC——对账时以本地时区解释网关 ts，「今天」边界差 8 小时属预期
- **与网关对比**（2026-08-06 对账）：8/1 起累计 session 940.5M vs 网关 935.0M（差 0.6%）；8/2、8/4 分毫不差；差异全部为覆盖结构——8/1 网关刚启用（仅 4 条记录，+37.2M）、8/3/8/5 网关多出其他客户端请求、pi 直连请求只在 session 目录；CLI 全量窗口含 8/1 前数据（≈494M）与网关不可比

## 结构

```
CONTEXT.md      领域术语表（统计口径、fork 会话、字段语义）
docs/adr/       架构决策记录（0001-fork-session-dedup、0002-total-tokens-gateway-alignment）
src/
  analyze.ts    数据读取层（目录遍历、合法会话判定、fork 去重、三窗口派生、过滤/分组/时间汇总）
  aggregate.ts  聚合模型（Totals 类型、指标计算）
  render.ts     终端表格渲染
  serialize.ts  JSON / CSV 序列化
  cli.ts        CLI 入口（参数解析、窗口路由、watch/serve 集成）
  watch.ts      实时监控增量读取器
  server.ts     serve HTTP 服务器（路由分发、生命周期、EADDRINUSE 友好提示）
  api.ts        HTTP API 层（端点处理、筛选、分页排序、统一错误体、会话重命名）
  webui.html    单 HTML 内联前端（深色主题、4 tab、fetch API、服务端分页、自动刷新、导出）
test/           node:test 测试（fixture JSONL → CLI 输出断言；serve → HTTP 端点断言）
dist/           构建产物（npm 发布内容；不入库）
.scratch/       功能规格与 issue（token-analyzer / token-analyzer-webui / webui-fixes / npm-publish）
.github/workflows/publish.yml  tag 触发自动发布
```
