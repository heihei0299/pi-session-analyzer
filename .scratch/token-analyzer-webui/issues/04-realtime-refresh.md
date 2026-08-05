# 自动刷新机制（对应 CLI --watch）

**Map**: token-analyzer-webui — see `.scratch/token-analyzer-webui/map.md`
**Type**: grilling（HITL 决策，产出 spec 段落）
**Status**: resolved

## Answer

**决策（用户 grilling 确认，全部按推荐）**：

### 后端机制（全量重算）

- 自动刷新与静态加载共用同一路径：每次请求 `readSessionFiles(dir)` 全量读取 → 按当前筛选参数过滤/分组/汇总，无常驻增量状态
- **不**复用 `watch.ts` 的 `IncrementalReader`（增量状态与 02 的多端点设计冲突：各端点都要叠增量，复杂度不匹配收益）
- 212 会话秒级处理，全量重算在 5s 最低轮询档下开销可接受

### 轮询间隔（Off/5s/30s/5min）

- 下拉选项：**Off / 5s / 30s / 5min**，默认 **Off**（用户主动开启）
- 前端 `setInterval` 轮询当前可见 tab 所需端点（总览= totals+groups；会话明细=sessions；请求明细=requests），切换 tab 时重建定时器
- 切换为 Off 时清除定时器

### 筛选组合（照常生效）

- 自动刷新时，当前已选的 model/cwd/时间筛选**照常生效**——全量重算每次按筛选参数重新过滤，无边界歧义
- 与 CLI `--watch` 不支持筛选形成对照（webui 因全量重算而天然支持；CLI 增量边界语义不清才禁止）
- spec 需注明此差异，避免实现时误对齐 CLI 行为

### 浏览器侧（静默替换 + 更新提示）

- 轮询请求期间页面保持旧数据渲染（不闪烁）；新数据到达后一次性替换 DOM
- 若新数据与旧数据有变化（totals 任一数值或明细行数/内容变化）→ 状态行显示「已更新 HH:MM:SS」提示；无变化则静默
- 手动「刷新」按钮行为一致（立即拉取一次）
**Blocked by**: 02-http-api-design

## Question

webui 的自动刷新如何实现，对应 CLI 的 `--watch` 实时监控能力？

- **后端机制**：页面定时轮询 API 时，服务端**全量重算**（每次请求重新 `readSessionFiles` + 聚合——212 会话秒级处理，实现最简单、口径天然一致）vs 复用 `watch.ts` 的 `IncrementalReader` 维护内存增量状态（省 IO 但引入状态与筛选组合的复杂度）？倾向哪个？
- **轮询间隔**：前端定时器轮询（参考页 Auto-refresh 下拉：Off / 5s / 30s / 5min）？间隔选项是否原样照搬？默认 Off 还是开启？
- **与筛选组合**：CLI `--watch` 明确不支持 `--model/--cwd/--since/--until` 组合（实时增量边界语义不清）——webui 自动刷新时，用户已选的模型/cwd/时间筛选是否允许生效？若允许，全量重算天然支持（每次按筛选重算）；若增量方案，筛选如何套用？
- **浏览器侧**：轮询期间页面闪烁问题（刷新时保持旧数据直到新数据到达）？变更提示（参考页是否提示「有新数据」）？

**resolve 时应产出**：spec 中「自动刷新机制」段落（后端机制、轮询间隔、筛选组合语义、浏览器侧行为）。

## Comments
