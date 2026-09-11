# 03: Pi 会话、请求与详情窗口切到 Go ledger

**What to build:** Pi 用户查看 sessions、requests 和会话详情时，所有列表、分页、排序、子代理合并与 totals 都来自同一个 Go ledger-backed Query Engine，不再重新扫描 session 文件计算统计结果；现有会话管理体验保持不变。

**Blocked by:** 02: Pi totals 全链路切到 Go normalized ledger.

**Status:** ready-for-agent

- [ ] sessions 的行、总数、totals、model/mixed、cwd、display name、isTask、parentSession 语义与 canonical expected 一致。
- [ ] requests 的逐请求时间、模型、token 字段、分页和排序与 canonical expected 一致。
- [ ] session detail 的 main/merged totals、childrenCount、子代理列表与请求时间线由 ledger/query 派生。
- [ ] WebUI 对 Pi 会话详情仍可正常打开，子代理仍按现有产品语义展示。
- [ ] Pi session rename 继续只作用于合法 Pi 会话，并在成功后刷新用户可见名称；统计数字不因重命名变化。
- [ ] Query Engine 在这些读取窗口中不扫描 session JSONL、不执行 refresh、不写 ledger。
- [ ] CLI 与 HTTP 对相同 sessions/requests 查询映射到相同 domain 结果。
