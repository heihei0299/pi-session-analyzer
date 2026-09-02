# Token Analyzer

分析 pi 会话数据（`~/.pi/agent/sessions/` 下的 JSONL 文件）token 消耗的 CLI 工具。读取全部合法会话，按统计口径 A 提取消耗数据（含 **fork 会话去重**——fork 复制的历史消息不重复计费），输出总消耗量 / 会话级 / 单请求级三个窗口的指标，支持模型 / cwd 维度拆分、时间维度汇总与筛选、结构化输出（JSON/CSV），并可实时监控正在运行的 pi 进程（`--watch` 同样按 fork 去重口径）；`serve` 子命令启动零依赖本地 Web 面板（总览卡片 / 分组表 / 会话与请求明细 / 会话管理），支持时间范围筛选、**服务端分页排序**、自动刷新、导出 JSON/CSV 与会话重命名。
已发布为 npm 包 **`token-analyzer`**（npmjs.org，日期式版本如 `2026.8.6`）。
功能规格见 [`.scratch/token-analyzer/spec.md`](.scratch/token-analyzer/spec.md)（含实施状态）；实现拆分为 5 个 issue（[`.scratch/token-analyzer-impl/issues/`](.scratch/token-analyzer-impl/issues/)）。WebUI 功能规格见 [`.scratch/token-analyzer-webui/spec.md`](.scratch/token-analyzer-webui/spec.md)，实现拆分为 6 个 issue（[`.scratch/token-analyzer-webui-impl/issues/`](.scratch/token-analyzer-webui-impl/issues/)）。WebUI 审查问题修复见 [`.scratch/token-analyzer-webui-fixes/spec.md`](.scratch/token-analyzer-webui-fixes/spec.md)（8 个 issue）。
领域术语见 [`CONTEXT.md`](CONTEXT.md)，关键决策见 [`docs/adr/`](docs/adr/)（当前：`0001-fork-session-dedup.md`、`0002-total-tokens-gateway-alignment.md`）。
## 安装

### 方式 A：Go 原生二进制（推荐，毫秒级响应、零外部依赖）

```bash
# 1. 直接编译并安装到 $GOPATH/bin
go install ./cmd/token-analyzer

# 2. 或使用 Makefile 构建当前平台单二进制 (产物 dist/token-analyzer-go，约 7.5MB)
make build

# 3. 多平台交叉编译发布 (生成 Linux / macOS / Windows 产物)
make release
```

### 方式 B：npm 安装（Node ≥ 18）

```bash
npm i -g token-analyzer
```

开发环境（本仓库）：支持 Go 1.22+ 或 TypeScript + Node 24 双开发环境，均为零运行时依赖。

```bash
# Go 环境验证
go test -v ./...

# Node.js 环境验证
npm install      # 安装 typescript + @types/node（devDependencies）
npm test         # 运行测试
npm run build    # tsc 编译
```

## 用法

```
token-analyzer [totals|sessions|requests] --dir <path> [选项]
token-analyzer opencode sync [--auth <a>] [--workspace <w>] [--data-dir <d>]
token-analyzer opencode export [--format json|csv] [--output <path>] [--data-dir <d>]
```

开发时可用 `node src/cli.ts` 或 `./dist/token-analyzer-go` 直接运行。

- **窗口**（位置参数，默认 `totals`）：`totals` 总消耗量 / `sessions` 会话级（每会话一行）/ `requests` 单请求级（逐 assistant 消息）
- **数据目录**：`--dir <path>`（默认 `~/.pi/agent/sessions/`）
- **输出格式**：`--format table|json|csv`（默认 `table` 终端表格）
- **筛选**（对所有窗口生效，可组合）：`--model <id>` / `--cwd <path>` / `--since <时间>` / `--until <时间>`
- **分组**（仅 totals 窗口）：`--by model|cwd|model,cwd` 按维度汇总
- **时间汇总**（仅 totals 窗口）：`--period day|week|month` 按周期汇总
- **实时监控**：`--watch [--interval <ms>]` 长驻跟随（默认 1s 轮询）
- **Web 面板**：`serve [--port <n>] [--host <h>] [--dir <path>]` 启动零依赖 HTTP 服务（默认 `127.0.0.1:50080`，仅本机；serve 模式仅支持这三个参数）
- **帮助/版本**：`-h/--help` 显示用法，`-v/--version` 显示版本；未知参数/命令显式报错（可用 `-h` 查看）

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
npm version 2026.8.28 && git push && git push --tags
```

- **版本**：日期式 semver（`YYYY.M.D`）；同日再次发布用 prerelease 后缀（`2026.8.6-1`）
- **校验**：tag 与 `package.json.version` 不一致时 workflow 失败（防手滑）
- **凭据**：`NPM_TOKEN`（npmjs Automation token）存于 GitHub Actions secret

## Web 面板（serve）

`token-analyzer serve` 启动本地 Web 服务（零依赖，Node 原生 `http` + 单 HTML 内联前端），浏览器访问 `http://127.0.0.1:50080/`：

- **四个 tab**：总览（8 张汇总卡片 + 按模型/cwd 分组表 + Token tape 构成条）/ 会话明细 / 请求明细 / 会话管理（按项目 cwd 分组 + 重命名会话）
- **视觉**：仪器台（bakelite 台面 `#0F1312` + enamel 纸面 `#FFFEF8` + 黄铜 `#C5A254` + 仪表青 `#0FA08C` + 报警朱 `#E2452E`，鼓轮读数 + 黄铜铆钉 + 链孔纸带），`Fraunces`（标题/数值）/ `IBM Plex Sans`（正文）/ `JetBrains Mono`（数据）
- **时间范围**：今天（默认） / 7天 / 30天 / 全部 / 自定义（date 日期 + 时分下拉 00:00-23:59，按本地时间解释，打开时自动预填当前筛选或数据范围），作用于总览与明细与导出；默认窗口 = 今天（本地今天 00:00-23:59:59.999）；头部「范围」胶囊随筛选即时显示，下方状态行范围提示已移除
- **明细服务端分页排序**：会话/请求明细每页 20/50/100 行，点击列头排序——翻页/排序/改页大小重新 fetch（page/size/sortKey/sortDir），不再全量拉取（真实数据 /api/requests 26.7MB → 每页 ~20KB）；会话明细显示筛选合计（总 tokens/请求/会话数，含子代理）且子代理会话带“子代理”徽标
- **统计口径（webui）**：时间筛选按**消息 timestamp 消息级**归属（跨天会话的凌晨请求计入当天，与明细一致）；「输入」列显示**总输入**（非缓存 input + 缓存命中 cacheRead，与 pi-switch 网关 Input 对齐）；CLI 与导出保持原始字段
- **自动刷新**：Off / 5s / 30s / 5min（后端每请求全量重算），数据变化时状态行显示「已更新 HH:MM:SS」
- **导出**：JSON（`{ totals, sessions, requests }`）与 CSV（`# totals` / `# sessions` / `# requests` 三段式）下载当前筛选范围
- **会话管理**：顶部最近会话 10 条 + 按规范化 cwd 分组展示全部会话（默认收起，组可折叠，点击标题展开），点击名称行内编辑重命名——改文件名前缀保留尾 UUID（`<显示名>_<UUID>.jsonl`），仅非活跃会话（mtime > 5min）可改，非法名 400 / 不存在 404 / 活跃与重名 409
- **会话详情抽屉**：点击会话/请求明细的会话名称或会话 ID 滑出右侧抽屉（760px，移动端全屏，遮罩/×/Esc 关闭），展示头部（可点击标题行内重命名，同校验 400/409、成功后重刷抽屉与列表）、Token 构成条、汇总卡（总 token/请求数/花费/缓存率，合并时小字“其中主 X · 子代理 Y（N 个）”）与请求时间线（按时间升序、全量无分页、子代理行淡底 #FFF6D6 +“子代理 短 ID 8”徽标）；支持“合并子代理”开关（默认开、每次打开重置，无子代理隐藏）与时间线“暂无计入口径请求”空态；请求表容器 max-height:60vh 可滚动（极端 >200 行）；打开期间暂停自动刷新轮询

HTTP API（`/api/*`，裸 JSON，与 CLI 结构化输出同字段）：`totals` / `sessions` / `requests` / `groups?by=` / `period?period=` / `meta`（筛选参数 `model`/`cwd`/`since`/`until`；明细端点另支持 `page`/`size`/`sortKey`/`sortDir`，响应含 `total`，`sessions` 另含 `totals` 聚合计与行 `isTask` 子代理标记）+ `POST /api/sessions/rename` + `GET /api/sessions/:id/detail`（或 `GET /api/sessions/detail?sessionId=`，返回 `{ session, children, totals{main,merged,childrenCount}, requests{source,sourceSessionId}, meta{hasChildren} }`，视图合并子代理消耗仅详情视图、列表保持独立，404 会话不存在）；错误统一 `{ error, detail }`（400/404/409/500）。

## 统计口径（口径 A）

- **计入口径**：仅 `type=message && role=assistant` 且携带 usage 的消息；toolResult / compaction / branch_summary / user 一律忽略
- **fork 会话去重**：header 含 `parentSession` 的 fork 会话，其复制历史（`message.timestamp < header.timestamp`，fork 创建时间）的 usage 已在父会话统计过，analyzeFile 剔除；fork 后新增消息保留（CLI/webui/--watch 一致，watch 首读解析 forkTs 剔除、替换/重读复用；与 pi-switch 网关统计对齐，8/1 起累计差异 ≈0.6%——8/2、8/4 分毫不差，剩余为覆盖结构，见下「与网关对比」）
- **请求数**：带 usage 的 assistant 消息数；全 0 usage 的失败/中止消息也计入请求数（token 为 0）
- **总 token**：总输入 + 输出 = `input + cacheRead + output`（对齐 pi-switch 网关 total；不含 cacheWrite，ADR-0002 已 Accepted，实现见 `.scratch/token-analyzer-gateway-alignment/`）
- **缓存率**：`cacheRead / (input + cacheRead)`（分母不含 cacheWrite，ADR-0002）；分母为 0 记 0；聚合先求和分子分母再除
- **花费**：直接累加 `usage.cost.total`；全 0 花费标注「费率未配置（免费/未定价）」
- **模型归属**：请求级（每条消息的 `model` 字段）
- **cwd 归属**：会话 header `cwd` 为权威键，规范化（绝对路径、去尾斜杠、符号链接解析）；目录名有损编码不参与归属
- **时间归属**：CLI `--since`/`--until` 按会话 header timestamp 闭区间（含端点）；webui 全端点（`totals`/`sessions`/`requests`/`groups`/`period`）按消息 timestamp 消息级（跨天会话凌晨请求计入当天，总览与会话明细求和一致；与 CLI 在跨天场景有预期差异）
- **网关可比窗口**：pi-switch 网关数据起点 = `2026-08-01T00:00:00Z`（UTC）；历史 webui「自 8/1」预设已移除（2026-09-01 清理），如需对账可通过自定义输入该起点，切「全部」查看含 8/1 前数据的完整历史
- **时区语义**：webui/CLI 时间参数按本地时区解释（CST 自然日）；网关日志 `ts` 为 UTC——对账时以本地时区解释网关 ts，「今天」边界差 8 小时属预期
- **与网关对比**（2026-08-06 对账）：8/1 起累计 session 940.5M vs 网关 935.0M（差 0.6%）；8/2、8/4 分毫不差；差异全部为覆盖结构——8/1 网关刚启用（仅 4 条记录，+37.2M）、8/3/8/5 网关多出其他客户端请求、pi 直连请求只在 session 目录；CLI 全量窗口含 8/1 前数据（≈494M）与网关不可比
- **与网关对比（2026-08-07 对账）**：逐条匹配（时间戳+token 数）确认两侧定价**完全一致**（679 条 0 差异），「今日」金额差异（约 $0.2/天）全部为覆盖结构：① pi 压缩/摘要等**内部请求**（无会话名、真实计费，约 $0.10/天）不入 session 对话流（compaction entry 无 usage 字段）→ 结构性漏算；② **其他客户端请求**（opencode dreamer 后台任务，约 $0.09/天）只在网关。用户决策：结构性接受、不引入网关数据源；webui 已加口径说明（见 `.scratch/webui-gateway-disclaimer/`）

## 结构

```
CONTEXT.md              领域术语表（统计口径、fork 会话、字段语义）
docs/adr/               架构决策记录（0001-fork-session-dedup、0002-total-tokens-gateway-alignment）
Makefile                多平台交叉编译与自动化测试脚本
go.mod                  Go 模块配置（零外部第三方依赖，保持纯标准库）
cmd/token-analyzer/     Go CLI 与 Serve 统一主程序入口
internal/
  domain/               核心聚合模型与指标定义（ADR-0002 口径、Totals）
  sessiondata/          SessionData 会话数据仓核心深模块（ADR-0001 fork 去重、快照缓存、singleflight）
  timerange/            CST 本地时区双语义时间引擎（严格公历校验）
  watch/                增量实时监控引擎（跨平台 Inode 隔离、负补偿重读）
  server/               原生 net/http 服务（内嵌 webui.html、会话重命名防护）
  opencode/             OpenCode RPC 逆向客户端、POSIX/Windows 文件锁、云端对账
  render/               终端 ASCII 格式化表格渲染
  serialize/            JSON / CSV 序列化器
src/                    TypeScript 原型与历史 Node.js 实现
  webui.html            单 HTML 内联前端（仪器台 dark bench + Canvas 2D 绘图，4 tab、服务端分页、导出）
test/                   集成与双轨金样对账测试（Node vs Go 5大窗口 0 误差验证）
dist/                   构建产物（包含多平台交叉编译二进制）
.scratch/               功能规格与 issue（token-analyzer / webui / opencode-sync 等）
.github/workflows/      自动化 CI / 发布流程
```
