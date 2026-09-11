# 07 — 自定义预填

**What to build:** 点击「自定义」时间预设时，6 个时间控件（起止日期 + 时分下拉）预填当前生效的筛选范围；无生效筛选（全部）时预填全量数据范围的本地日期。用户进入自定义即可直接微调，不再面对空控件。预填后修改任一控件仍即时生效。

**Blocked by:** None — can start immediately

**Status:** resolved

- [x] 有生效筛选时点「自定义」：控件值 = 当前 since/until（含时分拆分）
- [x] 无筛选时点「自定义」：控件值 = 全量数据范围（本地日期）
- [x] 预填后修改任一控件即时重新拉取数据
- [x] 切回其他预设：自定义控件隐藏、筛选按新预设生效

## 实施说明

- `applyPreset` 在切换预设时统一调用 `$("#custom-range").classList.toggle("hidden", preset !== "custom")`，切回 `today`/`7d`/`30d`/`all` 时确保隐藏自定义控件。
- 点击「自定义」预设时，通过 IIFE `setCustom` 函数预填 6 个时间控件：优先采用当前生效的 `state.since`/`state.until`（拆分本地年月日与时分下拉）；若无筛选则回退到 `lastMeta.dataRange`（或当前本地日期）。预填后调用 `applyCustomRange()` 保持即时筛选生效。
- 6 个输入控件均监听了 `input`/`change` 事件，修改任一控件即重新触发 `applyCustomRange()`。
- 自动化测试见 `test/42-webui-fixes-03-07.test.ts`。
