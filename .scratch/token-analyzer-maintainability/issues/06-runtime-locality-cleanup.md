# 06: 收口运行时私有结构并删除死代码

**What to build:** 让维护者可以在较小的局部内修改 server、CLI watch/output、会话重命名和同步辅助逻辑，同时保持已有用户行为；确认没有调用方的死代码和误导性入口不再增加理解成本。

**Blocked by:** 03: 统一 CLI/HTTP/Refresh 输入与 capability 校验；04: 让 source adapter 错误与诊断可观察；05: 锁定 Query 的 rollup、排序与分页契约

**Status:** resolved

- [x] server 的 HTTP handler、刷新状态、watch 和 Pi 重命名逻辑具有清晰的私有实现局部。
- [x] CLI 参数运行、watch loop、查询调用和输出格式化具有清晰的私有实现局部。
- [x] Pi/Codex source adapter 的 revision、读取、诊断和事务辅助逻辑不再把无关职责集中在同一实现单元。
- [x] 删除当前仓库内确认无调用方的死代码和误导性伪单例入口。
- [x] 不改变 CLI、HTTP、WebUI、QueryResult、Refresh 失败快照、watch 重试或文件重命名行为。
- [x] 不创建第二套 Query/Refresh 路径，不恢复 OpenCode runtime，也不引入新的框架或依赖。
- [x] canonical contract、server runtime、source adapter failure/retry 测试在整理后仍然成立。
