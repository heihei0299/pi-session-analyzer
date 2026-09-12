# Token Analyzer

分析 Pi 会话与 Codex rollout token 消耗的 Go-only CLI 工具（单二进制，零外部依赖）。Pi 默认读取 `~/.pi/agent/sessions/`，Codex 读取 `sessions/` 与 `archived_sessions/` 下的 plain/zstd rollout。读取全部合法会话，按统计口径 A 提取消耗数据（含 **fork 会话去重**——fork 复制的历史消息不重复计费），输出总消耗量 / 会话级 / 单请求级三个窗口的指标，支持模型 / cwd 维度拆分、时间维度汇总与筛选、结构化输出（JSON/CSV），并可实时监控正在运行的 pi 进程（`--watch` 同样按 fork 去重口径）；`serve` 子命令启动零依赖本地 Web 面板（总览卡片 / 分组表 / 会话与请求明细 / 会话管理），支持时间范围筛选、**服务端分页排序**、基于 source revision 的自动刷新、导出 JSON/CSV 与会话重命名。
以 GitHub Release 发布 Go 单二进制（Linux / macOS / Windows，日期式版本如 `2026.9.3`），不再提供 npm 分发。
领域术语与最终架构见 [`CONTEXT.md`](CONTEXT.md)，关键决策见 [`docs/adr/`](docs/adr/)。历史迁移计划与审计记录仅作背景参考，不是当前执行入口。

## 安装

```bash
# 1. 直接编译并安装到 $GOPATH/bin
go install ./cmd/token-analyzer

# 2. 或使用 Makefile 构建当前平台单二进制 (产物 dist/token-analyzer)
make build

# 3. 多平台交叉编译发布 (生成 Linux / macOS / Windows 产物)
make release
```

开发环境只需要 Go 1.23+（纯 Go zstd decoder 读取 Codex compressed rollout，modernc.org/sqlite 零 CGO）。运行 CLI/API/WebUI 不需要 Node/npm。

### 数据源能力矩阵（与 `/api/meta.sources = ["pi","codex"]` 一致）

| 能力 | pi | codex | all（pi + codex） |
| --- | --- | --- | --- |
| totals / sessions / groups / period | 支持 | 支持 | 支持（同一 Query Engine 组合查询） |
| requests 窗口 | 支持 | 明确拒绝（unsupported） | 明确拒绝 |
| 会话详情、重命名 | 支持 | 明确拒绝且不写 Pi 目录 | 明确拒绝 |
| cost | 已知 Pi 成本 | `unpriced`（无美元花费） | 保留 Pi 成本并标注含 unpriced 源 |

```bash
# 环境验证（限制并发以节省资源）
GOMAXPROCS=2 go test -p 1 ./...
```

## 用法

```
token-analyzer [totals|sessions|requests] [--source pi|codex|all] [--dir <pi-dir>] [--codex-dir <codex-home>] [选项]
```

OpenCode 云端用量同步与对账已迁入同仓独立项目 [`opencode-analyzer/`](opencode-analyzer/README.md)。token-analyzer 不再读取 OpenCode 凭据，也不再提供 OpenCode CLI、API 或 WebUI。

开发时可用 `go run ./cmd/token-analyzer` 或 `./dist/token-analyzer` 直接运行。

- **窗口**（位置参数，默认 `totals`）：`totals` 总消耗量 / `sessions` 会话级（每会话一行）/ `requests` 单请求级（逐条计入口径消息）
- **数据源**：`--source pi|codex|all`（默认 `pi`）；Codex 目录优先级为 `--codex-dir > CODEX_HOME > ~/.codex`。`--dir` 始终只表示 Pi 目录。
- **数据库**：`--db <path>` 可指定 normalized ledger；未指定时使用 `TOKEN_ANALYZER_DB`/默认缓存路径。
- **数据目录**：`--dir <path>`（默认 `~/.pi/agent/sessions/`）
- **输出格式**：`--format table|json|csv`（默认 `table` 终端表格）
- **筛选**（对所有窗口生效，可组合）：`--model <id>` / `--cwd <path>` / `--since <时间>` / `--until <时间>`
- **分组**（仅 totals 窗口）：`--by model|cwd|model,cwd` 按维度汇总
- **时间汇总**（仅 totals 窗口）：`--period day|week|month` 按周期汇总
- **实时监控**：`--watch [--interval <ms>]` 长驻跟随（默认 1s 轮询）
- **Web 面板**：`serve [--port <n>] [--host <h>] [--dir <path>] [--source pi|codex|all] [--codex-dir <path>]` 启动本地 HTTP 服务；面板只按后端 `/api/meta.sources` 声明渲染源选择器，Codex/All 下 requests、会话详情、重命名会禁用并给出原因。
- **帮助/版本**：`-h/--help` 显示用法，`-v/--version` 显示版本；未知参数/命令显式报错（可用 `-h` 查看）

### 示例

```bash
# 总消耗量（默认 Pi）
token-analyzer

# 查看 Codex totals/sessions
TOKEN_ANALYZER_DB=./data/codex.db token-analyzer totals --source codex --codex-dir ~/.codex
token-analyzer sessions --source codex --codex-dir ~/.codex --format json

# 按模型分组
token-analyzer totals --by model

# 按 cwd 分组（交叉）
token-analyzer totals --by model,cwd --cwd /home/shial/Project/token-analyzer

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

push `v<版本>` tag 由 GitHub Actions（[`.github/workflows/release.yml`](.github/workflows/release.yml)）自动完成 `go test -p 1 ./...` + `make release` + GitHub Release（Linux / macOS / Windows 二进制 + checksums）：

```bash
git tag v2026.9.12 && git push && git push --tags
```

- **版本**：日期式 semver（`YYYY.M.D`）；同日再次发布用 prerelease 后缀（`2026.9.3-1`）
- **产物**：只发布 Go-only token-analyzer 所需二进制，不再区分 Go/npm edition

## Web 面板（serve）

`token-analyzer serve` 启动本地 Web 服务（零依赖，Go 原生 `net/http` + 单 HTML 内联前端，唯一人工维护源为 `internal/server/webui.html`），浏览器访问 `http://127.0.0.1:50080/`：

- **四个 tab**：总览（8 张汇总卡片 + 按模型/cwd 分组表 + Token tape 构成条）/ 会话明细 / 请求明细 / 会话管理（按项目 cwd 分组 + 重命名会话）
- **视觉**：仪器台（bakelite 台面 `#0F1312` + enamel 纸面 `#FFFEF8` + 黄铜 `#C5A254` + 仪表青 `#0FA08C` + 报警朱 `#E2452E`，鼓轮读数 + 黄铜铆钉 + 链孔纸带），`Fraunces`（标题/数值）/ `IBM Plex Sans`（正文）/ `JetBrains Mono`（数据）
- **时间范围**：今天（默认） / 7天 / 30天 / 全部 / 自定义（date 日期 + 时分下拉 00:00-23:59，按本地时间解释，打开时自动预填当前筛选或数据范围），作用于总览与明细与导出；默认窗口 = 今天（本地今天 00:00-23:59:59.999）；头部「范围」胶囊随筛选即时显示，下方状态行范围提示已移除
- **明细服务端分页排序**：会话/请求明细每页 20/50/100 行，点击列头排序——翻页/排序/改页大小重新 fetch（page/size/sortKey/sortDir），不再全量拉取（真实数据 /api/requests 26.7MB → 每页 ~20KB）；会话明细显示筛选合计（总 tokens/请求/会话数，含子代理）且子代理会话带“子代理”徽标
- **统计口径（webui）**：时间筛选按**消息 timestamp 消息级**归属（跨天会话的凌晨请求计入当天，与明细一致）；「输入」列显示**总输入**（非缓存 input + 缓存命中 cacheRead，与 pi-switch 网关 Input 对齐）；CLI 与导出保持原始字段
- **自动刷新**：Off / 5s / 30s / 5min（后端按 source revision 变化刷新 snapshot，GET 本身只读），数据变化时状态行显示「已更新 HH:MM:SS」
- **导出**：JSON（`{ totals, sessions, requests }`）与 CSV（`# totals` / `# sessions` / `# requests` 三段式）下载当前筛选范围
- **会话管理**：顶部最近会话 10 条 + 按规范化 cwd 分组展示全部会话（默认收起，组可折叠，点击标题展开），点击名称行内编辑重命名——改文件名前缀保留尾 UUID（`<显示名>_<UUID>.jsonl`），仅非活跃会话（mtime > 5min）可改，非法名 400 / 不存在 404 / 活跃与重名 409
- **会话详情抽屉**：点击会话/请求明细的会话名称或会话 ID 滑出右侧抽屉（760px，移动端全屏，遮罩/×/Esc 关闭），展示头部（可点击标题行内重命名，同校验 400/409、成功后重刷抽屉与列表）、Token 构成条、汇总卡（总 token/请求数/花费/缓存率，合并时小字“其中主 X · 子代理 Y（N 个）”）与请求时间线（按时间升序、全量无分页、子代理行淡底 #FFF6D6 +“子代理 短 ID 8”徽标）；支持“合并子代理”开关（默认开、每次打开重置，无子代理隐藏）与时间线“暂无计入口径请求”空态；请求表容器 max-height:60vh 可滚动（极端 >200 行）；打开期间暂停自动刷新轮询

HTTP API（`/api/*`，裸 JSON，与 CLI 结构化输出同字段）：`totals` / `sessions` / `requests` / `groups?by=` / `period?period=` / `meta`（筛选参数 `source`/`model`/`cwd`/`since`/`until`；`source=pi|codex|all`，其他值返回 400 Unsupported；明细端点另支持 `page`/`size`/`sortKey`/`sortDir`，响应含 `total`，`sessions` 另含 `totals` 聚合计与行 `isTask` 子代理标记）+ `POST /api/sessions/rename` + `GET /api/sessions/:id/detail`（或 `GET /api/sessions/detail?sessionId=`，返回 `{ session, children, totals{main,merged,childrenCount}, requests{source,sourceSessionId}, meta{hasChildren} }`，视图合并子代理消耗仅详情视图、列表保持独立；Codex/All source 下 details/rename 明确返回 400 Unsupported，404 仅表示 Pi 会话不存在）；错误统一 `{ error, detail }`（400/404/409/500）。

## 统计口径（口径 A）

- **计入口径（四载体）**：`assistant`（`type=message, role=assistant`）、`toolResult`（`role=toolResult`）、`compaction`（`type=compaction`）、`branch_summary`（`type=branch_summary`）四者，门控 `has_billable||has_cost||failed` 任一成立即计入；其余 `user` 等一律不计入
- **fork 会话去重**：header 含 `parentSession` 的 fork 会话，其复制历史（`message.timestamp < header.timestamp`，fork 创建时间）的 usage 已在父会话统计过，Pi source adapter 在写入 ledger 前剔除；fork 后新增消息保留（CLI/API/WebUI/Watch 一致；与 pi-switch 网关统计对齐，8/1 起累计差异 ≈0.6%——8/2、8/4 分毫不差，剩余为覆盖结构，见下「与网关对比」）
- **请求数**：四载体中通过 billable/cost/failed 门控的计入口径消息数；全 0 usage 的失败/中止消息也计入请求数（token 为 0）
- **总 token**：总输入 + 输出 = `input + cacheRead + output`（对齐 pi-switch 网关 total；不含 cacheWrite，见 ADR-0002 与 `CONTEXT.md`）
- **缓存率**：`cacheRead / (input + cacheRead)`（分母不含 cacheWrite，ADR-0002）；分母为 0 记 0；聚合先求和分子分母再除
- **花费**：上报的 `usage.cost.total` 优先；缺失或非正时按 `model_pricing` 回算，二者都不可用才为 0 并标注「费率未配置（免费/未定价）」。All 窗口保留可定价 Pi 的美元合计并标注「含 unpriced 源 / 部分可用」（CLI 表格显示 `$X*`，表尾解释 `* 含 unpriced 源`；WebUI 在金额后标注）；Codex 单源仍为 `unpriced`；JSON/CSV 的 `costStatus` 与 `cost` 组合可区分完全未定价、部分可用与真实零花费。
- **模型归属**：请求级（每条消息的 `model` 字段）
- **cwd 归属**：会话 header `cwd` 为权威键，规范化（绝对路径、去尾斜杠、符号链接解析）；目录名有损编码不参与归属
- **时间归属**：CLI `--since`/`--until` 时间筛选按会话 header timestamp 闭区间（含端点）；period 汇总（CLI 与 webui）按每条 usage event 的消息 timestamp 归属，跨天 rollout 会拆分到实际使用日；webui 其余全端点（`totals`/`sessions`/`requests`/`groups`）也按消息 timestamp 消息级（总览与会话明细求和一致）
- **网关可比窗口**：pi-switch 网关数据起点 = `2026-08-01T00:00:00Z`（UTC）；历史 webui「自 8/1」预设已移除（2026-09-01 清理），如需对账可通过自定义输入该起点，切「全部」查看含 8/1 前数据的完整历史
- **时区语义**：webui/CLI 时间参数按本地时区解释（CST 自然日）；网关日志 `ts` 为 UTC——对账时以本地时区解释网关 ts，「今天」边界差 8 小时属预期
- **与网关对比**（2026-08-06 对账）：8/1 起累计 session 940.5M vs 网关 935.0M（差 0.6%）；8/2、8/4 分毫不差；差异全部为覆盖结构——8/1 网关刚启用（仅 4 条记录，+37.2M）、8/3/8/5 网关多出其他客户端请求、pi 直连请求只在 session 目录；CLI 全量窗口含 8/1 前数据（≈494M）与网关不可比
- **与网关对比（2026-08-07 对账）**：逐条匹配（时间戳+token 数）确认两侧定价**完全一致**（679 条 0 差异），「今日」金额差异（约 $0.2/天）全部为覆盖结构：① pi 压缩/摘要等**内部请求**（无会话名、真实计费，约 $0.10/天）不入 session 对话流（compaction entry 无 usage 字段）→ 结构性漏算；② **其他客户端请求**（opencode dreamer 后台任务，约 $0.09/天）只在网关。用户决策：结构性接受、不引入网关数据源；webui 已加口径说明（见 `.scratch/webui-gateway-disclaimer/`）

## 结构

```
CONTEXT.md              领域术语表（统计口径、fork 会话、字段语义）
docs/adr/               架构决策记录（0001～0005，终态见 0005 Go-only 后端）
Makefile                多平台交叉编译与自动化测试脚本（Go-only，无 Node 依赖）
go.mod                  Go 模块 `github.com/heihei0299/token-analyzer`（纯 Go zstd decoder 与 SQLite 依赖）
cmd/token-analyzer/     Go CLI 与 Serve 统一主程序入口
internal/
  pi/                   Pi source adapter（discovery/parse/identity/dedup/增量 refresh，fork 去重与双账本在此终结）
  codex/                Codex source adapter（durable usage、plain/zstd、diagnostics）
  db/                   normalized SQLite ledger（usage 唯一事实中心）
  query/                统一 Go Query Engine（totals/sessions/requests/groups/period/detail/meta，只读快照）
  refresh/              Refresh 编排（source → ledger，串行化，失败保快照并经 meta 暴露）
  domain/               核心聚合模型与指标定义（ADR-0002 口径、Totals）
  sessiondata/          共享查询类型（Filter/View/QueryResult）与 cwd/显示名/排序 helper
  timerange/            CST 本地时区双语义时间引擎（严格公历校验）
  server/               原生 net/http 服务（内嵌 webui.html 单一源码、会话重命名防护、change→refresh→query watch）
  render/               终端 ASCII 格式化表格渲染
  serialize/            JSON / CSV 序列化器
testdata/canonical/     synthetic fixtures + golden expected（长期行为契约，Pi/Codex 字段级断言）
dist/                   构建产物（Go 多平台交叉编译二进制）
opencode-analyzer/      独立 OpenCode Analyzer Go module（CLI/API/WebUI/storage/tests，可整目录迁出）
.scratch/               功能规格与 issue（背景材料，非规范入口）
.github/workflows/      自动化 CI / 发布流程（Go-only：ci.yml 测试 + release.yml 发版）
```
