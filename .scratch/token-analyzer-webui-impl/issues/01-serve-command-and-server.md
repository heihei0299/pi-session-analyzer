# 01 — serve 子命令与最小 HTTP 服务器

**What to build:** `node src/cli.ts serve` 启动本地 HTTP 服务器，浏览器访问 `/` 得到最小占位 HTML 页面（验证单 HTML 内联静态资源机制）；未知路径返回统一 JSON 错误体；端口占用有友好提示；Ctrl+C 优雅退出。`--port/--host/--dir` 参数解析与 serve 模式参数校验。spec 依据：`.scratch/token-analyzer-webui/spec.md` 的「运行形态」。

**Blocked by:** None — can start immediately.

**Status:** resolved

- [x] `node src/cli.ts serve` 启动服务器并打印访问 URL（http://127.0.0.1:50080/）
- [x] 默认端口 50080、默认 host 127.0.0.1；`--port` / `--host` / `--dir` 可覆盖
- [x] serve 模式传入窗口参数（totals/sessions/requests）时报错拒绝
- [x] `GET /` 返回 200 `text/html`（最小占位页，验证单 HTML 内联）
- [x] 未知路径返回 404 + 统一 JSON 错误体 `{ error, detail }`
- [x] 端口被占用时报 EADDRINUSE 友好提示（「端口 X 已被占用，可用 --port 更换」）
- [x] Ctrl+C 优雅退出（关闭 server）

## 实施总结
- 提交：`d17be94` — feat: serve 子命令与最小 HTTP 服务器（零依赖、路由分发、EADDRINUSE 友好提示、Ctrl+C 优雅退出）
- 实现的 seams：S1 parseArgs serve 解析与校验（默认 50080/127.0.0.1/默认目录，拒绝窗口位置参数与全部非 serve 参数，非法端口报错）/ S2 GET / 与 /index.html 200 text/html / S3 未知路径 404 `{ error, detail }` / S4 EADDRINUSE 友好提示 / S5 子进程 SIGINT 优雅退出（退出码 0）
- 验收标准：7 条全部 `- [x]`（见上）
- 测试结果：8/8 全绿（test/06-serve.test.ts）
- typecheck：通过（npm run typecheck）
- 文档对齐：README 的 serve 用法在 ticket 06 收尾一次性补齐（本 ticket 未描述 serve，无文档失配）
- 遗留 / 后续建议：/api/* 路由当前返回 404，由 ticket 02 接入 API 层
