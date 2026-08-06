# 01 — 严格日期校验

**What to build:** CLI 与 webui API 对非法日历日期（如 2026-02-30、2026-07-32、2026-13-99）显式拒绝，不再静默归一化为相邻日期。CLI 传 `--since/--until` 非法日期时命令报错退出；API 携带非法 since/until 时返回 400 统一错误体。合法日期（含闰年 2024-02-29、月末 2026-08-31）行为不变，统计结果与修复前一致。

**Blocked by:** None — can start immediately

**Status:** ready-for-agent

- [ ] 纯日期参数回读比对：与输入不一致（JS Date 溢出归一化）即拒绝
- [ ] CLI `--since 2026-02-30` 退出码非 0 且 stderr 有错误信息
- [ ] API `since=2026-07-32` / `until=2026-13-99` → 400，错误体含「无效 since/until」
- [ ] 闰年与月末合法日期解析不变
- [ ] 既有时间筛选测试全量通过（无回归）
