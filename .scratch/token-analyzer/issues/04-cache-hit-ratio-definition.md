# 04 — 缓存率定义

Type: grilling
Status: resolved
Blocked by: 01

## Question

「缓存率」指标的精确定义是什么？

候选口径：
- A: `cacheRead / input`
- B: `(cacheRead + cacheWrite) / input`
- C: 按官方口径（Anthropic / OpenAI / 所用 provider 的缓存比例定义）
- D: 自定义（请说明）

## 背景

需求明确包含「缓存率」指标；usage 结构含 `cacheRead` 与 `cacheWrite` 两个字段，口径选择影响指标含义。此决策被「usage 字段语义研究」（ticket 01）的结论支撑。

## Answer

**决策：口径 A** —— `缓存率 = cacheRead / (input + cacheRead + cacheWrite)`

- 语义：命中缓存的输入 token 占**全部输入类 token**（未命中 input + 命中 cacheRead + 写入 cacheWrite）的比例
- cacheWrite 视为未命中的新输入计入分母；分子分母均为输入类 token
- 字段语义依据：ticket 01（input = 未命中缓存输入；cacheRead = 命中缓存输入；cacheWrite = 写入缓存输入）
- 边界：分母为 0（全 0 usage / 失败消息）时缓存率记为 `0`（spec 中明确）
- 聚合规则：各统计窗口（总/会话/单请求）**先求和分子分母再除**，避免对零值请求做除法
