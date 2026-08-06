# 04 — 数字显示打磨

**What to build:** 紧凑数字格式化增加十亿档：≥ 10 亿显示为 B 单位（如 1.4B、13.85B），格式与既有 k/M 档一致（1 位小数、去尾 0）；10 亿以下行为不变。同时移除请求明细表头从未被脚本读取的无效 data-sort/data-dir 静态属性，排序箭头仍由前端状态渲染。

**Blocked by:** None — can start immediately

**Status:** ready-for-agent

- [ ] ≥10 亿显示 B（如 1415.6M → 1.4B），1 位小数去尾 0
- [ ] 1 万~10 亿仍显示 k/M，1 万以下千分位，均不变
- [ ] 请求明细表头 HTML 不再含 data-sort/data-dir 属性
- [ ] 排序箭头（▲/▼）随点击正常切换
