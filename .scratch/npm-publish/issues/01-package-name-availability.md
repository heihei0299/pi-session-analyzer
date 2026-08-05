# 01 — npm 包名可用性研究

Type: research
Status: resolved

## Question

`token-analyzer` 作为 npm 包名是否可用？不可用（或被占用且语义冲突）时选哪个包名？

具体子问题：
1. npmjs 上 `token-analyzer` 是否已被占用（registry API 查询；记录占用者、description、版本数、周下载量）
2. 若被占用：评估备选名可用性——`@heihei0299/token-analyzer`（个人 scope）、`token-analyzer-cli`、`pi-token-analyzer`、`token-analyzer-ts`（按需增补）
3. 若可用：确认无保留字 / 大小写规范问题，`token-analyzer` 直接可用
4. 产出**推荐包名 + 理由**（决策输入，供 06 落地 `package.json.name`）

## 调研指引

- 权威来源：npm registry API（`https://registry.npmjs.org/<name>`——404 = 可用，200 = 被占用）；npmjs.com 包页面
- 对每个候选名：查询占用状态；被占用则记录 owner、最近 publish 时间、周下载量，判断是否同类型活跃工具（影响语义冲突判断）
- 每个结论给出证据：registry API 响应（status code / 版本数 / 时间）
- 输出格式：每个候选名一行结论 + 推荐包名 + 理由

## Answer

### 结论摘要

- **`token-analyzer` 可用**：npm registry API `https://registry.npmjs.org/token-analyzer` 返回 `404 {"error":"Not found"}`（不存在 = 可用），小写、无保留字、语义准确
- **推荐包名：`token-analyzer`**（直接使用，不加 scope）
- 备选核验（worker 并行调研）：`@heihei0299/token-analyzer`、`token-analyzer-cli`、`pi-token-analyzer`、`token-analyzer-ts` 均返回 404 可用；仅当首发前被抢注时按序启用

### 证据

| 候选名 | registry 状态 |
|--------|---------------|
| token-analyzer | 404 Not found（可用） |
| @heihei0299/token-analyzer | 404（可用，备选 1） |
| token-analyzer-cli | 404（可用，备选 2） |
| pi-token-analyzer | 404（可用，备选 3） |
| token-analyzer-ts | 404（可用，备选 4） |

### 对 06 的落地输入

- `package.json.name = "token-analyzer"`（不加 scope）
- 残留风险：首发前存在极小被抢注窗口；若 06 落地时已不可用，改 `@heihei0299/token-analyzer` 并回注本 ticket
