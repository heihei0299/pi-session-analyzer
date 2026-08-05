# 10 — 实时监控途径研究

Type: research
Status: resolved

## Question

实时监控 pi 进程 token 消耗的**技术途径**是什么？

具体子问题：
1. 会话 JSONL 是否 **append-only**？（新消息追加到文件尾部 → 可 tail / 轮询增量；写盘时机与频率）
2. pi 是否有**运行时事件 / 插件 API** 暴露 token 消耗（hooks、订阅、日志）？
3. 对分析工具的推荐方案：tail -f vs 轮询 mtime 增量 vs pi 插件/事件——各自约束（性能、完整性、延迟）
4. 增量读取的边界：一次请求的 usage 何时完整落盘（请求完成才写，还是流式分片写）？

## 背景

用户于 2026-08 明确要求工具支持"实时监控 pi 进程"（map Destination 修订；原 Out of scope 条目已移除）。「会话边界识别」（ticket 03）已确认文件=会话、首行 header 结构——本 ticket 补足实时增量读取途径。

## 调研指引

- 权威来源：pi 源码 `pi-agent-core/dist/harness/session/jsonl-storage.js`（写入逻辑）、插件/hooks 文档（`docs/` 下 extensions / hooks）、`~/.local/share/fnm/node-versions/v24.16.0/installation/lib/node_modules/@earendil-works/pi-coding-agent/`
- 数据源：`/home/shial/.pi/agent/sessions/` 会话样本（直接 bash 绝对路径访问，勿用 fffind 索引）
- 每个结论给出证据：源码路径 + 行号，或会话文件样本

## Answer

### 1. 会话 JSONL 是 append-only，写盘时机为消息级即时

- 证据：`pi-agent-core/dist/harness/session/jsonl-storage.js:110` `appendEntry` 用 `fs.appendFile(filePath, JSON.stringify(entry)+"\n")`；`:58` `setLeafId` 同样 appendFile；`:75` `create` 仅 `writeFile` 一次写首行 header。全文件无整体重写路径（compaction 也是追加 compaction 条目，不重写旧行）。
- 推论：新消息恒追加文件尾，可安全 tail / 轮询增量；每行 = 一条完整 JSON 记录（带 \n 结尾）。
- 写盘频率：assistant 消息在 `message_end` 事件处理器内**立即 await append**（`agent-harness.js:455-457` → `appendMessage` → `appendEntry`）；turn 级小条目（model_change / thinking_level_change / active_tools_change）先入 `pendingSessionWrites` 队列，在 `prepareNextTurn`/`turn_end`/`agent_end`/`executeTurn` finally 统一 flush（`agent-harness.js:421-451, 468-476, 547`）。

### 2. 有运行时事件/插件 API 暴露 token 消耗

- `docs/extensions.md:588-616`：`message_start / message_update / message_end` hooks；`message_end` 的 `event.message.usage` 携带完整 usage（含 `cost`），官方示例即按 `event.message.role === "assistant"` 过滤。
- `docs/extensions.md:1038-1041`：`ctx.getContextUsage()` 返回当前上下文 usage。`tool_result` hook 亦带 usage（`extensions.md:814-845`），但那是 tool 调用返回的嵌套模型 usage，非主请求。
- 约束：hooks 在 pi 进程内（扩展形式注入）执行，外部独立监控进程无法直接订阅。

### 3. 推荐方案：tail -f 为主 + 轮询兜底；插件/事件仅作进程内选项

| 方案 | 延迟 | 完整性 | 性能/侵入性 |
|---|---|---|---|
| **tail -f + 逐行 JSON.parse（推荐）** | 最低：写盘即见（appendFile 即时、消息粒度） | 高：append-only + 单行原子记录，从上次位置续读不丢 | 零侵入；读侧成本≈JSONL 解析 |
| 轮询 mtime + 行数增量 | 轮询间隔（如 1s） | 高；需比对 size/inode 处理文件替换/截断 | 零侵入；实现简单、无长驻句柄 |
| pi 插件/事件（message_end hook） | 事件级，早于文件写盘 | 高（结构化 usage+cost 对象） | 侵入：扩展注入 pi 进程，与 pi 生命周期耦合、版本敏感 |

结论：**tail -f 为主**（低延迟、零侵入、单行原子），辅以定时 mtime/行数轮询做断线/文件替换兜底重同步。完整 usage 只出现在 role=assistant 的 message entry，增量解析时据此过滤。

### 4. 增量读取边界：一次请求的 usage 在请求完成后一次性落盘（非流式分片）

- 流式过程中 `text_delta`/`thinking_delta`/`toolcall_delta` 只 emit `message_update`，不写盘（`agent-loop.js:222-239`）。
- 流式结束（done/error）后 `await response.result()` 得到含完整 usage 的 finalMessage 并 emit `message_end`（`agent-loop.js:240-242`），harness 随即 append（`agent-harness.js:455-457`）。
- 结论：读侧以「一行完整 message entry（role=assistant、含 usage 字段）」为增量边界；usage 字段语义（input/output/cacheRead/cacheWrite/cost）见 ticket 01。
