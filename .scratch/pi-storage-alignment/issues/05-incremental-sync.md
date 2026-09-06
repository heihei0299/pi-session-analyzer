# 05: 指纹增量同步（revision 4096B + 行游标）

**What to build:** `piFileRevision(path)` + `syncPiUsage(db, files, {full?})` 指纹增量同步，按 `session_usage_pi.rs:pi_file_revision` 的 `tail4096 SHA256 + complete` 规则，`session_log_sync` 的 `revision` 编码与 `last_line_offset` 行游标，`Tx` 内原子 `dedup查+写 + proxy_request_logs INSERT OR IGNORE + session_log_sync UPDATE`。

**Blocked by:** 04: 双账本去重, 02: Pi 会话发现

**Status:** ready-for-agent

- [ ] `pi_file_revision()` 读末 4096B（`pi-session-tail-v1` 域标签）SHA256 + `complete = 末字节=='\n'`，`revision` 编码 `modified_nanos|file_size|tail_fingerprint|complete` 于 `last_synced_at`
- [ ] `sync_pi_usage`：`load_sync_cursors()` → 追加校验 `tail(oldEOF)==expected` 则 `seek(oldSize)` 续读，否则全量重扫（账本防双算）
- [ ] `read_until('\n')` 半行不推进 `committed_offset`，`complete=false` 下轮补全后重验指纹
- [ ] `Tx` 内原子 `dedup查+写 + proxy_request_logs INSERT OR IGNORE + session_log_sync UPDATE`，`last_line_offset` 行游标保证半行不推进
- [ ] `CLI token-analyzer sync [--full] [--db]`（`--full` 忽略 `session_log_sync` 全量重扫），`--prune [--days 30]` 入口占位
