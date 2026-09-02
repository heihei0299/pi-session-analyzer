# OpenCode 数据拉取、本地持久化与 WebUI 对账面板规范

`ready-for-agent`

## Problem Statement

作为 `pi-session-anylize` 的使用者，用户在本地使用 Pi 生成大量会话并由本地工具进行 Token 统计，但在云端使用的是 OpenCode（`opencode.ai`）服务。当前存在以下痛点：
1. **云端后台数据孤立**：OpenCode 后台（`https://opencode.ai/workspace/<wrk_id>/go`）记录了真实发生的模型使用量（按模型细分成本）和近期每笔请求的使用历史（输入、输出、缓存命中、推理、真实计费、会话 ID），但缺乏自动化工具将其导出到本地进行归档与深入分析。
2. **缺乏直观对账手段**：本地 Pi 会话基于本地 `*.jsonl` 会话文件统计，无法直接与 OpenCode 官方账单和请求明细进行对照，导致用户难以核实本地预估费用与云端实际扣费是否吻合，也无法看清因 Pi 内部任务（如 context compaction 结构性差额）或非 Pi 外部调用产生的使用差异。
3. **接口为内部 SolidStart RPC**：OpenCode 控制台并非标准 RESTful API，而是基于 SolidStart 的 `/_server` RPC 端点及 Seroval 序列化协议，外部常规脚本无法直接调用。

## Solution

构建一套高内聚、模块化的 OpenCode 数据同步与对账系统：
1. **逆向 RPC 客户端（`OpenCodeClient`）**：实现 SolidStart RPC 与 Seroval 序列化/反序列化通信，支持凭证（Auth Cookie / Workspace ID）注入与自动发现，提供对 `getCosts`（月度模型成本聚合）与 `getUsageInfo`（分页请求历史明细）的稳定调用。
2. **本地分层持久化与增量同步（`OpenCodeStorage`）**：将拉取的数据保存至 `data/opencode/` 目录下（`costs.json`、`history.json` 及导出的 `history.csv`），基于请求记录的唯一 ID 与时间戳实现增量去重同步。
3. **CLI 命令行工具**：在现有 CLI 增加 `opencode sync` 与 `opencode export` 命令，支持指定凭证、工作区、拉取限制及全量/增量模式。
4. **WebUI 专属「OpenCode 对账」面板**：
   - 增加独立 Tab「OpenCode 对账」，支持毫秒级读取本地持久化数据；
   - 提供「🔄 立即同步」按钮，可一键在后台调用 `/api/opencode/sync` 更新数据；
   - 呈现月度按模型细分成本的堆叠柱状图（复刻官方视觉）；
   - 提供分页、可筛选的使用历史明细表（含输入、缓存、输出、推理、费用、会话 ID）；
   - 提供本地 Pi 统计 vs OpenCode 官方扣费的按月/按日对账看板与差额率提示。

## User Stories

1. As a developer using OpenCode, I want to fetch my workspace's monthly cost breakdown from opencode.ai via CLI, so that I can keep track of historical spending without manually logging into the website.
2. As a developer, I want to fetch paginated usage history logs (tokens, reasoning, cache hits, cost, session ID) from opencode.ai, so that I have a local audit trail of every API call.
3. As a developer, I want the client to automatically handle Seroval stream serialization/deserialization, so that communication with OpenCode's SolidStart server functions is completely transparent.
4. As a developer, I want to configure my OpenCode credentials via environment variables (`OPENCODE_AUTH`, `OPENCODE_WORKSPACE_ID`) or `.env` file, so that I do not need to pass sensitive cookies in plaintext CLI commands every time.
5. As a developer, I want CLI flags (`--auth`, `--workspace`) to override environment variables, so that I can switch between different accounts or workspaces on demand.
6. As a developer, I want the client to auto-discover my default workspace ID if none is explicitly provided, so that I don't need to manually look up long workspace hashes.
7. As a developer, I want usage history to be saved locally in structured JSON and CSV formats under `data/opencode/`, so that I can easily inspect the data in spreadsheet software or external analysis scripts.
8. As a developer, I want incremental sync capability based on `lastSyncedTime` / record IDs, so that repeated sync operations are fast and do not duplicate historical records.
9. As a developer, I want to view a dedicated "OpenCode 对账" tab in the WebUI, so that I can review cloud billing and usage data within my existing token analyzer dashboard.
10. As a developer, I want the WebUI OpenCode tab to load from local cache instantly on page load, so that there is zero network latency when browsing.
11. As a developer, I want a "🔄 立即同步" button on the WebUI OpenCode panel, so that I can trigger an on-demand cloud sync and see live updates without leaving the browser.
12. As a developer, I want to view a monthly cost stacked bar chart categorized by AI models in the WebUI, so that I can visually compare model cost distributions across days and months.
13. As a developer, I want to switch months in the WebUI chart view, so that I can inspect past months' cost trends.
14. As a developer, I want a paginated table of usage history records in the WebUI with sorting and filtering by model and session ID, so that I can investigate anomalous requests.
15. As a developer, I want an Audit Bar comparing local Pi session token totals vs OpenCode official billing totals for a given month, so that I can quickly verify billing accuracy and identify unmetered internal requests (such as context compaction).
16. As a developer, I want descriptive error messages when credentials expire or when network calls fail, so that I know exactly when my cookie needs to be refreshed.

## Implementation Decisions

### 1. Module Structure & Seams
- **`OpenCodeClient` (Reverse RPC Seam)**:
  - Communicates directly with `https://opencode.ai/_server`.
  - Implements Seroval payload encoding (`{ t: { t: 9, i: 0, l: N, a: [...] }, f: 31, m: [] }`) and stream chunk decoder for chunked evaluated responses.
  - Exposes clean async methods:
    - `getCosts(workspaceId: string, year: number, month: number, tzOffset?: number): Promise<MonthlyCostsResult>`
    - `getUsageInfo(workspaceId: string, page: number): Promise<UsageRecord[]>`
    - `getWorkspaces(): Promise<WorkspaceInfo[]>`
- **`OpenCodeStorage` (Persistence Seam)**:
  - Manages storage in `data/opencode/`.
  - Maintains `data/opencode/costs.json` (keyed by `year-month`).
  - Maintains `data/opencode/history.json` (ordered list of `UsageRecord` objects, deduplicated by `id`).
  - Automatically generates/updates `data/opencode/history.csv` on sync.
  - Implements incremental sync loop: fetches page `0, 1, 2...` until reaching records with `timeCreated <= lastSyncedTime` or empty page.
- **`OpenCodeAudit` (Reconciliation Engine)**:
  - Aggregates local session messages from `SessionData` for a given month/range.
  - Compares local token sum & estimated cost against OpenCode's recorded `totalCost` and `inputTokens + cacheReadTokens + outputTokens`.
  - Computes match percentage and structural discrepancy summary (e.g. non-session requests).
- **HTTP API Extension (`api.ts` & `server.ts`)**:
  - `GET /api/opencode/costs?year=YYYY&month=M`: Returns cached or queried monthly costs breakdown.
  - `GET /api/opencode/history?page=N&size=M&model=X`: Returns paginated OpenCode history records.
  - `GET /api/opencode/audit?year=YYYY&month=M`: Returns reconciliation metrics between local sessions and OpenCode bill.
  - `POST /api/opencode/sync`: Triggers background sync and returns sync statistics (new records count, elapsed time).
- **WebUI Tab (`webui.html`)**:
  - Adds tab button `OpenCode 对账` (`data-tab="opencode"`).
  - Renders reconciliation summary metrics, month picker, Canvas-based stacked bar chart (matching the style of overview charts), and paginated history table.
  - Includes sync status badge with last sync timestamp and "立即同步" button.
- **CLI Command (`cli.ts`)**:
  - Subcommands: `opencode sync` (supports flags `--auth`, `--workspace`, `--full`, `--limit`) and `opencode export` (`--format=json|csv`).

### 2. Protocol & Data Contract Types
```typescript
interface OpenCodeUsageRecord {
  id: string; // "usg_..."
  workspaceID: string; // "wrk_..."
  timeCreated: string; // ISO string
  timeUpdated: string;
  timeDeleted: string | null;
  model: string; // e.g. "x-preview-f-free", "deepseek-v4-flash"
  provider: string; // "inf.oa-compat"
  inputTokens: number;
  outputTokens: number;
  reasoningTokens: number | null;
  cacheReadTokens: number | null;
  cacheWrite5mTokens: number | null;
  cacheWrite1hTokens: number | null;
  cost: number; // micro-unit / floating dollars
  keyID: string;
  sessionID: string | null;
  enrichment: unknown | null;
}

interface OpenCodeMonthlyCostItem {
  date: string | null;
  model: string;
  totalCost: number; // scaled integer (e.g. 891912946 -> $8.9191)
  keyId: string;
  plan: string; // "lite" | "standard"
}

interface OpenCodeCostsResult {
  usage: OpenCodeMonthlyCostItem[];
  keys: Array<{ id: string; name: string }>;
}
```

### 3. Cost Scaling & Unit Normalization
- OpenCode API `totalCost` in monthly breakdown is expressed in scaled micro-currency units (e.g. `891912946` represents `$8.91912946`, scaling factor $10^{-8}$).
- Formatting in UI and tables should normalize this to standard USD currency display (`$X.XXXX`).

## Testing Decisions

- **Black-box Unit & Integration Tests**:
  - Test `Seroval` serializer and deserializer against mock response streams without making real network calls.
  - Test `OpenCodeStorage` incremental merge and deduplication with simulated overlapping pages.
  - Test API handlers (`/api/opencode/*`) with mock storage instances.
  - Test CLI parameter parsing and fallback to `.env`.
- **E2E / Live Verification**:
  - Use sandbox bypass test script with live credentials to verify end-to-end sync, file creation in `data/opencode/`, and WebUI API endpoint responses.

## Out of Scope

- Writing modifications back to OpenCode (e.g. creating API keys or altering subscription plans).
- Intercepting active proxy requests in real-time (handled by pi-switch gateway, not this exporter).
- Altering the core local `SessionData` schema or modifying raw `*.jsonl` session files.

## Further Notes

- OpenCode SolidStart RPC endpoint IDs are deterministic for a given frontend build (`bfd684bfc2e4eed05cd0b518f5e4eafd3f3376e3938abb9e536e7c03df831e5c` for usage history, `15702f3a12ff8bff357f8c2aa154a17e65b746d5f6b96adc9002c86ee0c15205` for monthly costs, `def39973159c7f0483d8793a822b8dbb10d067e12c65455fcb4608459ba0234f` for workspaces).
- If OpenCode updates its build and changes these IDs, `OpenCodeClient` can also support dynamic chunk discovery to extract the latest hash IDs automatically.
