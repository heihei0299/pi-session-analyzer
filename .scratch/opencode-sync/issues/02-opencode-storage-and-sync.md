# 02: OpenCode 本地数据分层持久化与增量同步仓

**What to build:** 构建 OpenCode 本地持久化与增量数据同步模块（`OpenCodeStorage`），负责将从远端抓取的数据持久化到 `data/opencode/` 目录下（`costs.json`、`history.json` 及导出 CSV）。具备按记录唯一 `id` 与 `timeCreated` 去重合并的能力，维护 `lastSyncedTime` 游标以支持快速增量同步，并提供数据查询过滤接口。

**Blocked by:** 01: OpenCode SolidStart RPC 通讯与端点客户端

**Status:** resolved

- [x] 实现本地数据目录（默认 `data/opencode/`）的创建与文件结构维护（`costs.json`, `history.json`）
- [x] 实现 `costs.json` 按 `year-month` 存储与查询逻辑
- [x] 实现 `history.json` 记录按 `id` 去重合并与按 `timeCreated` 逆序排序逻辑
- [x] 实现 `exportCsv(filePath?)` 方法，自动将使用历史记录导出为格式清晰的标准 CSV 文件
- [x] 实现增量同步驱动器（`sync()`），从第 0 页开始自动遍历分页，直到遇到已存在记录或到达末尾，更新 `lastSyncedTime` 并返回同步统计（新增条数、耗时）
- [x] 编写单元测试验证去重、增量截断、CSV 生成与文件持久化

## 实施总结
- 提交：`56b32ca` — `feat(opencode-sync): OpenCode local storage and sync (#02)`
- 实现的 seams：
  - T1 ensureDataDir + 初始文件结构 — `src/opencode/storage.ts:ensureDataDir`
  - T2 costs 按 year-month 存储与查询：saveCosts/getCosts/listCosts — `src/opencode/storage.ts:saveCosts/getCosts/listCosts`
  - T3 history 去重合并与排序：mergeHistory — `src/opencode/storage.ts:mergeHistory`
  - T4 exportCsv(filePath?) 标准 CSV — `src/opencode/storage.ts:exportCsv`
  - T5 sync 增量同步驱动器 — `src/opencode/storage.ts:sync`
  - T6 持久化往返与查询过滤：loadHistory/getHistory/clear — `src/opencode/storage.ts:loadHistory/getHistory/clear`
- 验收标准：
  - [x] 实现本地数据目录（默认 `data/opencode/`）的创建与文件结构维护 — `ensureDataDir` + `costs.json`/`history.json` + 测试 T1 幂等
  - [x] 实现 `costs.json` 按 `year-month` 存储与查询逻辑 — `saveCosts`/`getCosts`/`listCosts`，key `${year}-${MM}`，测试 T2 往返/覆盖/排序/跨年
  - [x] 实现 `history.json` 记录按 `id` 去重合并与按 `timeCreated` 逆序排序逻辑 — 去重保留最新、逆序、lastSyncedTime，测试 T3
  - [x] 实现 `exportCsv(filePath?)` 自动导出标准 CSV — 表头含 17 列、RFC4180 转义、默认 `history.csv`，测试 T4
  - [x] 实现增量同步驱动器 `sync()` — page0起遍历、id/时间截断、full/limit、返回 {added,pages,elapsedMs,lastSyncedTime}、支持多重载，测试 T5
  - [x] 编写单元测试验证去重、增量截断、CSV 生成与文件持久化 — `test/02-opencode-storage.test.ts` 34项全绿
- 测试结果：相关测试 34 项全绿（`TZ=Asia/Shanghai node --test test/02-opencode-storage.test.ts` pass 34 fail 0）；与 01 合跑 59 项全绿；typecheck 通过
- typecheck：通过（`npm run typecheck` 无输出）
- 文档对齐：CONTEXT.md 已含 OpenCode 术语（上游 #01 已补）；README 暂无需更新（内部仓未暴露 CLI/webui，待 03/04 再对齐）；spec `costs.json: {"2026-08": result}` 与 `history.json: {records,lastSyncedTime,updatedAt}` 已与实现一致，CSV 自动生成
- 遗留 / 后续建议：`sync` 单进程无锁（ponytail 注释）；并发场景需文件锁；CSV enrichment 字段为 JSON.stringify，已正确转义；`getAllCosts()` 额外暴露供 04 使用，非 spec 必需但零成本

