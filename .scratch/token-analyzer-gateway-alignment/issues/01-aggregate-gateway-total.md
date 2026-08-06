# 01 — 聚合口径对齐网关

**What to build:** 让「总 token」与缓存率的定义与 pi-switch 网关完全一致——总 token = 总输入 + 输出（非缓存输入 + 缓存命中 + 输出，不含缓存写），缓存率 = 缓存读 / (非缓存输入 + 缓存读)。所有统计窗口（总量/会话/单请求/分组/周期）与所有输出形态（终端表格、JSON、CSV、webui）自动跟随同一口径；缓存写保持为独立指标列展示，仅总量与缓存率分母不含它。

**Blocked by:** None — can start immediately

**Status:** resolved

- [x] 聚合收尾计算改为：总 token = input + cacheRead + output（不含 cacheWrite）；缓存率分母 = input + cacheRead（不含 cacheWrite）
- [x] cacheWrite 保持独立累加与展示（表格「缓存写」列、JSON/CSV 字段、webui「缓存」列不变）
- [x] 组件和测试改造为网关口径：cacheWrite 非零 fixture 断言总 token 不含 cacheWrite、缓存率分母不含 cacheWrite；cacheWrite=0 fixture 断言数值与旧组件和相同（回归保护）
- [x] 回归验证：现有导出/API/fork 去重测试在 cacheWrite=0 fixture 下数值不变（测试全绿）
- [x] CLI 全量/筛选 totals 跑通，输出与网关公式核对一致

## 实施总结

- 提交：`ffd4d1f` — feat: 总 token 与缓存率口径对齐 pi-switch 网关（ADR-0002）
- 实现的 seams：CLI totals 公共接口（S1 总 token 列 / S2 缓存率列 / S3 cacheWrite=0 回归）
- 验收标准：5 条全部 `- [x]`（见上）
- 测试结果：116/116 全绿（含 s6 三个网关口径用例 + 7 个既有测试期望迁移）
- typecheck：通过
- 文档对齐：CONTEXT.md totalTokens/cacheRate 行同步（commit 内）；README 全面改写属 03 票，未在本票处理
- 遗留 / 后续建议：watch.ts `Increment extends Totals` 的 totalTokens/cacheRate 死字段为既有问题（review 发现，超出本票范围）；README 引用路径 `.scratch/token-analyzer-total-tokens/` 为旧残留，待 03 票校正
