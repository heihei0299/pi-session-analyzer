# 会话管理 tab（按项目组织 + 重命名会话）

**Map**: token-analyzer-webui — see `.scratch/token-analyzer-webui/map.md`
**Type**: grilling（HITL 决策，产出 spec 段落）
**Status**: resolved

## Answer

**决策（用户 grilling 确认，全部按推荐）**：

### UI 形态（新「会话管理」tab）

- 新增第四个 tab：**会话管理**（总览 / 会话明细 / 请求明细 / 会话管理）
- 按 cwd 项目分组展示全部会话：每组标题 = 规范化 cwd，组内每行 = 显示名 / 时间 / 模型 / 请求数 / 总token
- 每组可折叠展开/收起；行内「重命名」按钮（改名后即时刷新该组）
- 数据源：`/api/sessions`（现有端点，已含 sessionId/timestamp/cwd/model + 指标；显示名由 fileName 派生，见下）

### 重命名机制（改文件名，保留尾 UUID）

- 文件名 `<时间戳>_<UUID>.jsonl` → `<显示名>_<UUID>.jsonl`（尾 UUID 与 header id 一致不变，pi 兼容——仓库已有非标准命名先例）
- 显示名规范化：去除路径分隔符与非法文件名字符（`/` `\` `:` `*` `?` `"` `<` `>` `|` 及首尾空白），空名拒绝
- 不改写文件内容（不动 header、不动 cwd 归属——统计口径不受影响）

### 活跃会话限制（仅非活跃）

- 仅允许重命名**非活跃**会话：文件 mtime 距今 > 5 分钟视为非活跃（可配置常量，spec 定为 5min）
- 活跃会话（mtime ≤ 5min，pi 正在写入）重命名请求 → **409**（detail 注明「会话活跃中，稍后再试」）

### 重名冲突（409 拒绝）

- 目标目录内 `<显示名>_<UUID>.jsonl` 已存在 → **409**（detail 注明「同名文件已存在」）；前端提示用户换名
- 不做自动加序号

### API 补充

- `GET /api/sessions` 响应行**增加 `fileName` 字段**（原始文件名；显示名 = 去尾 `_<UUID>.jsonl` 的前缀，无前缀则为原始时间戳名）——会话管理 tab 由此派生显示名与重命名目标
- 新增 `POST /api/sessions/rename`，body `{ sessionId, name }`：按 header id 定位文件（扫描目录匹配首行 session.id）→ 校验显示名合法 → 校验非活跃 → 校验目标不冲突 → 改名；成功返回 `{ ok: true, fileName }`；失败按错误体（400 非法名 / 404 会话不存在 / 409 活跃或冲突）
- 错误处理沿用 ticket 02 的统一 JSON 错误体 `{ error, detail }`
**Blocked by**: 02-http-api-design

## Question

webui 增加**会话管理**能力：按 cwd 项目分组浏览全部会话，并支持**重命名会话**（直接操作 `~/.pi/agent/sessions/` 下的文件）。

- **UI 形态**：新增「会话管理」tab，按 cwd 项目分组展示所有会话（显示名/时间/模型/请求数/总token），每组可折叠，行内重命名按钮？
- **重命名机制**：改文件名 `<时间戳>_<UUID>.jsonl` → `<显示名>_<UUID>.jsonl`（保留尾 UUID 与 header id 一致，pi 兼容；仓库已有非标准命名先例）？还是旁路映射/写 header？
- **活跃会话**：是否仅允许重命名非活跃会话（文件 mtime 超过阈值无写入）？阈值多少？
- **重名冲突**：目标文件名已存在时 409 拒绝还是自动加序号？
- **API**：重命名端点形态？会话管理 tab 的数据源（需要文件名/显示名，现有 `/api/sessions` 只有 sessionId）？

**resolve 时应产出**：spec 中「会话管理 tab」段落（UI、重命名机制、活跃判定、冲突处理、API 补充），供实现 effort 照此落地。

## Comments
