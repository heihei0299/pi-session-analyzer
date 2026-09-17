# 12: 对齐 review remediation 的验收证据

**What to build:** 让本地 issue tracker 的状态、验收 checklist 和实现/review 证据一致，使 resolved 不再与全未勾选条目或未解决的阻塞问题同时存在。

**Blocked by:** 09: 固定 root identity，消除 Refresh 与 rename 的 TOCTOU；10: 让无 Pi root 的 All source 保持一致可用；11: 持久化 Pi mutation 与 commit failure diagnostics

**Status:** resolved

- [x] Issue 01 和 02 的 checklist 已按已有真实验证结果更新，不再保留 resolved + 全部未勾选的矛盾状态。
- [x] 09、10、11 的 acceptance criteria 已按本机授权的全量验证结果勾选（10 的未采纳分支除外）。
- [x] 每个 resolved ticket 都记录修改摘要、验证命令/结果和剩余阻塞。
- [x] 文档中的 root、All source、diagnostics 和 release/Go-only 语义与当前实现和 ADR 一致。
- [x] 不通过删除历史记录、伪造验证结果或提前修改状态来隐藏未解决问题。
- [x] 完成后可从 issue tracker 重建本轮整改闭环，并明确等待最终 review，而不是自行宣称 REVIEW PASS。

## Comments

- Source spec: `.scratch/token-analyzer-maintainability-review/spec.md`
- Review finding: `5feee53` / `09aff72` checklist evidence mismatch
- Triage: ready-for-agent

---

## Completion note

### 第二轮（remediation）

- 证据同步：Issue 01/02 保持已根据既有记录勾选；Issue 09/10/11 保持 `claimed`，未把未执行的 Go 测试写成行为验证，本轮为三项 finding 补写实现摘要、回归测试清单与“未执行”声明。
- 文档同步：README、CONTEXT 和 ADR-0005 已补充（1）校验后固定 canonical physical root、拒绝候选 project/`.jsonl` symlink；（2）无 Pi root 的 Codex-only All 仅在 ledger 无 token-analyzer 自有 Pi history 时成立，legacy Pi history 一律 fail closed 且不自动认领。
- 静态验证：改动 Go 文件 `gofmt -l` 无输出；`git diff --check` 通过。未执行 `go test`、`go vet`、build 或全量测试（未获授权）。
- 剩余阻塞：09–11 的聚焦 Go 测试与 review；本轮 commit 后等待 w9:p1 review。
- Awaiting: review。

### 第三轮（review pass）

- Issue 09/10/11 已根据本机授权的验证结果勾选 acceptance（issue 10 的“All 必须包含 Pi”未采纳分支保持未勾选，并注明产品选择 Codex-only All）。
- 验证证据（w9:p1 review 独立复跑一致）：`test -z "$(gofmt -l $(git ls-files '*.go'))"` → exit 0；`GOMAXPROCS=2 go vet ./...` → exit 0；`TOKEN_ANALYZER_DB= GOMAXPROCS=2 go test -p 1 -parallel 1 ./...` → 154 passed in 12 packages，exit 0。
- 涉及 commit：`4b38290`、`1f51c11`（本轮整改）；`9135c81`（独立基线 gofmt，仅内部测试文件格式）。
- Review：REVIEW_ROUND_4 REVIEW PASS；无剩余阻塞；未执行项仅 `make release`、交叉编译、打包、安装（未授权）。
