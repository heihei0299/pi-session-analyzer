# serve 子命令与零依赖 HTTP 服务器骨架

**Map**: token-analyzer-webui — see `.scratch/token-analyzer-webui/map.md`
**Type**: grilling（HITL 决策，产出 spec 段落）
**Status**: resolved

## Answer

**决策（用户 grilling 确认）**：

- **命令形态**：`serve` **子命令**——`node src/cli.ts serve [--port <n>] [--host <h>] [--dir <path>]`，与窗口参数（totals/sessions/requests）同层；serve 模式下窗口参数非法（校验拒绝，报「serve 模式不接受窗口参数」）
- **默认端口**：**50080**（`--port` 可覆盖；参考 pi-switch 的 43110 风格但错开，用户指定）
- **静态资源**：**单 HTML 内联**——服务端只 serve 一个 index.html（CSS/JS 全部内嵌），前端 fetch API 取数渲染；不做分离静态文件

**实现约定（低风险默认，供实现 effort 参照，如有异议可再改）**：

- `--host`：默认 `127.0.0.1`（仅本机访问；无鉴权前提下的安全默认）
- `--dir`：继承 CLI 现有默认 `~/.pi/agent/sessions/`，可 `--dir` 覆盖
- `--interval`：**不引入**——自动刷新间隔归 ticket 04 决策，由前端控件控制，serve 侧不设轮询参数
- **路由分发**：同一 Node 原生 `http` 服务器内分发——`/` 与 `/index.html` 返回单 HTML；`/api/*` 留待 ticket 02 挂 JSON 端点；其余路径 404
- **MIME**：HTML → `text/html; charset=utf-8`；API JSON → `application/json`；无静态文件则无需其他 MIME
- **生命周期**：端口占用 → `EADDRINUSE` 友好提示（「端口 X 已被占用，可用 --port 更换」）；启动时打印访问 URL（`http://127.0.0.1:50080/`）；Ctrl+C 优雅退出（关闭 server）

**Blocked by**: （无——frontier 起点）

## Question

webui 的服务器骨架如何落地？

- **命令形态**：在现有 `src/cli.ts` 增加 `serve` 子命令（如 `node src/cli.ts serve --port 8080`），还是 `--serve` 标志？与现有窗口参数（totals/sessions/requests）如何区分？
- **参数**：`--port`（默认值？参考 pi-switch 用 43110，我们用什么？）、`--host`（默认 `127.0.0.1` 仅本机）、`--dir`（数据目录，继承现有默认 `~/.pi/agent/sessions/`）、`--interval`（自动刷新间隔，或留给前端控制）？
- **服务器实现**：零依赖下用 Node 原生 `http` 模块——静态资源（HTML/CSS/JS）与 API 路由如何在同一服务器分发？静态资源是单文件内联（HTML 内嵌 CSS/JS）还是分离文件由 `fs` 读取？MIME 类型如何处理？
- **生命周期**：端口占用时如何报错（`EADDRINUSE` 友好提示）？Ctrl+C 优雅退出？是否打印访问 URL 提示？

**resolve 时应产出**：spec 中「serve 子命令与服务器骨架」段落（命令形态、参数、静态资源组织、生命周期），供后续实现 effort 照此落地。

## Comments
