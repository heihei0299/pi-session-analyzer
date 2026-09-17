# 03: 统一 CLI/HTTP/Refresh 输入与 capability 校验

**What to build:** 让 CLI 用户和 HTTP 客户端面对非法参数时得到一致、明确、可处理的错误；非法请求不会先打开或修改 ledger，也不会因 watch interval 等输入触发进程 panic。

**Blocked by:** 01: 降低 Query Engine 的变更半径

**Status:** resolved

- [x] source、view、format、group、period、时间范围、分页和排序参数在统一入口按支持能力校验。
- [x] Refresh 在执行数据库写入或 rollup/prune 前拒绝未知 source 和其他非法同步配置。
- [x] CLI 的未知 command、未知 format、非法 group/period 和非法 watch interval 返回明确错误并以非成功状态退出。
- [x] HTTP 对非法参数返回稳定的 400 error contract；合法请求的成功响应结构不变。
- [x] `--watch` 的零或负 interval 被明确拒绝，不构造会 panic 的 ticker。
- [x] Pi-only 的详情和重命名能力在 Codex/All source 下继续返回明确 unsupported，且不会触碰 Pi 文件。
- [x] 重命名请求体有大小限制，非法 JSON、空名称和非法文件名遵循一套与文档一致的策略。
- [x] 测试证明非法输入不会创建、写入或维护 ledger。

## Implementation summary

- Added shared source normalization and query-contract validation before read/write entry points.
- CLI now rejects unknown commands/flags, source/format/group/period/port/watch interval errors before Refresh; HTTP parses pagination and maps invalid query input to 400.
- Bounded rename bodies to 1 MiB, retained sanitization semantics, and rejected control characters/empty sanitized names without touching Pi files.
- Added regression coverage for invalid Refresh and HTTP inputs, no-ledger side effects, CLI option validation, and reversed time ranges.
- Tests: `TOKEN_ANALYZER_DB= GOMAXPROCS=2 go test -p 1 ./cmd/token-analyzer ./internal/server ./internal/refresh ./internal/query ./internal/timerange`; `go vet` on the same packages (pass).
- Commit: deferred to the single request-level feature commit by repository policy.
