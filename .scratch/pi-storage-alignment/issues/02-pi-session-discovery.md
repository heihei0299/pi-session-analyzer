# 02: Pi 会话发现（双布局 + 环境变量）

**What to build:** `resolvePiSessionRoot()` 与 `collectPiJsonlFiles()` 按 cc-switch `providers/pi.rs` 双布局实现：`PI_CODING_AGENT_SESSION_DIR`（绝对路径才可枚举）> `pi native defaults.session_dir` > `~/.pi/agent/sessions`，`Flat`（根下 `*.jsonl`）vs `ProjectDirectories`（`sessions/<project>/*.jsonl`）按布局枚举，相对路径直接报错 `PI_SESSION_DIR_REQUIRES_PROJECT_CONTEXT`（HTTP 400）。

**Blocked by:** 01: SQLite 持久化基座

**Status:** ready-for-agent

- [ ] `resolvePiSessionRoot()` 按三级优先级解析，绝对/相对路径判定与 `requires project cwd` 报错一致
- [ ] `Flat` 布局仅枚举根下 `*.jsonl`，`ProjectDirectories` 仅枚举 `<project>/*.jsonl` 两层，不再递归兜底
- [ ] `PI_CODING_AGENT_SESSION_DIR` 环境变量优先于配置与默认，与 cc-switch 行为一致
- [ ] `collectPiJsonlFiles` 返回排序列表，`data/` 忽略，相对路径场景 `400` 错误体 `{ error, detail }`
- [ ] Node 与 Go 双实现一致，`--dir` 显式覆盖仍生效
