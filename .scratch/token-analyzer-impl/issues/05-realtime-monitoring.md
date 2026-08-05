# 05 — 实时监控：--watch（tail -f + 轮询兜底）

**What to build:** 给工具增加**实时监控**模式（`--watch`）——跟随正在运行的 pi 进程，实时刷新 token 消耗统计。基于会话 JSONL append-only、消息级即时写盘的特性：`tail -f` 式跟随新追加行为主 + 定时轮询兜底（断线/文件替换重同步）。

端到端行为：
- 增量读取器：为每个会话文件维护读取位置（offset），跟随新追加行，仅解析完整一行 JSON 记录
- 增量边界：一行完整 `role=assistant` message entry（含 usage）——usage 在请求完成后一次性落盘，非流式分片
- 断线/文件替换兜底：定时轮询比对 mtime / 行数 / size / inode，检测文件替换或截断后重同步，不丢不重
- `--watch` 模式持续刷新输出（终端表格/JSON/CSV 与静态模式同一套结构与格式）
- 实时增量与静态统计共享同一数据读取与聚合层

**Blocked by:** 01 — 最小闭环：总消耗量统计

**Status:** resolved

## Acceptance criteria

- [x] 向正在 append 的会话文件追加新 assistant 消息 → `--watch` 输出增量出现，数字正确累加
- [x] 文件被替换/截断 → 轮询检测并重同步，不丢已读行、不重复计数
- [x] 非 assistant 行（user/toolResult/model_change 等）不触发统计变化
- [x] fixture 覆盖：模拟 append 追加、模拟文件替换/截断

## 实施总结
- 提交：`8ac3b4a` — feat: 实时监控 --watch（tail 增量 + 轮询兜底）
- 实现的 seams：S1 增量读取器（offset 续读、不重复）/ S2 非 assistant 行过滤 / S3 文件替换重同步（inode 变化）/ S4 文件截断重同步（truncate）/ S5 --watch 集成（CLI 解析 + runWatch 单步 + applyIncrements 累加）
- 验收标准：4 条全部 `- [x]`（见上）
- 测试结果：46/46 全绿（`npm test`）
- typecheck：通过（`npm run typecheck`，tsc --noEmit strict）
- 真实数据核对：IncrementalReader 初始扫描与静态 totalsFromFiles 对同一快照完全一致（残留文件修复后）；追加一条 assistant 消息后 requests +1；无追加时二次读取不重复
- Code Review 修复：跨轮半行永久丢失（offset 恒为完整行边界）/ 替换截断重读重复计数（扣减旧贡献 + 累加新内容，S3/S4 改净效果断言）/ 同 inode 截断重写 mtime 检测（size===offset && mtime 变）/ --watch 与筛选组合显式报错（原静默忽略）/ 死代码 initialize() 删除 / collectJsonlFiles 复用 analyze 导出（消除重复）/ S5d 补 runWatch 本体测试
- 遗留 / 后续建议：--watch 仅支持 totals+table（阶段②确认范围，JSON/CSV 刷新未实现）；文件删除后其历史贡献保留在 totals（语义选择：删除 = 历史已发生）；contrib 累计逻辑可后续简化
