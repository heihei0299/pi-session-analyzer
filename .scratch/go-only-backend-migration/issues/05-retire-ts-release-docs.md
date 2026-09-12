# 05: 删除 TypeScript 后端并完成 Go-only contract / release 收尾

**What to build:** 在统一 Go 数据流和运行时已经覆盖全部生产行为后，删除 TypeScript/Node 后端和迁移期 oracle，清理旧统计路径与构建链，并统一 module/repository/release/README/CONTEXT/ADR，使用户最终只看到一个边界清晰的 Go-only token-analyzer；OpenCode 保持同仓独立 `opencode-analyzer/` 项目。

**Blocked by:** 04: 收口 Refresh / Query / Watch / Go server / WebUI 运行时.

**Status:** resolved

- [ ] 只有在 Go canonical acceptance 覆盖全部用户可见行为后，才删除 TypeScript CLI/API/server/db/session/watch 等生产实现。
- [ ] 删除迁移期 TypeScript parity oracle 后，canonical golden tests 独立承担长期回归。
- [ ] 删除旧 SessionData 文件扫描/内存 aggregation 生产路径及仅服务该路径的遗留代码。
- [ ] 删除 Node backend、旧双 WebUI 同步脚本和不再需要的后端 TypeScript 构建配置。
- [ ] 运行 token-analyzer CLI/API/WebUI 不要求 Node/npm；如仍有前端工具链，其职责不得包含 backend runtime。
- [ ] repository/module/import/release metadata 统一使用当前项目名称，处理旧拼写带来的必要 breaking change。
- [ ] release 只发布 Go-only token-analyzer 所需产物，不再区分 Go/npm edition。
- [ ] README、安装/运行文档与项目开发指南只描述最终 Go-only 产品。
- [ ] CONTEXT/ADR 更新为 normalized ledger 唯一事实中心、Go-only backend、Refresh/Query 分离、Watch 新语义，以及 OpenCode 已抽离为独立项目边界。
- [ ] `opencode-analyzer/` 保持可整目录迁出；token-analyzer 不依赖其 runtime/API/UI/storage/credential。
- [ ] 最终 deletion test 成立：无 TS backend、无旧 SessionData 生产 aggregate、无 Codex SessionFileData 回绕、无 Watch 独立统计、无 WebUI 双副本、无 token-analyzer/OpenCode 运行时耦合。

## Answer

- 已删除旧 `SessionData` 文件扫描/parser/cache，rename 改用 `internal/pi` header-only locator；standalone `opencode-analyzer` 自有四载体 Pi audit、门控、fork/request/semantic 去重与月份边界 fixture。
- 已统一 `Makefile` 普通构建产物为 `dist/token-analyzer`，release 平台产物保持平台后缀；README、CONTEXT、ADR 与最终 Go-only 架构一致。
- 关键提交：`929f4ea`（R1/R2）、`379430e`（R3），R4/R5 收尾随本次提交完成；已执行 root/standalone 包级验证及静态 boundary 检查。`make build` 曾在用户停止本机编译前成功，`make release` 未执行。
