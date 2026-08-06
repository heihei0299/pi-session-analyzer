# Token Analyzer WebUI 显示审查报告

> 审查依据：`webui-review-standard.md`（同目录）
> 审查对象：http://localhost:50080/（`token-analyzer serve`，数据目录 `~/.pi/agent/sessions`，231 会话）
> 审查时间：2026-08-06
> 审查方法：静态代码走查（src/webui.html、src/api.ts、src/analyze.ts、src/serialize.ts）+ 真实浏览器交互验证（agent-browser/Chrome）+ 接口响应核对（curl）
> 审查环境：Chrome（agent-browser），viewport 1280×800 / 800×900 / 390×844

## 0. 结论摘要

| 级别 | 数量 | 说明 |
|---|---|---|
| P0 阻断 | 0 | 无页面不可用/数据全错问题 |
| P1 严重 | 0 | — |
| P2 一般 | 3 | 非法日期静默归一化；Esc 取消重命名误报；分组表分组键未转义 |
| P3 轻微 | 6 | 大数无 B 单位；导出全量无保护；死属性；轮询冗余；自定义空值语义；筛选后会话数口径 |

**总体结论**：webui 主体功能（总览卡片、时间预设、自定义范围、明细服务端排序/分页、消息级时间过滤、会话管理重命名、导出、自动刷新、响应式、空态）经实测全部正确，与接口契约一致。发现的 9 项问题中 3 项 P2 集中在**参数校验契约**（非法日期被 JS Date 自动归一化）、**取消操作误报**（Esc 分支逻辑混叠）与**唯一未转义注入点**（分组表 key 列），均不影响正常 UI 操作的主路径，但建议按优先级修复。

---

## 1. 发现明细

### P2-1 非法日历日期被静默归一化，400 校验失效

- **位置**：`src/analyze.ts` `parseTimestamp()` 纯日期分支；`src/api.ts` `filtersFromParams()`
- **现象**：`parseTimestamp` 用 `new Date(y, mo-1, d)` 解析纯日期，JS Date 对溢出日期**自动归一化而不抛错**——`2026-02-30` → `2026-03-02`、`2026-07-32` → `2026-08-01`、`2026-13-99` → `2027-04-09`。`filtersFromParams` 依赖 `parseTimestamp` 抛错来实现「非法时间参数 → 400」，因此校验形同虚设。
- **复现**：`curl "http://127.0.0.1:50080/api/totals?since=2026-07-32&until=2026-07-32"` → **200**，返回 914 条请求（即 8/1 的数据）。对照 `since=2026-08-01` 同样 914 条。
- **预期**：非法日历日期（2/30、7/32、13 月等）→ 400 统一错误体 `{ error: "Bad Request", detail: "无效 since: ..." }`，与 `loadFiltered` 注释「非法时间参数 → 400」一致；且 E-2 标准要求非法参数 400。
- **影响**：前端 `<input type="date">` 会阻止正常输入非法日期，UI 主路径不可达；但 URL 直达、书签、外部脚本构造参数时筛选范围静默错位，数据口径错误。
- **修复建议**：纯日期分支在 `new Date` 后回读 `getFullYear/getMonth/getDate` 与输入比对，不一致即抛错（标准做法）。

### P2-2 Esc 取消重命名误报「显示名不能为空」

- **位置**：`src/webui.html` `exitEdit()`
- **现象**：`if (!save || !name) { if (name === "") errEl.textContent = "显示名不能为空"; restore; }`——`!save`（Esc/取消）与 `!name`（空名）共用同一分支，Esc 取消时若输入框内容为空，仍会显示「显示名不能为空」错误提示。
- **复现**：会话管理 → 点击会话名进入行内编辑 → 清空输入框 → 按 Esc → 名称恢复为原值，但行内错误区显示「显示名不能为空」。
- **预期**：Esc 取消编辑不应显示任何错误（标准 I-10）。
- **影响**：轻微误导，用户以为操作失败；实际名称未变。
- **修复建议**：拆分为 `if (!save) { restore; return; }` 与 `if (!name) { err; restore; return; }`。

### P2-3 分组表分组键未转义（唯一 XSS 注入点）

- **位置**：`src/webui.html` `renderTable()`（`<td>${f ? f(v) : v}</td>` 直接内插）+ `refreshGroups()` 的 key 列
- **现象**：分组表（总览页「按模型/按 cwd」）的**分组键列**（model 名 / cwd 路径）直接拼接进 `innerHTML`，未过 `escapeHtml`。对比：明细表的 displayName/model/cwd、会话管理的 name/cwd 均已转义，分组表是唯一遗漏。
- **复现**：当前数据（9 个 model、24 个 cwd）不含 HTML 特殊字符，未实际触发渲染错乱；但构造含 `<img onerror=...>` 的 model 名/cwd 的会话文件后刷新即可触发注入（本地服务、数据来自本机文件，实际攻击面低，但显示错乱风险真实）。
- **预期**：所有注入 `innerHTML` 的动态字符串经 `escapeHtml`（标准 S-1）。
- **影响**：潜在 XSS；若 cwd 路径含 `&`、`<` 等字符（合法文件名）将显示错乱。
- **修复建议**：`refreshGroups` 中 key 列套 `escapeHtml`；或 `renderTable` 对非数字列统一转义。

### P3-1 大数字无 B（十亿）单位

- **位置**：`src/webui.html` `fmtCompact()`
- **现象**：仅 `≥1e6 → M`、`≥1e4 → k`，无 `≥1e9 → B` 档。实测卡片：缓存 `1384.4M`（=1.38B）、总 token `1415.6M`（=1.42B），6-7 位数字可读性差。
- **预期**：≥1e9 显示 `1.4B`（标准 D-8）。
- **修复建议**：`fmtCompact` 增加 `n >= 1e9 → (n/1e9).toFixed(1) + "B"`。

### P3-2 导出全量数据无进度/超时保护

- **位置**：`src/webui.html` `exportData()`；`src/api.ts` 无参数分页兼容
- **现象**：导出无筛选时拉全量 `/api/requests`（实测 **28.4MB** 原始 JSON，pretty 缩进后更大——今天 2533 条即 13.7MB），`JSON.stringify` 拼接大对象后 Blob 下载。数据增长（10 万+ 请求）后导出将明显卡顿、内存压力上升，无进度提示。
- **预期**：功能可用（设计使然，导出需全量），但建议标注限制或加导出中提示（标准 P-4 边界）。
- **影响**：当前规模（11969 条）可正常导出（实测成功）；规模扩大后体验劣化。

### P3-3 请求明细 thead 无效属性（死代码）

- **位置**：`src/webui.html` `<thead id="request-head" data-sort="timestamp" data-dir="desc">`
- **现象**：`data-sort`/`data-dir` 属性从未被 JS 读取（默认排序定义在 `detailState.requests`），属无效残留。
- **预期**：删除属性或由 JS 统一读取（标准 C-1）。

### P3-4 自动刷新轮询冗余请求

- **位置**：`src/webui.html` `poll()` / `refreshOverview()`
- **现象**：overview 下每周期先 `snapshot()`（totals+groups 2 请求）做对比，变化后再 `refreshAll()` → `refreshOverview()` 重拉 totals+meta+groups（3 请求），一周期最多 5 个请求，其中 totals/groups 重复。
- **预期**：变化后仅重拉变化的部分或复用 snapshot 数据（标准 C-3）。
- **影响**：自动刷新默认 off，实际影响小。

### P3-5 自定义预设空值语义模糊

- **位置**：`src/webui.html` `applyPreset("custom")`
- **现象**：点「自定义」且未填任何值时 `since/until = null`，数据=全部，状态行显示全量范围，但预设按钮高亮为「自定义」。用户可能误以为已应用了某个筛选。
- **预期**：空值=全部（合理回退），但可提示（如状态行显示「自定义（未设置）= 全部」或预填默认范围）。

### P3-6 筛选后状态行会话数仍为全量

- **位置**：`src/webui.html` `updateStatusRange()`；`src/api.ts` `buildMeta()`
- **现象**：应用时间筛选后状态行「会话数: 231」始终为全量（meta 的 sessionCount），与当页筛选后的数据（如今天仅 15 个会话）口径不同，可能误导。
- **预期**：属设计（meta 为全量基准），可标注「会话数: 231（全量）」或在筛选时给出筛选后会话数。

---

## 2. 通过项记录（抽查覆盖）

| 标准项 | 验证结果 |
|---|---|
| L-1~L-7 布局 | 4 tab 切换正常、工具栏/状态行/8 卡片/分组表/分页器/会话分组齐全 |
| D-1/D-2 卡片 | 与 `/api/totals` 逐项一致（输出=output+reasoning、缓存=cacheRead+cacheWrite）；cost=0 显示「费率未配置」 |
| D-4/D-5 明细 | 会话/请求明细列与接口字段对应；total 与行数相符（231 行·1/12 页、11969 行·1/599 页） |
| D-9 空态 | 筛选 2026-06-01~06-02 → 0 行、按钮禁用、无旧行残留 |
| I-2 时间预设 | 今天=当日 00:00~23:59:59、7天=7/31~8/6、全部=无筛选，按钮 active 正确 |
| I-3 自定义范围 | 纯日期/带时分/单边 since 均生效；页面 6520 与接口完全一致 |
| I-4 状态行范围 | 纯日期补 00:00/23:59:59；未筛选显示全量数据范围；去 T 显示 |
| I-5/I-6 排序分页 | 表头 asc→desc 切换、箭头正确、排序回第 1 页；翻页边界禁用正确、改页大小回第 1 页、尾页 31 行 |
| I-7/I-8 刷新轮询 | 刷新按钮即时重拉；5s 自动刷新 6.2s 内 2 次请求，静默替换 |
| I-9 导出 | JSON 导出成功（13.7MB pretty，随「今天」筛选），totals/sessions/requests 三段结构完整 |
| I-10 重命名 | 行内编辑/Enter 保存/Esc 取消/空名拦截/非法字符 400 均验证；同名幂等成功无副作用 |
| I-11 竞态 | 快速切 tab 8 次，无未捕获异常、无旧数据覆盖 |
| T-1~T-3 时间语义 | 本地时间显示正确（UTC→东八区换算核对）；消息级过滤跨天会话保留（今天 2515 条，接口 2516 一致）；状态行边界正确 |
| V-1~V-5 视觉 | 暗色主题一致、数值右对齐、名称列 240px ellipsis+title 截断验证（scrollWidth 15119 > clientWidth 240）、hover 反馈正常 |
| E-1/E-2 错误 | API 失败走错误横幅；page/size/sortKey/sortDir 单独出现、非法值均 400 |
| S-1/S-2 安全 | 明细表/会话管理动态字符串均转义（仅分组表 key 例外见 P2-3）；重命名非法字符净化正确 |
| P-1 分页 | 明细端点只传输当前页（实测 page/size 生效） |
| 响应式 | 1280/800/390 三档均无 body 横向溢出；卡片 auto-fit 自适应；工具栏/顶栏 flex-wrap 换行 |

## 3. 修复建议优先级

1. **P2-1 非法日期校验**（api.ts/analyze.ts，5 行内改动）：纯日期回读比对，保证契约「非法时间参数 → 400」成立。
2. **P2-2 Esc 取消逻辑拆分**（webui.html，2 行改动）：`!save` 与 `!name` 分支分离。
3. **P2-3 分组键转义**（webui.html，1 行改动）：`refreshGroups` key 列套 `escapeHtml`。
4. P3 项可随下次迭代批量处理（B 单位、死属性删除、状态行口径标注）。

## 4. 附注

- 审查期间数据源（`~/.pi/agent/sessions`）处于活跃写入（pi 正在使用），页面与接口数值存在秒级漂移（如 2496 vs 2498），属正常现象，非缺陷。
- 视觉证据截图存于 `.scratch/webui-review/`（overview.png、requests-tab.png）。
- 审查过程中产生的测试下载文件已清理。
