# 07: 自动化 CI、版本与发布闭环

**What to build:** 让 pull request、主分支和 release 都能自动验证 token-analyzer 自己拥有的代码和产物；版本、测试、构建、文档和正式迁出后的 OpenCode 产品边界保持一致。

**Blocked by:** 01: 降低 Query Engine 的变更半径；02: 建立可重放的 ledger schema migration；03: 统一 CLI/HTTP/Refresh 输入与 capability 校验；04: 让 source adapter 错误与诊断可观察；05: 锁定 Query 的 rollup、排序与分页契约；06: 收口运行时私有结构并删除死代码

**Status:** resolved

- [x] CI 在 pull request 和主分支变更时自动运行，而不是只依赖手动触发或 release tag。
- [x] CI 执行 Go 格式检查、标准静态检查和 root token-analyzer module 的串行测试。
- [x] CI/release 不再引用已迁出的 OpenCode Analyzer 目录；其测试和发布由外部项目负责。
- [x] release 保留跨平台构建、checksums、`--help` 和 `--version` smoke check。
- [x] CLI version、构建版本和 release tag 只有一个权威来源，不会发生版本漂移。
- [x] 构建仍保持 Go-only 单二进制和无运行时 Node/npm 依赖。
- [x] README、CONTEXT、ADR 和 Go-only boundary test 都描述正式迁出后的仓库布局。
- [x] 历史 audit/migration 文档可以保留，但不会被 CI 或当前实现当作执行入口。
