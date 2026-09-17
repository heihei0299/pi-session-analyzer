# Token Analyzer 可维护性计划

Status: ready-for-agent
Type: task

## Problem Statement

作为项目维护者，我需要在不改变既有统计口径和 Go-only 架构的前提下，降低 token-analyzer 的长期维护成本。

当前项目已经具备清晰的主链：Pi/Codex source adapter → Refresh → normalized SQLite ledger → Query Engine → CLI/HTTP/WebUI。领域口径、数据源能力、fork 去重、rollup、时间语义和 OpenCode 产品边界也已经记录在领域术语表与 ADR 中。项目不需要重新设计核心架构。

但当前仍有以下维护风险：

- OpenCode Analyzer 已正式迁出本仓库，但 CI、Go-only boundary test、README 和 ADR 仍残留“同仓 standalone module”的假设；当前测试和 CI 无法代表正式迁出后的仓库状态。
- Query Engine、HTTP server、Pi 增量同步和数据库初始化分别集中在很大的实现文件中，一个小的行为修改可能需要理解多个无关职责。
- Pi 文件读取、定价读取、Codex 诊断持久化等路径存在静默跳过或忽略错误的行为。维护者难以区分“源数据本身没有 usage”和“读取或写入失败导致数据没有进入 ledger”。
- 数据库通过建表、补列和 user version 的组合方式演进，但缺少可顺序重放、可验证失败回滚的显式 migration 结构。
- CLI、HTTP、Refresh 和 Query 的参数校验不完全一致。某些非法 source、group、period、format 或 watch interval 可能在真正执行前没有被明确拒绝。
- 已有 canonical fixture、failure/retry 测试和 server runtime 测试较好，但 CLI 入口、schema migration、错误分类和默认排序仍缺乏同等强度的长期契约。
- CI 目前主要依赖手动触发；版本值在构建配置和 CLI 实现中存在重复来源，发布可重复性仍可加强。

这些问题会增加新维护者的理解成本，放大 review 范围，并可能让数据完整性问题以“统计结果偏小”而不是明确错误的形式出现。

## Solution

建立一套以“行为不变、错误可见、迁移可重放、变更局部、交付可验证”为目标的可维护性基线：

1. 先收口仓库基线，确认 OpenCode Analyzer 已正式迁出，并使测试、CI、文档和边界契约不再依赖该目录。
2. 保持现有 `Refresh → ledger → Query` 主链和单一统计 seam，不恢复 TypeScript/Node 双实现，不建立第二套聚合逻辑。
3. 在现有最高接缝统一 source、view、format、period、group、分页和 watch 参数的验证，使非法输入在任何数据库写入或维护动作之前失败。
4. 统一 source adapter 的错误策略：可恢复的坏记录以诊断呈现，文件级不完整读取不推进游标，数据库 mutation 错误必须回滚并返回，定价和诊断丢失不能静默伪装成正常结果。
5. 将数据库 schema 演进改为顺序、事务化、可测试的 migration，同时保持 normalized SQLite ledger、rollup contract 和现有数据语义。
6. 按职责拆分 Query、server、CLI 和同步实现的私有文件，降低变更半径，但不引入 ORM、依赖注入框架、通用 factory 或新的公共抽象。
7. 用现有 canonical fixture、Refresh/Query contract、SQLite maintenance 和 `httptest` prior art 建立最小但完整的回归矩阵。
8. 将格式化、静态检查、root module/standalone module 测试和发布 smoke check 纳入自动 CI，并使版本来源唯一。

## User Stories

1. As a project maintainer, I want the repository to explicitly record that OpenCode Analyzer has migrated out, so that CI, tests, documentation, and release behavior do not depend on a removed directory.
2. As a project maintainer, I want the root Go-only boundary contract to match the post-migration repository layout, so that a passing boundary test proves the actual product boundary.
3. As a contributor, I want the accepted domain vocabulary and ADR decisions to remain authoritative, so that I do not accidentally introduce a second meaning for an existing metric or source concept.
4. As a maintainer, I want Refresh to remain the only production write path, so that ledger mutations have one place to reason about and verify.
5. As a query consumer, I want every totals, sessions, requests, groups, period, detail, and meta result to come from the same normalized ledger, so that CLI, HTTP, WebUI, and watch cannot silently diverge.
6. As a maintainer, I want Query to stay read-only, so that adding a query view cannot unexpectedly discover files, advance cursors, or mutate the database.
7. As a CLI user, I want an unknown source to fail before refresh or rollup maintenance starts, so that a typo cannot perform unrelated database work.
8. As a CLI user, I want unknown formats, groups, periods, views, and commands to produce explicit errors, so that I never mistake a fallback output for a successful requested operation.
9. As a CLI user, I want an invalid or non-positive watch interval to be rejected safely, so that bad input cannot crash the process through ticker construction.
10. As an HTTP consumer, I want invalid query parameters to use the documented 400 error contract, so that clients can distinguish bad requests from server failures.
11. As a WebUI user, I want invalid rename names to follow one documented policy, so that sanitization and error messages do not disagree.
12. As a local server operator, I want rename request bodies to have a bounded size and explicit JSON validation, so that a malformed or excessive request cannot create avoidable resource pressure.
13. As a maintainer, I want malformed individual source records to be skipped with diagnostics, so that one bad record does not discard valid usage from the same source.
14. As a maintainer, I want unreadable or partially read files to be distinguishable from files with no billable usage, so that missing data can be investigated instead of looking like a legitimate zero.
15. As a maintainer, I want a file cursor to advance only after all relevant ledger writes and metadata writes commit, so that a failed refresh can be retried without permanent data loss.
16. As a maintainer, I want every ledger mutation error to be returned and rolled back, so that dedup rows, usage rows, session metadata, and cursors cannot become partially committed.
17. As a user, I want known unpriced Codex usage to remain visible as `unpriced`, so that “no price available” is not confused with zero cost.
18. As a user, I want pricing-table read failures to be distinguishable from a model with no configured price, so that cost results are not silently understated.
19. As an operator, I want diagnostics persistence failures to be observable, so that coverage warnings cannot disappear while the usage ledger appears healthy.
20. As a database maintainer, I want schema changes to be represented as ordered migrations, so that a new database and an existing database reach the same schema deterministically.
21. As a database maintainer, I want each migration to be transactional and versioned, so that a failed upgrade does not leave an ambiguous partially upgraded database.
22. As a database maintainer, I want migration tests for fresh, previous-version, and failed-upgrade databases, so that schema evolution is verified without relying on a developer’s local database.
23. As a query maintainer, I want the raw-ledger and daily-rollup inclusion rules to remain explicit, so that historical rollup data is neither omitted nor double-counted.
24. As a query consumer, I want views that cannot reconstruct session or cwd identity from rollups to remain raw-ledger-only, so that the system does not fabricate detail that the storage schema cannot support.
25. As a query consumer, I want default row ordering to be deterministic, so that pagination, repeated queries, exports, and WebUI refreshes do not move equal rows unpredictably.
26. As a contributor, I want Query implementation files organized by read, rollup, source view, combined view, and row-building responsibilities, so that a change to one concern does not require navigating the whole engine.
27. As a contributor, I want HTTP handlers, refresh state, watch behavior, and session rename logic to have clear private implementation locations, so that endpoint changes do not accidentally alter synchronization behavior.
28. As a contributor, I want Pi file revision, parsing, identity, deduplication, and transaction writing to have clear internal seams, so that changes to one incremental-sync rule can be reviewed independently.
29. As a maintainer, I want unused helpers and misleading names removed, so that the codebase does not advertise behavior or abstractions that no caller relies on.
30. As a contributor, I want to reuse the existing standard-library-based HTTP, JSON, SQL, and flag tooling, so that maintainability work does not add dependency upgrade or build-chain cost.
31. As a maintainer, I want canonical source fixtures to exercise Refresh through Query, so that tests protect externally observable statistics rather than implementation details.
32. As a maintainer, I want regression coverage for malformed records, unreadable files, transaction failure, cursor failure, retry, and idempotency, so that data-integrity guarantees remain enforceable.
33. As a maintainer, I want regression coverage for invalid CLI and HTTP inputs, so that protocol behavior stays aligned with the documented contract.
34. As a maintainer, I want regression coverage for database migration and read-only query behavior, so that storage safety is protected independently of source parsing tests.
35. As a maintainer, I want Pi, Codex, and All contract tests to preserve their source-specific capabilities and restrictions, so that a structural refactor cannot accidentally make Codex requests or Pi-only operations appear supported.
36. As a release operator, I want formatting and standard static checks to run automatically, so that trivial quality failures are found before review or release.
37. As a release operator, I want pull requests and the main development branch to run the relevant Go tests automatically, so that release tags are not the first point at which integration failures are discovered.
38. As a release operator, I want the root Go module to be tested without references to the migrated-out OpenCode project, so that this repository’s CI covers only artifacts it owns.
39. As a release operator, I want the binary version to have one source of truth, so that release tags, CLI version output, and release artifacts cannot drift.
40. As a release operator, I want cross-platform builds and help/version smoke checks to remain in the release pipeline, so that packaging failures are detected before users download artifacts.
41. As a new contributor, I want the active architecture and maintenance rules to be discoverable from the main project documentation, so that historical migration notes do not look like current implementation instructions.
42. As a maintainer, I want user-visible semantic changes to require an ADR or domain glossary update, so that future contributors can understand why a seemingly unusual rule exists.
43. As a maintainer, I want pure refactors to preserve canonical outputs, so that file organization improvements do not become accidental behavior changes.
44. As a project owner, I want the plan to preserve the zero-runtime-dependency single-binary product direction, so that maintainability work does not increase installation or operational complexity.

## Implementation Decisions

- 本 effort 只处理可维护性、可靠性和交付一致性，不改变既有统计口径。fork 去重、四载体门控、`totalTokens`、cache rate、input/cacheRead、cost 状态、时间双语义、Codex snapshot 处理和 OpenCode 产品边界继续遵循现有领域术语表与 ADR。

- 第一个必须完成的 work package 是仓库基线收口。OpenCode Analyzer 已正式迁出本仓库；root boundary test、root CI、README、CONTEXT 和 ADR 不得继续要求或引用该目录。独立项目的测试和发布责任由外部项目负责，本 effort 不修改外部项目。

- 保留 `refresh` 作为 source → ledger 的唯一写入编排，保留 `query` 作为唯一生产统计 seam。Pi 和 Codex 仍是两个独立 source adapter，不新增一个为了形式统一而存在的通用 source interface。

- 在 Refresh 和 Query 的入口统一校验 source、view 和 source-specific capability。CLI 的参数解析使用一个可返回 error 的运行接缝，`main` 只负责输出和退出；HTTP handler 只负责协议映射。所有非法输入必须在打开或修改 ledger 前失败。

- CLI 与 HTTP 的外部契约保持现有成功响应结构。非法 source、format、group、period、view、分页参数、排序参数、watch interval 和 rename payload 必须使用明确、稳定的错误类别；仅在确实需要修正文档与实现不一致时改变错误行为。

- source adapter 采用明确的错误分类：单条格式错误可以跳过但必须进入诊断；文件级读取或 revision 错误不得伪装为成功同步，也不得推进该文件 cursor；数据库 mutation、事务提交、schema migration 和关键诊断持久化失败必须返回错误；“没有配置价格”和“读取价格表失败”必须保持可区分。

- 保持每个 Pi 文件和每个 Codex rollout 的原子 ledger 写入。dedup、usage、session metadata、diagnostics 和 cursor 的提交顺序由真实 `sql.Tx` 保证；commit 失败或任一 mutation 失败时，重试必须能恢复且不产生重复账。

- 数据库 schema 采用显式、顺序、事务化的 migration 组织。最新 schema 的创建和旧版本升级使用同一套最终列、索引和约束定义；user version 只在对应 migration 成功后推进。现有 rollup contract 不因维护性重构而废弃。

- `usage_daily_rollups` 的查询规则继续按已有能力声明执行：totals、period 和可安全按 model 聚合的 groups 可以使用 rollup；需要 session、cwd、request identity 或 detail 的视图只使用 raw ledger。raw 与 rollup 的贡献范围必须明确互斥或按现有 contract 正确相加。

- Query Engine 的公共 Interface 不变，私有实现按职责组织为 ledger 读取、rollup、Pi 视图、Codex 视图、All 合并、行构建和分页等内部模块。拆分只改善 locality，不复制聚合算法，也不引入新的统计旁路。

- 所有 Query loader 必须提供显式排序。默认排序和用户指定排序都要有稳定 tie-breaker，保证相同 key 的结果可重复并适用于分页和导出。

- server 模块继续使用 Go 标准库 `net/http` 和内嵌单一 WebUI 源码。HTTP handler、watch、刷新状态、查询响应和 Pi 重命名可以拆到私有实现单元，但不引入前端构建链、Web framework 或第二份 HTML。

- Pi 同步模块和 Codex 同步模块可以抽出文件读取、revision、诊断和 ledger transaction 的私有辅助逻辑，但不得把 Pi/Codex 的不同 identity、dedup 和 usage 语义压缩成错误的共享实现。

- 删除已经确认没有调用方的死代码和误导性命名，包括伪单例入口、未使用的 watch 辅助、Codex 不支持视图的未调用 row builder、未使用的时间解析辅助和未读取的局部状态。删除前只检查当前仓库内部引用，不为假设中的外部调用保留 `internal` 死代码。

- 版本采用单一来源。构建配置与 CLI 版本输出不能各自维护独立常量；发布构建必须能够证明 tag、artifact 和 `--version` 一致。

- CI 使用 Go 标准工具和现有串行资源约束。自动执行格式检查、标准静态检查、相关 Go module 测试、发布构建和 help/version smoke check；不添加新的 lint 服务或第三方 CI 框架作为本 effort 的前置条件。

- 文档只在语义或仓库边界变化时更新领域术语表与 ADR。历史 migration/audit 文档保留其审计价值，但必须明确它们不是当前执行入口；不再新增一份与已有领域文档重复的架构说明。

## Testing Decisions

- 测试优先验证外部可观察行为：QueryResult、QueryMeta、HTTP 状态与 JSON error contract、CLI 返回错误/输出、ledger 行和 cursor 状态、migration 后 schema 与 retry 结果。不要把私有函数的调用顺序、文件拆分方式或 SQL 字符串排列当作测试目标。

- 主要测试 seam 是现有的 synthetic source fixture → Refresh → normalized ledger → Query。该 seam 覆盖最高层的真实产品路径，继续作为 Pi、Codex、All 的 canonical contract 入口；测试应优先复用现有 fixture 和 golden 断言，而不是建立第二套期望值。

- 存储安全使用现有 SQLite integration seam：创建 fresh database 或 previous-version fixture，经过打开/迁移/Refresh/Query，再检查 schema、数据、cursor 和回滚结果。迁移测试必须覆盖成功升级、重复升级、失败升级和 read-only Query。

- source adapter 的回归测试沿用现有的增量、半行、重写、zstd、压缩切换、fork、dedup、diagnostics、shared-database isolation、rollup 和 partial coverage prior art。新增测试只补维护计划涉及的缺口，不复制已经存在的场景。

- 错误测试使用可确定的失败注入或无效 schema/fixture，不能通过 sleep、随机竞态或“看起来像失败”的时间安排证明事务安全。至少验证：usage mutation 失败时 cursor 不前进；cursor mutation 失败时 usage/dedup 回滚；重试后结果恰好一次且重复 Refresh 幂等。

- Query contract 必须覆盖：raw 与 rollup 的正确合并、cwd/session/request 视图不伪造 rollup identity、invalid view/source 的明确错误、默认排序稳定、相同排序 key 的分页不漂移，以及 Pi/Codex/All 的能力限制。

- CLI 测试只通过命令运行接缝验证参数到结果的行为，覆盖未知 command、source、format、group、period、负数或零 interval、时间范围错误和成功的 table/JSON/CSV 路径。非法参数测试应确认不会创建、写入或维护 ledger。

- HTTP 测试继续使用标准库 `httptest`，通过真实 handler 验证 400/404/409/500/503 分类、source capability、参数解析、rename payload 限制、snapshot 保持和 query-only 行为。不要为每个 handler 的内部辅助函数建立独立测试套件。

- WebUI 仍以嵌入源码同步测试和 HTTP contract 测试为主。除非未来明确要求浏览器级交互测试，否则不引入 npm、浏览器测试框架或前端构建系统。

- 交付验证在 CI 或得到授权的本地环境执行 token-analyzer root module 测试，并保持项目约定的 `GOMAXPROCS=2`、`-p 1` 串行限制。OpenCode Analyzer 的测试不属于本仓库验证范围。当前 spec 生成过程不执行本机 build/test。

- 完成标准包括：现有 canonical 输出无变化；新增维护性回归覆盖通过；没有未解释的静默数据库 mutation error；migration 可从受支持版本重放；CI 在 pull request 和主分支提供反馈；发布版本与 CLI version 一致；最终仓库布局、测试和文档互相一致。

## Out of Scope

- 不恢复 TypeScript、Node.js、npm 分发或迁移期 parity oracle。
- 不改变 Pi、Codex、All 的统计数字、字段语义、时间归属、fork 去重、cost 状态、rollup 能力或 source capability，除非另行提出并接受新的 ADR。
- 不重新设计 normalized SQLite ledger，不迁移到 PostgreSQL、ORM 或其他持久化系统。
- 不创建通用 repository、factory、dependency injection container、event bus 或新的跨 source 抽象。
- 不把 Query 改回文件扫描、内存聚合或 Codex SessionFileData 回绕路径。
- 不拆分成第二份 WebUI，不引入前端依赖、前端构建系统或视觉重做。
- 不进行全量代码重写、性能优化专项、云端 telemetry、后台服务化或多用户部署支持。
- 不在本 effort 中自动处理当前工作区已有的无关删除、未跟踪 scratch 内容或其他用户修改；OpenCode Analyzer 迁出后的 root 清理仅限本仓库残留引用。
- 不为历史上明确不兼容的旧产品形态增加额外兼容层；只为已接受的 schema migration 和当前支持的 ledger contract 提供可验证升级路径。

## Further Notes

- 这是一个维护性收口 spec，不是新功能 spec。最小落地顺序为：仓库基线 → 输入与错误策略 → schema migration → 私有实现拆分与死代码清理 → CI/发布收口。
- `CONTEXT.md`、ADR 和 canonical fixture 是长期行为依据；历史 audit/remediation 文档用于理解背景，不应成为新的实现入口。
- 当前项目规模仍适合标准库、显式 SQL 和少量私有辅助函数。维护性目标是增加 locality 和可观察性，而不是增加架构层级。
- OpenCode Analyzer 已正式迁出；其 standalone module 的维护计划应在独立仓库建立。本 spec 只负责 root token-analyzer 对迁出结果的契约收口。
