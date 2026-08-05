# HTTP API 设计（前端取数端点）

**Map**: token-analyzer-webui — see `.scratch/token-analyzer-webui/map.md`
**Type**: grilling（HITL 决策，产出 spec 段落）
**Status**: resolved

## Answer

**决策（用户 grilling 确认，全部按推荐）**：

### 端点集（多端点细粒度）

| 端点 | 参数 | 响应（裸 JSON） |
|---|---|---|
| `GET /api/totals` | model/cwd/since/until | `Totals` 对象（requests/input/output/cacheRead/cacheWrite/reasoning/totalTokens/cost/cacheRate） |
| `GET /api/sessions` | model/cwd/since/until | `{ window: "sessions", rows: SessionRow[] }`（每行含 sessionId/timestamp/cwd/model + 指标） |
| `GET /api/requests` | model/cwd/since/until | `{ window: "requests", rows: RequestRow[] }`（每行含 sessionId/timestamp/model + 指标） |
| `GET /api/groups` | `by=model|cwd|model,cwd` + 筛选 | `{ window: "totals", by, rows: GroupRow[] }`（每行含分组键 model/cwd + 指标） |
| `GET /api/period` | `period=day|week|month` + 筛选 | `{ window: "totals", period, rows: PeriodRow[] }`（每行含 period + 指标） |
| `GET /api/meta` | 无 | `{ dir, sessionCount, dataRange: { since, until } }`（数据目录、合法会话数、数据时间范围，供页面标题/状态栏） |

字段结构与现有 `serialize.ts` 的 JSON 输出完全一致（复用 `totalsToObject`/`sessionToObject`/`requestToObject`/`groupToObject`/`periodToObject`），驼峰命名，cacheRate 为 0-1 小数。

### 筛选参数（直接映射 CLI）

- `model` / `cwd` / `since` / `until` 语义与 CLI 完全一致：since/until 闭区间（含端点）、cwd 规范化比较、按会话时间戳归属
- 复用现有 `filterFiles(files, { model, cwd, since, until })` 直接映射，无额外换算层
- `since`/`until` 非法时间格式 → 400

### 响应结构

- 数据端点返回**裸 JSON**（不加 envelope 包装），与现有 serialize 类型一致
- 元数据独立于 `/api/meta` 端点，数据端点不携带元信息

### 错误处理（统一 JSON 错误体）

- 错误响应一律 `{ error: "<机器可读代码>", detail: "<人读描述>" }` + 对应 HTTP 状态码
- 400：查询参数非法（未知 by/period 值、since/until 非时间格式）
- 404：未知路径（`/api/*` 未匹配端点）
- 500：数据目录不可读 / 无合法会话（detail 附原因）

### 实现提示（供实现 effort）

- 服务端每次请求时 `readSessionFiles(dir)` 全量读取后按参数过滤/分组/汇总（212 会话秒级处理，无需常驻状态）
- 路由前缀 `/api/` 与静态 HTML 在同一 http server 内分发（见 ticket 01）
**Blocked by**: 01-serve-command-and-server

## Question

前端页面通过什么 JSON 端点取数？端点集与参数如何设计？

- **端点集**：复用现有数据层，设计 REST-ish 端点。候选：`GET /api/totals`（总消耗）、`GET /api/sessions`（会话级明细）、`GET /api/requests`（单请求明细）、`GET /api/groups?by=model|cwd|model,cwd`（分组）、`GET /api/period?period=day|week|month`（时间汇总）。是否够用？是否需要独立端点返回「全部」数据供前端一次拉取（212 会话秒级处理，单端点简单）vs 多端点细粒度？
- **筛选参数**：`model` / `cwd` / `since` / `until` 如何映射现有 `filterFiles`？参数校验与 400 错误响应格式？
- **响应结构**：直接复用现有类型（`Totals` / `SessionRow[]` / `RequestRow[]` / `GroupRow[]` / `PeriodRow[]`）序列化为 JSON？响应是否含元数据（数据目录路径、文件数、处理耗时、数据时间范围）？
- **错误处理**：数据目录不存在 / 无合法会话时的响应？404 路由回退？

**resolve 时应产出**：spec 中「HTTP API 设计」段落（端点集、筛选参数映射、响应结构、错误处理），供前端与实现 effort 对照。

## Comments
