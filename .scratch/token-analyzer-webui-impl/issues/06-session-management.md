# 06 — 会话管理 tab（按项目分组 + 重命名会话）

**What to build:** 第四个 tab「会话管理」：按 cwd 项目分组展示全部会话（每组可折叠，组内行 = 显示名/时间/模型/请求数/总token），行内重命名按钮。后端补 `/api/sessions` 的 fileName 字段与 `POST /api/sessions/rename` 端点（改文件名 `<显示名>_<UUID>.jsonl` 保留尾 UUID，仅非活跃会话 mtime>5min 可改，重名/活跃 409、非法名 400、会话不存在 404；不改写文件内容，统计口径不受影响）。spec 依据：`.scratch/token-analyzer-webui/spec.md` 的「会话管理 tab」。

**Blocked by:** 02 — HTTP API 端点集；04 — 明细视图（tab 骨架）

**Status:** resolved

- [x] 会话管理 tab 按 cwd 分组展示全部会话，组可折叠，每行含显示名/时间/模型/请求数/总token
- [x] `/api/sessions` 每行新增 fileName 字段；显示名 = 去尾 `_<UUID>.jsonl` 的前缀（无前缀显示原始时间戳名）
- [x] `POST /api/sessions/rename`：成功改名且尾 UUID 保留、header 内容未改、`/api/totals` 统计不变
- [x] 非法显示名（含路径分隔符/非法字符/空名）→ 400
- [x] 会话不存在 → 404
- [x] 活跃会话（文件 mtime ≤ 5min）→ 409「会话活跃中，稍后再试」
- [x] 目标目录同名文件已存在 → 409「同名文件已存在」
- [x] 前端行内重命名交互 + 错误提示（409/400/404 分别人读提示）

## 实施总结
- 提交：`7d1cbbd` — feat: 会话管理 tab（按 cwd 分组 + 重命名）+ /api/sessions 扩展字段 + rename 端点
- 实现的 seams：S17 /api/sessions 扩展 fileName/displayName/cwdNorm（显示名派生：时间戳前缀/ni/中文名/无 `_` 尾缀→原始名）/ S18 重命名成功（尾 UUID=header id 保留、header 未改、totals 不变）/ S19 非法名 400（空/纯空白/纯非法字符；混合非法字符去除后照常改名）/ S20 不存在 404 / S21 活跃 409 + 重名 409 / S22 会话管理 UI 骨架（分组容器/折叠/行内重命名按钮）
- 验收标准：8 条全部 `- [x]`（见上；行为级实现完成，自动化断言 HTTP seam + 文件系统副作用，UI 交互走手工验收）
- 测试结果：6/6 全绿（test/11-session-management.test.ts）；完整套件 71/71
- typecheck：通过（npm run typecheck）
- 文档对齐：README 的 webui 描述在收尾阶段补齐（见下一 commit）
- 遗留 / 后续建议：修复了整体 review 发现的请求明细「缓存」列显示空（cacheRead+cacheWrite 合并）与 server 请求处理器 rejection 兜底；`return renameSession(...)` 曾因缺 await 导致 ApiError 逃逸（handleApi try/catch 不捕获 `return promise` 的 rejection），已修复为 `return await`
