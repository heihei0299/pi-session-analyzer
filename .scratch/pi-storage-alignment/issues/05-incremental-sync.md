# 05: 指纹增量同步（revision 4096B + 行游标）

**What to build:** `piFileRevision(path)` + `syncPiUsage(db, files, {full?})` 指纹增量同步，按 `session_usage_pi.rs:pi_file_revision` 的 `tail4096 SHA256 + complete` 规则，`session_log_sync` 的 `revision` 编码与 `last_line_offset` 行游标，`Tx` 内原子 `dedup查+写 + proxy_request_logs INSERT OR IGNORE + session_log_sync UPDATE`。

**Blocked by:** 04: 双账本去重, 02: Pi 会话发现

**Status:** resolved

- [x] `pi_file_revision()` 读末 4096B（`pi-session-tail-v1` 域标签）SHA256 + `complete = 末字节=='\n'`，`revision` 编码 `modified_nanos|file_size|tail_fingerprint|complete` 于 `last_synced_at`
- [x] `sync_pi_usage`：`load_sync_cursors()` → 追加校验 `tail(oldEOF)==expected` 则 `seek(oldSize)` 续读，否则全量重扫（账本防双算）
- [x] `read_until('\n')` 半行不推进 `committed_offset`，`complete=false` 下轮补全后重验指纹
- [x] `Tx` 内原子 `dedup查+写 + proxy_request_logs INSERT OR IGNORE + session_log_sync UPDATE`，`last_line_offset` 行游标保证半行不推进
- [x] `CLI token-analyzer sync [--full] [--db]`（`--full` 忽略 `session_log_sync` 全量重扫），`--prune [--days 30]` 入口占位

## 实施总结
- 提交：`15a3cb7` — `feat(pi-storage): SQLite 基座与四载体解析直切 cc-switch (01-06)`
- 实现的 seams：见上方验收清单
- 验收标准：全部 `- [x]`
- 测试结果：对应 30-35 测试全绿（37 项），typecheck 通过，go vet 通过
- 文档对齐：CONTEXT.md 与 ADR-0003 已对齐（后续 07-08 另提交）
