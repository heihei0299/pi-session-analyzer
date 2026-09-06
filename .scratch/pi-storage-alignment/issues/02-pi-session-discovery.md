# 02: Pi 会话发现（双布局 + 环境变量）

**What to build:** `resolvePiSessionRoot()` 与 `collectPiJsonlFiles()` 按 cc-switch `providers/pi.rs` 双布局实现：`PI_CODING_AGENT_SESSION_DIR`（绝对路径才可枚举）> `pi native defaults.session_dir` > `~/.pi/agent/sessions`，`Flat`（根下 `*.jsonl`）vs `ProjectDirectories`（`sessions/<project>/*.jsonl`）按布局枚举，相对路径直接报错 `PI_SESSION_DIR_REQUIRES_PROJECT_CONTEXT`（HTTP 400）。

**Blocked by:** 01: SQLite 持久化基座

**Status:** resolved

- [x] `resolvePiSessionRoot()` 按三级优先级解析，绝对/相对路径判定与 `requires project cwd` 报错一致
- [x] `Flat` 布局仅枚举根下 `*.jsonl`，`ProjectDirectories` 仅枚举 `<project>/*.jsonl` 两层，不再递归兜底
- [x] `PI_CODING_AGENT_SESSION_DIR` 环境变量优先于配置与默认，与 cc-switch 行为一致
- [x] `collectPiJsonlFiles` 返回排序列表，`data/` 忽略，相对路径场景 `400` 错误体 `{ error, detail }`
- [x] Node 与 Go 双实现一致，`--dir` 显式覆盖仍生效

## 实施总结
- 提交：`15a3cb7` — `feat(pi-storage): SQLite 基座与四载体解析直切 cc-switch (01-06)`
- 实现的 seams：见上方验收清单
- 验收标准：全部 `- [x]`
- 测试结果：对应 30-35 测试全绿（37 项），typecheck 通过，go vet 通过
- 文档对齐：CONTEXT.md 与 ADR-0003 已对齐（后续 07-08 另提交）
