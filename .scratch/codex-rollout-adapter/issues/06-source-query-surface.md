# 06: Codex source selector 与查询表面契约

**Type:** grilling
**Status:** resolved
**Blocked by:** 04, 05

## Question

决定 `--source pi|codex|all`、`--codex-dir` 在 Go CLI、HTTP API 和现有 WebUI 中的完整行为：默认值、参数冲突、目录发现优先级、空数据与不支持数据提示、source filter、显式合计，以及 totals/sessions 的返回字段。

目标是保持现有 Pi 命令兼容，同时不给 Codex cost 或 requests 伪造可用值。


## Answer

用户确认采用以下 source/query 契约：

- **CLI source selector**：保留现有命令，增加 `--source pi|codex|all`；默认 `pi`，`--codex-dir` 只在 `codex/all` 时使用；不引入 `codex` 平行子命令，也不自动猜 source。
- **目录解析**：`--codex-dir` > `CODEX_HOME` > 默认 `~/.codex`；`--dir` 始终保持 Pi 目录语义。
- **WebUI**：复用总览、分组和 sessions 页面，增加 `Pi / Codex / All` source selector；Codex cost 显示 unavailable，requests 不可用；不复制独立 Codex Tab。
- **`all` 返回**：保持现有 `Totals` 结构；sessions 行增加 source，meta 标出实际参与统计的 sources；需要分源明细时使用 source filter。
- **异常可见性**：空目录、跳过文件和部分损坏不会让整个查询失败；返回部分结果并提供 warnings/diagnostics，CLI 显示跳过原因。
- **requests 边界**：Codex requests 查询明确拒绝；`source=all` 的 requests 查询同样拒绝，不返回仅 Pi 的伪合计。

因此 source/query 的产品决策已完成；关系 metadata 的展示只保留可诊断信息，不改变 v1 的不聚合规则。下一步是 [Go Codex 适配器 seam 与验收矩阵](09-go-implementation-acceptance.md)。
