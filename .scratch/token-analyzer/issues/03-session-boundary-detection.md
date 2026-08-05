# 03 — 会话边界识别研究

Type: research
Status: resolved

## Answer

### 1. 一个 JSONL 文件 = 一个会话？（type: session 语义与例外）

全量 212 文件首行 `type` 统计：**session 208 / message 3 / custom 1**。

- `type: session` 文件是唯一合法会话：header 为 `{type, version, id, timestamp, cwd}`，由 `newSession()` 写入（`dist/core/session-manager.js:650-657`）。一个文件 = 一个会话。
- 例外样本：3 个 `type: message` 文件（如 `--home-shial-t--/2026-07-30T22-53-23-712Z_019fb53b-....jsonl`，首行是单条 `role: toolResult` 消息，含 `parentId`）+ 1 个 `type: custom` 文件（`plan-mode-state` 事件，含 `parentId`）——是会话内**单条记录的导出/残留**（parentId 指向的父会话已不在同目录），不是会话。
- **结论：只按首行 `type == "session"` 判会话，message/custom 文件跳过。**

### 2. 目录命名 `--` 分隔的语义、与 cwd 的关系

源码 `dist/core/session-manager.js:245`：
```js
const safePath = `--${resolvedCwd.replace(/^[/\\]/, "").replace(/[/\\:]/g, "-")}--`;
```
即：`--` 包裹 cwd 去掉首 `/` 后把 `/`、`\`、`:` 替换为 `-` 的编码。**目录名 = cwd 的正向编码，一一对应**（33 个目录内 cwd 全部一致，0 个混合）。

但编码有损：路径段内已有的 `-` 不转义，**反向解码有歧义**——实测 208 个会话中 81 个无法从目录名唯一还原 cwd（如 `--home-shial-Project-Pi-guard-test--` 实际 cwd 是 `/home/shial/Project/Pi/guard-test`，而非 `/home/shial/Project/Pi-guard/test`）。

### 3. 文件命名：标准格式与非标准名

- 标准名：`session-manager.js:666-667` `fileTimestamp = timestamp.replace(/[:.]/g, "-")`，文件名 = `{ISO时间戳(:/.→-)}_ {uuidv7()}.jsonl`（sessionId 为 UUIDv7，`session-manager.js:12`，如 `2026-07-31T01-55-30-577Z_019fb5e2-....jsonl`）。
- 非标准名（68 个，如 `生成规范化的commit_019faa60-....jsonl`、`grill-with-docs_....jsonl`）：pi 核心只生成标准名，slug 名来自用户扩展/备份导出。验证 **68/68 文件名尾 UUID == header.id**，且首行均为完整 `type: session` header → **合法会话，必须纳入**。
- **结论：文件名不可作为合法性判据，一律以首行 `type` 为准。**

### 4. cwd 字段作为项目归属依据；与目录名冲突时以谁为准

`cwd` 由 `newSession()` 在会话创建时写入完整绝对路径（`session-manager.js:653` `cwd: this.cwd`），**权威且无歧义**。目录名只是 cwd 的有损编码（Q2 的 81/208 歧义即证据），无法反推。

**冲突/歧义时以首行 `cwd` 为准；目录名仅作展示与快速分组，不作归属判据。**

### 5. 对分析工具的推荐

- **聚合粒度：按规范化后的 `cwd` 聚合为项目归属主键**（绝对路径 `resolve` 去尾斜杠/符号链接），目录名降级为辅助分组标签。
- 处理流程：遍历目录 → 每文件读首行 → `type: session` 才解析 → 取 `header.cwd` 归一化 → 聚合；跳过 message/custom；非标准命名文件照常纳入。
- 可选增强：10 个会话带 `parentSession`（值为父会话文件绝对路径，如 `--home-shial-Project-Pi-chajian--/2026-07-28T18-32-07-208Z_....jsonl`），可识别续接/子会话链避免 token 重复计数；第一版可忽略。
