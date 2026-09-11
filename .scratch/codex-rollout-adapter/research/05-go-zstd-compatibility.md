# Research: Go zstd reader 依赖与兼容策略

**研究日期**：2026-09-08
**项目约束**：Go module `go 1.23.0`；Linux/macOS/Windows cross-build；本 effort 只实现 Go。

## Findings

1. **标准库不可用**
   - 本仓库当前 Go 环境执行 `go list std` 不包含 `compress/zstd`；Go 1.23 module 约束也不能依赖未来标准库新增 API。
   - 因此无法用标准库直接读取 Codex 的 `.jsonl.zst`。

2. **推荐 dependency：`github.com/klauspost/compress/zstd`**
   - 官方仓库的 zstd package 明确是 pure Go，提供 streaming decoder，支持普通 Zstandard stream。
   - `v1.18.0` 的 `go.mod` 要求 Go 1.22，兼容本项目 Go 1.23；当前更新版本的最低 Go 要求更高，因此 spec 应固定到兼容项目最低 Go 的版本线，而不是无约束使用 `latest`。
   - package 自带 BSD 风格许可；不要求 C compiler 或 CGO，适合现有多平台交叉编译。

3. **不推荐 CGO wrapper**
   - `github.com/valyala/gozstd` 和 `github.com/DataDog/zstd` 都是 C zstd wrapper；其文档要求 CGO/跨平台 C toolchain，直接破坏当前 release 的 `CGO_ENABLED=0` 约束。
   - 不应通过系统 `zstd` 命令解压：这会增加运行时外部依赖，并在 Windows/无命令环境中失败。

4. **不需要 seekable zstd**
   - Codex rollout 的 v1 读取是顺序 JSONL 扫描；不需要在压缩流内按解压后 byte offset 随机访问。
   - seekable zstd wrapper 会额外增加格式和 dependency 复杂度，当前应排除。

5. **plain/compressed sibling 仍由上层负责**
   - zstd reader 只负责把一个 `.jsonl.zst` 变成行流；发现器仍需实现 plain 优先、compressed sibling 去重、表示切换后的 revision 失效和完整重扫。

## 结论

spec 采用一个直接的纯 Go zstd streaming decoder dependency，建议锁定 `github.com/klauspost/compress/zstd` 的 Go 1.23 兼容版本线（研究快照中 `v1.18.0` 满足要求）。不使用 CGO、不调用外部命令、不引入 seekable format。实现时需要将 dependency 纳入 `go.mod` / `go.sum`，并补充 plain、zstd、损坏压缩流及 cross-build fixture 验证。

## Primary sources

- [`klauspost/compress v1.18.0 go.mod`](https://github.com/klauspost/compress/blob/v1.18.0/go.mod)
- [`klauspost/compress v1.18.0 zstd README`](https://github.com/klauspost/compress/blob/v1.18.0/zstd/README.md)
- [`klauspost/compress v1.18.0 LICENSE`](https://github.com/klauspost/compress/blob/v1.18.0/LICENSE)
- [`valyala/gozstd README`](https://github.com/valyala/gozstd/blob/master/README.md)
- [`DataDog/zstd README`](https://github.com/DataDog/zstd/blob/master/README.md)
