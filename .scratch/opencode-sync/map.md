# OpenCode 数据同步与 WebUI 对账 — Map

**Map id**: `opencode-sync` — see `docs/agents/issue-tracker.md` for tracker conventions.

## Destination

在 `pi-session-anylize` 中实现 OpenCode（`opencode.ai`）云端后台数据的逆向拉取、本地持久化与 WebUI 对账可视化面板。提供：
1. 独立的 SolidStart RPC / Seroval 通信客户端（`OpenCodeClient`）；
2. 本地分层存储与增量同步引擎（`data/opencode/` 下的 `costs.json`、`history.json` 及 `history.csv`）；
3. CLI 子命令（`opencode sync` / `opencode export`）；
4. HTTP API 端点（`/api/opencode/*`）；
5. WebUI「OpenCode 对账」专属面板（含月度成本柱状图、历史明细表、对账看板与一键同步）。

终点 = **开发者可通过 CLI 或 Web 界面一键同步 OpenCode 官方账单与历史明细，在 WebUI 仪表盘直观比对本地 Pi 会话与云端扣费**。

## Notes

- **Domain**: OpenCode 逆向 API / SolidStart RPC / Seroval 协议 / 数据持久化与对账
- **技能**: `grill-to-spec`（已完成）、`to-tickets`（已完成）、`tdd-implement`（后续实现）
- **已定基线**:
  - ① 凭证管理：环境变量 / `.env`（`OPENCODE_AUTH`, `OPENCODE_WORKSPACE_ID`）+ CLI 参数临时覆盖；
  - ② 存储格式：`data/opencode/` 原始 JSON 分层持久化 + CSV 导出，自动增量去重；
  - ③ WebUI 形态：独立 Tab「OpenCode 对账」，首屏本地缓存秒开 + 一键后台同步；
  - ④ 对账口径：按月/按日对比本地 Pi 消耗与官方扣费，标注差额。

## Tickets

1. [01: OpenCode SolidStart RPC 通讯与端点客户端](issues/01-opencode-rpc-client.md) — `ready-for-agent`
2. [02: OpenCode 本地数据分层持久化与增量同步仓](issues/02-opencode-storage-and-sync.md) — `ready-for-agent`
3. [03: OpenCode CLI 命令行工具（sync / export）](issues/03-opencode-cli-commands.md) — `ready-for-agent`
4. [04: OpenCode HTTP API 服务端端点](issues/04-opencode-http-api.md) — `ready-for-agent`
5. [05: WebUI「OpenCode 对账」面板与可视化图表](issues/05-webui-opencode-tab.md) — `ready-for-agent`

## Decisions so far

- **接口协议逆向确认（2026-09-01）**：OpenCode 后台基于 SolidStart `/_server` 端点与 Seroval 序列化；使用历史明细端点 ID `bfd684bfc2e4eed05cd0b518f5e4eafd3f3376e3938abb9e536e7c03df831e5c`，月度成本端点 ID `15702f3a12ff8bff357f8c2aa154a17e65b746d5f6b96adc9002c86ee0c15205`，工作区列表端点 ID `def39973159c7f0483d8793a822b8dbb10d067e12c65455fcb4608459ba0234f`。
- **费用单位换算**：月度聚合成本按 $10^{-8}$ 微单位进行标准美元格式化（`$X.XXXX`）。
- **架构 Seam 解耦**：OpenCode 模块作为外部对比基准，与本地 `SessionData` 会话数据仓完全解耦，不污染本地原始会话统计口径。
