# 08: Go zstd reader 依赖与兼容策略研究

**Type:** research
**Status:** resolved
**Blocked by:** None
**Research artifact:** `../research/05-go-zstd-compatibility.md`

## Question

在本项目 Go 1.23+、跨平台 release 和“只实现 Go”的约束下，确认读取 `.jsonl.zst` 所需的最小可靠方案：标准库能力、现有依赖是否可复用、是否需要新增纯 Go dependency、许可证/平台/构建影响，以及 plain/compressed sibling 切换时的 reader 行为。

研究必须优先使用 Go 官方文档和候选 dependency 的 primary source，最终给出一个可写进 spec 的选择，不实现代码。


## Answer

研究结论：Go 标准库不能满足 `.jsonl.zst`；CGO wrapper 和系统 `zstd` 命令破坏跨平台 release。采用一个纯 Go streaming decoder dependency，锁定 `github.com/klauspost/compress/zstd` 的 Go 1.23 兼容版本线，研究快照中 `v1.18.0` 满足最低版本要求；不引入 seekable zstd。

详情见 [`Go zstd reader 依赖与兼容策略研究`](../research/05-go-zstd-compatibility.md)。
