# 02: pi 双布局发现（Flat vs ProjectDirectories）

**What to build:** `resolvePiSessionRoot` 按 `PI_CODING_AGENT_SESSION_DIR`（绝对路径才可枚举，相对路径抛 `PI_SESSION_DIR_REQUIRES_PROJECT_CONTEXT`）> `pi native defaults` > `~/.pi/agent/sessions` 解析，`collectPiJsonlFiles` 按 `Flat`（根下 `*.jsonl`）vs `ProjectDirectories`（`sessions/<project>/*.jsonl`）双布局枚举，不再递归兜底；`--dir` 透传与 `~/.pi-switch` 等其他项目目录隔离。

**Blocked by:** 01: pi 与 cc-switch 统计对齐（共库 + 四窗口对账）

**Status:** resolved

- [x] `src/pi-discovery.ts` + `internal/pi/discovery.go` 已实现 `Flat` vs `ProjectDirectories` 双布局，`collectPiJsonlFiles(root, layout)` 取代递归 `collectJsonlFiles`（已存量）
- [x] `PI_CODING_AGENT_SESSION_DIR` 相对路径抛 `400 PI_SESSION_DIR_REQUIRES_PROJECT_CONTEXT`，绝对路径校验通过 — `31 5/5` 全绿
- [x] `npm test 31` + `go test ./internal/pi -run Discovery` 全绿，`withDirDb` 改双布局枚举（`resolvePiSessionRoot` + `collectPiJsonlFiles` + 回退 `collectJsonlFiles`），`31 5/5` + `38 4/4` 全绿

## 实施总结

- 提交 `withDirDb` 双布局：`src/db-aggregation.ts:XXge` 改 `resolvePiSessionRoot`/`collectPiJsonlFiles` + 回退，`31 5/5` 全绿
- 验收：`npm run typecheck` 全绿，`npm test 31+38` 全绿
