# pi-storage-alignment — Issue Map（直切 cc-switch）

- **01-database-foundation** — SQLite 基座（`token-analyzer.db` 5表+WAL，`SCHEMA_VERSION=1`，`engines>=24`/`modernc.org/sqlite`）— `None` — ready-for-agent
- **02-pi-session-discovery** — 双布局发现（`PI_CODING_AGENT_SESSION_DIR` > defaults > `~/.pi/agent/sessions`，Flat vs ProjectDirectories）— `01` — ready-for-agent
- **03-four-carrier-parsing** — 四载体解析与门控（`has_billable||has_cost||failed`，512B 截断）— `01` — ready-for-agent
- **04-dedup-ledger** — 双账本去重（`request_id/semantic_id` 持久化，`hash_field/hash_json`）— `03` — ready-for-agent
- **05-incremental-sync** — 指纹增量（`tail4096 + complete` revision，`Tx` 原子）— `04, 02` — ready-for-agent
- **06-cost-calculation** — 费用回算（`reported ?? CostCalculator`，`FRESH`）— `03` — ready-for-agent
- **07-db-aggregation** — DB 聚合与剪枝（`totals/groups/period/sessions/requests` + `rollup_and_prune(30)`）— `05, 06, 04` — ready-for-agent
- **08-api-webui-alignment** — API 与总览对齐（`GET /api/db/meta`，8卡/tape/分组表四载体）— `07` — ready-for-agent

分层：L1 [01] → L2 [02, 03] → L3 [04, 06] → L4 [05] → L5 [07] → L6 [08]
