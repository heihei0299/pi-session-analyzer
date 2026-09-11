# 04 — 数字显示打磨

**What to build:** 紧凑数字格式化增加十亿档：≥ 10 亿显示为 B 单位（如 1.4B、13.85B），格式与既有 k/M 档一致（1 位小数、去尾 0）；10 亿以下行为不变。同时移除请求明细表头从未被脚本读取的无效 data-sort/data-dir 静态属性，排序箭头仍由前端状态渲染。

**Blocked by:** None — can start immediately

**Status:** resolved

- [x] ≥10 亿显示 B（如 1415.6M → 1.4B），1 位小数去尾 0
- [x] 1 万~10 亿仍显示 k/M，1 万以下千分位，均不变
- [x] 请求明细表头 HTML 不再含 data-sort/data-dir 属性
- [x] 排序箭头（▲/▼）随点击正常切换

## 实施说明

- `fmtCompact` 新增 `>=1e9` 分支，格式化为 `(n/1e9).toFixed(1).replace(/\.0$/,"") + "B"`，1 位小数去尾 0。10 亿以下档位保持原样。
- `<tr id="request-head">` 移除了死属性 `data-sort="timestamp" data-dir="desc"`，默认排序与方向统一由 `detailState.requests` 承载，排序箭头行为不受影响。
- 同步更新了 `test/09-detail-views-ui.test.ts` 并新增 `test/43-webui-fixes-04-05.test.ts`。
