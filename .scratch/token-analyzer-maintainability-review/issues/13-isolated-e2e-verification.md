# 13: 隔离环境与限并发下的端到端验证

**What to build:** 在隔离环境、限制并发的前提下，用真实构建产物端到端跑通 CLI 与 HTTP 用户路径，证明 09–11 的整改在组装后的二进制上成立，而不是只在单元/集成测试里成立。

**Blocked by:** None (can start immediately；09–11 已 REVIEW PASS 并 resolved)

**Status:** resolved

- [x] 使用临时隔离环境（临时 HOME、临时 ledger、临时 Pi session root、临时 Codex home），全程不触碰真实 `~/.pi`、`~/.cache/token-analyzer`、cc-switch 或任何真实 ledger；测试结束后临时目录可整体删除。
- [x] 并发受限：Go 命令使用 `GOMAXPROCS=2`；端到端步骤串行执行；同一时刻只运行一个被测进程；不并行发起 HTTP 请求；不进行压力/负载测试。
- [x] 只构建当前平台的 CLI 二进制用于验证；不执行 `make release`、交叉编译、打包或安装。
- [x] CLI 端到端：`--version`、`--help`、`totals|sessions|requests`、`--source pi|codex|all`、`--format table|json|csv`、`--by model|cwd|model,cwd`、`--period day|week|month`、模型/cwd/时间过滤均按当前契约返回。
- [x] CLI 非法输入端到端：未知 source、codex/all 的 requests、非法 group/period/format、`--watch` 非正 interval 都明确失败且非 0 退出，且不创建或维护 ledger。
- [x] HTTP 端到端：`serve` 在 127.0.0.1 上顺序请求 totals/sessions/requests/groups/period/meta/db/meta，成功响应结构与状态码正确；`/api/db/meta` 报告临时 ledger 路径与 schema version。
- [x] HTTP 错误契约端到端：非法 source/分页/排序返回稳定 400；超大 page/size 不 panic；Codex/All 的 requests 与 Pi 专属详情/重命名返回明确 unsupported；root mismatch 返回 409。
- [x] root 隔离端到端：先用 root A 建立 binding，再用 root B 查询/重命名被拒绝且不修改 B 下文件；无 binding 的 legacy Pi 历史在 `source=all` 下 fail closed。
- [x] Pi 重命名端到端：对非活跃会话文件成功重命名后，同一 sessionId 的详情仍可查询；失败路径不伪装成功且外部文件不变。
- [x] watch 冒烟：`--watch` 在限时内正常运行、打印 totals，并只刷新配置的 source，不产生独立统计路径。
- [x] 记录实际命令、退出码、关键输出与隔离证据；发现真实缺陷时另开修复 commit，不修改断言或跳过用例来规避。（本轮未发现缺陷，无修复 commit）
- [x] 结果落盘为可版本化的证据文件，并按仓库规则提交 commit。

## Comments

- Source spec: `.scratch/token-analyzer-maintainability-review/spec.md`
- 前置：09/10/11/12 已 `REVIEW PASS`（commits 4b38290、1f51c11、9135c81）
- Triage: ready-for-agent

---

## Completion note

- 环境：`mktemp -d` 临时根 `/tmp/ta-e2e.gMpdjH`，`HOME`/`TOKEN_ANALYZER_DB`/`PI_CODING_AGENT_SESSION_DIR`/`CODEX_HOME` 全部指向临时根；Pi 用 `testdata/canonical/pi` fixture（flat 复制）+ 合成 rename 会话，Codex 用临时 `CODEX_HOME` 内合成 rollout。
- 并发：`GOMAXPROCS=2`；步骤串行、单进程；`serve` 只监听 `127.0.0.1:51837/51838`，测试后进程关闭、端口确认未监听；curl 顺序执行，无压力测试。
- 构建：`GOMAXPROCS=2 go build -o <tmp>/e2e/bin/token-analyzer ./cmd/token-analyzer` → exit=0（25.6s），`--version` = `token-analyzer dev`。
- 结果：CLI 正常路径 26/26 exit=0；非法输入 9/9 exit=2 且 `invalid.db` 未创建、主 ledger sha256 不变；HTTP 7 个端点全部 200（`/api/db/meta` 报告临时 ledger 路径 + `schemaVersion=3`）；错误契约 400/404/409 与超大 page/size 不 panic 均符合；root B 查询/详情/重命名全 409 且 B 下文件 sha256 前后一致；删除 binding 后 `--source all` 与 `--source pi` 均 exit=1（`source root binding required`）且历史行数不变、binding 仍为 0；rename 成功 + 详情可查，失败路径 404/400/409 不伪装成功；watch（pi/codex）限时运行打印 totals，codex watch 前后 Pi 游标集合一致（`7|4861`）。
- 隔离证据：临时根文件清单 + 真实 `~/.cache/token-analyzer`（mtime 1789679691）、`~/.pi`（1788387116）、`~/.cc-switch`（1789668956）mtime 与测试前一致；曾用的 `/tmp/ta-*` 临时重定向与路径指针文件已删除，内容归档到 `<ROOT>/e2e/evidence/scratch/`。
- 缺陷：未发现需要修复的端到端缺陷，本轮无修复 commit；未修改任何生产代码/测试。
- 证据文件：`.scratch/token-analyzer-maintainability-review/e2e-evidence.md`（原始日志保留在临时根 `evidence/`）。
- 未执行（未授权）：`make release`、交叉编译、打包、安装、发布、压力/负载测试。
- Awaiting: review。
