# 01 — 严格日期校验

**What to build:** CLI 与 webui API 对非法日历日期（如 2026-02-30、2026-07-32、2026-13-99）显式拒绝，不再静默归一化为相邻日期。CLI 传 `--since/--until` 非法日期时命令报错退出；API 携带非法 since/until 时返回 400 统一错误体。合法日期（含闰年 2024-02-29、月末 2026-08-31）行为不变，统计结果与修复前一致。

**Blocked by:** None — can start immediately

**Status:** resolved

- [x] 纯日期参数回读比对：与输入不一致（JS Date 溢出归一化）即拒绝
- [x] CLI `--since 2026-02-30` 退出码非 0 且 stderr 有错误信息
- [x] API `since=2026-07-32` / `until=2026-13-99` → 400，错误体含「无效 since/until」
- [x] 闰年与月末合法日期解析不变
- [x] 既有时间筛选测试全量通过（无回归）

## Implementation summary

- `src/time-range.ts` 的 `parseTimestamp` 纯日期分支已通过 `new Date(y, mo-1, d)` 回读年月日做严格公历校验；非法日期抛 `无效时间`。
- API 的 `dbFilterFromParams` / `filterFromParams` 继续调用该解析器并映射为 400 统一错误体。
- npm/TS CLI 原读路径直接走 `db-aggregation.parseSinceUntil`，绕过严格解析。现 `src/cli.ts` 在 `validateArgs` 中调用 `parseTimestamp(--since, false)` / `parseTimestamp(--until, true)`，非法日期在 IO 前抛 `无效 since/until`；direct CLI 走既有错误路径退出码 1。
- 新增 `test/41-webui-fixes-01-02.test.ts`：API 非法/合法日期、npm/TS CLI 非法/合法 `--since/--until` 回归。

## Comments

- 2026-09-12 验证：
  - `node src/cli.ts --since 2026-02-30` → exit 1，stderr 含 `无效 since`。
  - API `since=2026-02-30` / `until=2026-07-32` / `since=2026-13-99` → 400；`2024-02-29` / `2026-08-31` → 200。
  - `npm run typecheck` clean；`npm test` 323/323 通过。
