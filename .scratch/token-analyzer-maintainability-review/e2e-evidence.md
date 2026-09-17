# 13: 隔离环境与限并发下的端到端验证 — 证据

- 日期：2026-09-18
- 被测版本：`token-analyzer dev`（`go build` 本机当前平台二进制）
- 结论：ticket 13 覆盖的 CLI / HTTP / root 隔离 / rename / watch 路径全部通过；未发现需要修复的缺陷，本轮无修复 commit。
- 原始日志：保留在临时根 `/tmp/ta-e2e.gMpdjH/e2e/evidence/`（`cli-matrix.log`、`cli-invalid.log`、`http-happy.log`、`http-errors.log`、`http-root-isolation.log`、`cli-root-isolation.log`、`legacy-no-binding.log`、`watch.log`、`watch-codex-isolation.log`、`serve-a.log`、`serve-b.log`、`ledger.sha256.before`、`root-b.sha256.before`）。

## 隔离与并发设置

```
$ ROOT=$(mktemp -d /tmp/ta-e2e.XXXXXX)      # /tmp/ta-e2e.gMpdjH
$ HOME=$ROOT/e2e/home
$ TOKEN_ANALYZER_DB=$ROOT/e2e/ledger.db
$ PI_CODING_AGENT_SESSION_DIR=$ROOT/e2e/pi-sessions
$ CODEX_HOME=$ROOT/e2e/codex
$ GOMAXPROCS=2 go build -o $ROOT/e2e/bin/token-analyzer ./cmd/token-analyzer   # exit=0（25.6s）
```

- Pi 数据：`testdata/canonical/pi/{project,tasks}/*.jsonl` 复制到临时 `pi-sessions/`（flat layout，由 env 指定），另合成 `OldName_123e4567-e89b-42d3-a456-426614174000.jsonl`（mtime = 10 分钟前，用于 rename）。
- Codex 数据：临时 `CODEX_HOME` 内合成 rollout（`sessions/2026/09/17/rollout-2026-09-17T10-00-00-*.jsonl`，1 session / 15 tokens）。
- 并发：所有 Go 命令 `GOMAXPROCS=2`；步骤串行；同一时刻只有一个被测进程（`serve` 逐个启动、测试后关闭）；curl 顺序执行；无压力/负载测试。
- `serve` 只监听 `127.0.0.1`，端口 51837 / 51838（高位、预先确认空闲）；测试结束 `ss -ltn` 确认两个端口均未监听、无残留进程。

隔离证明（全部写入均在临时根内）：

```
$ fd -t f --max-depth 3 $ROOT | sed "s#$ROOT#<ROOT>#" | sort
<ROOT>/e2e/bin/token-analyzer
<ROOT>/e2e/env.sh
<ROOT>/e2e/evidence/*.log|*.pid|*.sha256   （见上）
<ROOT>/e2e/help.txt
<ROOT>/e2e/ledger.db
<ROOT>/e2e/pi-sessions/{fork_fork,main_main,nested_nested,task_task,OldName_…}.jsonl
<ROOT>/e2e/pi-root-b/project/Bsession_…jsonl
<ROOT>/e2e/t1.json

$ stat -c '%n mtime=%Y' /home/shial/.cache/token-analyzer
/home/shial/.cache/token-analyzer mtime=1789679691   # 与测试前记录值一致，未变化
$ stat -c '%n mtime=%Y' /home/shial/.pi
/home/shial/.pi mtime=1788387116                     # 未变化
$ stat -c '%n mtime=%Y' /home/shial/.cc-switch
/home/shial/.cc-switch mtime=1789668956              # 未变化
```

说明：为记录退出码/响应体曾使用 `/tmp/ta-*` 临时重定向文件与 `/tmp/ta-e2e-root` 路径指针，均已删除（内容已归入 `<ROOT>/e2e/evidence/`）。真实 `~/.pi`、`~/.cache/token-analyzer`、`~/.cc-switch` 未被读取内容、创建或修改。

## 1. `--version` / `--help`

| 命令 | 退出码 | 关键输出 |
| --- | --- | --- |
| `<bin> --version` | 0 | `token-analyzer dev` |
| `<bin> --help` | 0 | 用法/选项四段（`totals|sessions|requests`、`serve`、选项列表） |

## 2. CLI 正常路径矩阵（`evidence/cli-matrix.log`，26/26 exit=0）

- `totals|sessions|requests --source pi --format table|json|csv`（9 条）全部 exit=0；table/csv/json 结构与数据一致，例：

```
请求数   输入  输出  缓存读  缓存写  推理  总 token  花费      缓存率
11      52    28    9      4      5    89        $0.4000  14.8%
```

- `--by model|cwd|model,cwd`、`--period day|week|month` 全部 exit=0。
- 过滤 `--model m1`、`--cwd /workspace/e2e`、`--since 2026-09-17 --until 2026-09-18` 全部 exit=0。
- `--source codex`（合成 rollout）：totals/sessions/groups/period exit=0，totals = `1 请求 / 15 tokens / unpriced`。
- `--source all`：totals/sessions/groups/period exit=0，totals = `12 请求 / 104 tokens / $0.4000*`（含 unpriced 源标注）。

## 3. CLI 非法输入（`evidence/cli-invalid.log`，9/9 exit=2）

命令均带 `TOKEN_ANALYZER_DB=$ROOT/e2e/invalid.db`：

| 用例 | 退出码 |
| --- | --- |
| `--source bogus` | 2 |
| `requests --source codex` | 2 |
| `requests --source all` | 2 |
| `--by bogus` | 2 |
| `--period year` | 2 |
| `--format xml` | 2 |
| `--by model --period day` | 2 |
| `--watch --interval 0` | 2 |
| `sessions --by model` | 2 |

- `invalid-db-exists=NO`：非法输入未创建 ledger。
- 主 ledger sha256 前后一致（`ledger-unchanged=YES`），未维护/推进游标。

## 4. HTTP 正常路径（`serve --source pi --port 51837`，`evidence/http-happy.log`）

顺序请求（单进程、串行）：`/api/totals`、`/api/sessions`、`/api/requests`、`/api/groups?by=model`、`/api/period?period=day`、`/api/meta`、`/api/db/meta` → 全部 `200`。

```
$ curl -s .../api/db/meta | python3 -m json.tool
{
    "dbPath": "/tmp/ta-e2e.gMpdjH/e2e/ledger.db",   # 临时 ledger
    "lastRefreshAt": 1789685121,
    "lastRefreshError": null,
    "lastSyncAt": 1789685089,
    "rollupWatermark": null,
    "schemaVersion": 3
}
$ /api/meta?source=pi → dir=/tmp/ta-e2e.gMpdjH/e2e/pi-sessions, sessionCount=5, sources=['pi','codex']
```

## 5. HTTP 错误契约（`evidence/http-errors.log`）

| 请求 | 状态码 | 关键 body |
| --- | --- | --- |
| `/api/totals?source=bogus` | 400 | 非法 source |
| `/api/sessions?source=pi&page=1`（缺 size） | 400 | page/size 必须同时省略或为正 |
| `/api/sessions?source=pi&page=9223372036854775807&size=2` | 200 | 空页（`rows:[]`, `total:5`），无 panic |
| `/api/sessions?source=pi&page=1&size=9223372036854775807` | 200 | 返回 rows（全部会话），无 panic |
| `/api/sessions?source=pi&sortKey=bogus` | 400 | unknown sort key |
| `/api/sessions?source=pi&sortKey=requests&sortDir=up` | 400 | sortDir 非法 |
| `/api/requests?source=codex` / `source=all` | 400 | 明确 unsupported |
| `/api/sessions/detail?source=codex` | 400 | Pi 专属能力 unsupported |
| `/api/sessions/detail?source=pi&sessionId=does-not-exist` | 404 | 会话不存在 |
| `POST /api/sessions/rename?source=codex` | 400 | codex 不支持重命名 |
| 超大 page/size 之后 `/api/totals?source=pi` | 200 | 服务未崩溃 |

## 6. rename（`evidence/http-errors.log`）

- 成功：`POST /api/sessions/rename {"sessionId":"123e4567-…","name":"E2ENewName"}` → `200 {"ok":true,"fileName":"E2ENewName_123e4567-….jsonl"}`；磁盘上旧文件名消失、新文件名出现；随后同 sessionId `GET /api/sessions/detail` → `200`。
- 失败不伪装成功：
  - 不存在 sessionId → `404 会话不存在`，目录未变化；
  - 名称清洗后为空 → `400 显示名非法`；
  - 活跃文件（mtime=now）→ `409 会话活跃中，稍后再试`，文件名保持 `Active_aaaaaaaa-….jsonl`。

## 7. root 隔离（`evidence/http-root-isolation.log`、`evidence/cli-root-isolation.log`）

- root A（`pi-sessions`）已建立 binding；改用 root B（`pi-root-b`，内含 1 个文件）启动 `serve --port 51838`（同一临时 ledger）：
  - `/api/totals?source=pi` → `409 source root mismatch: bound "…/pi-sessions", requested "…/pi-root-b"`（sessions/detail 同 409，rename 同 409）；
  - root B 文件 sha256 前后一致（`b2053a67…` → 一致），目录中无新文件。
- CLI `PI_CODING_AGENT_SESSION_DIR=…/pi-root-b <bin> totals --source pi` → `exit=1` + `同步失败: source root mismatch …`；B 文件未变。
- 删除 binding 但保留 Pi 历史（`sqlite3` 删除 `source_root_bindings` 中唯一行）后：
  - `<bin> totals --source all` → `exit=1`，`同步失败: source root binding required: Pi history has no root binding…`；
  - `<bin> totals --source pi` → `exit=1`，同错误；
  - `pi_sessions=6`、`pi_session rows=12` 前后不变，`bindings=0`（未自动认领）。

## 8. watch 冒烟（`evidence/watch.log`、`evidence/watch-codex-isolation.log`）

```
$ timeout 8 <bin> totals --source pi --watch --interval 500
开始监控数据源: pi (轮询间隔: 500ms)...
[06:46:47] 请求数: 12 | 总 Token: 91 | 花费: $0.4000
# exit=124（timeout 终止，符合预期）

$ timeout 8 <bin> totals --source codex --watch --interval 500
开始监控数据源: codex (轮询间隔: 500ms)...
[06:46:55] 请求数: 1 | 总 Token: 15 | 花费: $0.0000
# exit=124

$ timeout 5 <bin> totals --source codex --watch --interval 500
# Pi 游标集合前后一致：7|4861 → 7|4861（codex watch 未触碰 Pi 数据）
```

watch 仅刷新配置的 source，totals 与同一快照的普通 Query 同源，无独立统计路径。

## 未执行项

- `make release`、交叉编译、打包、安装、发布、压力/负载测试：本轮未授权，未执行。
- 临时根 `/tmp/ta-e2e.gMpdjH` 保留至 review 结束，可整体删除。
