# Token Analyzer

分析 pi 会话数据（`~/.pi/agent/sessions/` 下的 JSONL 文件）token 消耗的 CLI 工具。读取全部合法会话，按统计口径 A 提取消耗数据，输出总消耗量 / 会话级 / 单请求级三个窗口的指标，支持模型 / cwd 维度拆分、时间维度汇总与筛选、结构化输出（JSON/CSV），并可实时监控正在运行的 pi 进程；`serve` 子命令启动零依赖本地 Web 面板（总览卡片 / 分组表 / 会话与请求明细 / 会话管理），支持时间范围筛选、自动刷新、导出 JSON/CSV 与会话重命名。

功能规格见 [`.scratch/token-analyzer/spec.md`](.scratch/token-analyzer/spec.md)（含实施状态）；实现拆分为 5 个 issue（[`.scratch/token-analyzer-impl/issues/`](.scratch/token-analyzer-impl/issues/)）。WebUI 功能规格见 [`.scratch/token-analyzer-webui/spec.md`](.scratch/token-analyzer-webui/spec.md)，实现拆分为 6 个 issue（[`.scratch/token-analyzer-webui-impl/issues/`](.scratch/token-analyzer-webui-impl/issues/)）。

## 技术栈与运行

- TypeScript + Node 24（原生 type-stripping 直接运行 `.ts`，零运行时依赖）
- 测试：Node 内置 `node:test`（71 用例）
- 无构建步骤：`node src/cli.ts` 直接运行

```bash
npm install      # 安装 typescript + @types/node（devDependencies）
npm test         # 运行全部测试（71 用例）
npm run typecheck  # tsc --noEmit
```

## 用法

```
token-analyzer [totals|sessions|requests] --dir <path> [选项]
```

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
node src/cli.ts

# 按模型分组
node src/cli.ts totals --by model

# 按 cwd 分组（交叉）
node src/cli.ts totals --by model,cwd --cwd /home/shial/Project/pi-session-anylize

# 会话级窗口 + 模型过滤
node src/cli.ts sessions --model deepseek-v4-flash

# 按月汇总 + 时间范围
node src/cli.ts totals --period month --since 2026-07-01 --until 2026-08-31

# JSON 输出（供脚本消费）
node src/cli.ts totals --by model --format json

# 实时监控
node src/cli.ts totals --watch --interval 1000

# 启动 Web 面板（浏览器访问 http://127.0.0.1:50080/）
node src/cli.ts serve
```

## Web 面板（serve）

`node src/cli.ts serve` 启动本地 Web 服务（零依赖，Node 原生 `http` + 单 HTML 内联前端），浏览器访问 `http://127.0.0.1:50080/`：

- **四个 tab**：总览（8 张汇总卡片 + 按模型/cwd 分组表）/ 会话明细 / 请求明细 / 会话管理（按项目 cwd 分组 + 重命名会话）
- **时间范围**：今天 / 7天 / 30天 / 全部 / 自定义（datetime-local，按 UTC 解释），作用于总览与明细与导出
- **自动刷新**：Off / 5s / 30s / 5min（后端每请求全量重算），数据变化时状态行显示「已更新 HH:MM:SS」
- **导出**：JSON（`{ totals, sessions, requests }`）与 CSV（`# totals` / `# sessions` / `# requests` 三段式）下载当前筛选范围
- **会话管理**：按规范化 cwd 分组展示全部会话（组可折叠），点击名称行内编辑重命名——改文件名前缀保留尾 UUID（`<显示名>_<UUID>.jsonl`），仅非活跃会话（mtime > 5min）可改，非法名 400 / 不存在 404 / 活跃与重名 409

HTTP API（`/api/*`，裸 JSON，与 CLI 结构化输出同字段）：`totals` / `sessions` / `requests` / `groups?by=` / `period?period=` / `meta`（筛选参数 `model`/`cwd`/`since`/`until` 映射 CLI 语义）+ `POST /api/sessions/rename`；错误统一 `{ error, detail }`（400/404/409/500）。

## 统计口径（口径 A）

- **计入口径**：仅 `type=message && role=assistant` 且携带 usage 的消息；toolResult / compaction / branch_summary / user 一律忽略
- **请求数**：带 usage 的 assistant 消息数；全 0 usage 的失败/中止消息也计入请求数（token 为 0）
- **总 token**：按组件和 `input + output + cacheRead + cacheWrite` 计算（不信任 `totalTokens` 字段）
- **缓存率**：`cacheRead / (input + cacheRead + cacheWrite)`；分母为 0 记 0；聚合先求和分子分母再除
- **花费**：直接累加 `usage.cost.total`；全 0 花费标注「费率未配置（免费/未定价）」
- **模型归属**：请求级（每条消息的 `model` 字段）
- **cwd 归属**：会话 header `cwd` 为权威键，规范化（绝对路径、去尾斜杠、符号链接解析）；目录名有损编码不参与归属
- **时间归属**：会话 header `timestamp` 为基准；`--since`/`--until` 闭区间（含端点）

## 结构

```
src/
  analyze.ts    数据读取层（目录遍历、合法会话判定、三窗口派生、过滤/分组/时间汇总）
  aggregate.ts  聚合模型（Totals 类型、指标计算）
  render.ts     终端表格渲染
  serialize.ts  JSON / CSV 序列化
  cli.ts        CLI 入口（参数解析、窗口路由、watch/serve 集成）
  watch.ts      实时监控增量读取器
  server.ts     serve HTTP 服务器（路由分发、生命周期、EADDRINUSE 友好提示）
  api.ts        HTTP API 层（端点处理、筛选、统一错误体、会话重命名）
  webui.html    单 HTML 内联前端（深色主题、4 tab、fetch API、自动刷新、导出）
test/           node:test 测试（fixture JSONL → CLI 输出断言；serve → HTTP 端点断言）
```
