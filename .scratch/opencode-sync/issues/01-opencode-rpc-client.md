# 01: OpenCode SolidStart RPC 通讯与端点客户端

**What to build:** 为项目提供一个独立的逆向 RPC 客户端模块，封装对 `https://opencode.ai/_server` 的 SolidStart POST 请求与 Seroval 序列化/反序列化。支持通过 Cookie 认证与工作区 ID 检索，调用 `getUsageInfo` 获取分页请求明细、调用 `getCosts` 获取月度按模型细分成本以及调用 `workspaces` 获取工作区列表。包含针对 Seroval 编解码与端点响应解析的完整单元测试。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [ ] 实现 Seroval 载荷编码器（能够将参数序列化为 SolidStart RPC 接受的格式）
- [ ] 实现 Seroval 流式分块解码器（能够从 chunked 响应中正确提取并解析返回的数据对象）
- [ ] 实现 OpenCodeClient 类，支持传入 `auth` Cookie 与可选 `workspaceId`，提供 `getWorkspaces()`、`getCosts(workspaceId, year, month, tzOffset)`、`getUsageInfo(workspaceId, page)` 方法
- [ ] 封装健全的异常处理（认证失效 401/403/500、网络超时输出友好的错误诊断信息）
- [ ] 编写独立单元测试，使用 Mock 数据测试序列化、反序列化与方法解析（无网络依赖）

## 实施总结
- 提交：`b6dfd1c8e5da56c6f3fbb6b21b44505d58b08123` — `feat(opencode-sync): OpenCode RPC client (#01)`
- 实现的 seams：
  - T1 Seroval 编码器 `encodePayload(args: unknown[]) => string` — `src/opencode/seroval.ts:encodePayload`
  - T2 Seroval 解码器 `decodeStreamChunk(chunk: string) => unknown` — `src/opencode/seroval.ts:decodeStreamChunk` / `decodeResponseText`
  - T3 `OpenCodeClient.getWorkspaces(): Promise<WorkspaceInfo[]>` — `src/opencode/client.ts:getWorkspaces`
  - T4 `OpenCodeClient.getCosts(workspaceId, year, month, tzOffset?) => MonthlyCostsResult` — `src/opencode/client.ts:getCosts`
  - T5 `OpenCodeClient.getUsageInfo(workspaceId, page) => UsageRecord[]` — `src/opencode/client.ts:getUsageInfo`
  - T6 异常处理 401/403/500/网络超时友好错误 — `src/opencode/client.ts:rpc`
- 验收标准：
  - [x] 实现 Seroval 载荷编码器（能够将参数序列化为 SolidStart RPC 接受的格式） — `src/opencode/seroval.ts:encodePayload`，测试 `T1 encodePayload 空数组...` / `多参数...`
  - [x] 实现 Seroval 流式分块解码器（能够从 chunked 响应中正确提取并解析返回的数据对象） — `src/opencode/seroval.ts:decodeStreamChunk`，测试 `T2 decodeStreamChunk 解码 Seroval 包裹...` / `SSE 前缀...` / `普通 JSON...`
  - [x] 实现 OpenCodeClient 类，支持传入 `auth` Cookie 与可选 `workspaceId`，提供 `getWorkspaces()`、`getCosts(workspaceId, year, month, tzOffset)`、`getUsageInfo(workspaceId, page)` 方法 — `src/opencode/client.ts` + `src/opencode/types.ts`，测试 `T3 getWorkspaces 成功返回...` / `T4 getCosts 成功返回...` / `T5 getUsageInfo 成功返回...`
  - [x] 封装健全的异常处理（认证失效 401/403/500、网络超时输出友好的错误诊断信息） — `src/opencode/client.ts:rpc`，测试 `T6 401...` / `T6 403...` / `T6 500...` / `T6 网络异常...` / `T6 超时...` / `T6 缺少 auth...`
  - [x] 编写独立单元测试，使用 Mock 数据测试序列化、反序列化与方法解析（无网络依赖） — `test/01-opencode-client.test.ts` 25 项全绿，mock fetch 全局，不 mock 内部协作
- 测试结果：相关测试 25 项全绿（`TZ=Asia/Shanghai node --test test/01-opencode-client.test.ts` pass 25 fail 0）；typecheck 通过（`npm run typecheck` 无输出）
- typecheck：通过
- 文档对齐：无需更新 README（内部 RPC 客户端未暴露用户-facing CLI/webui，待后续 03/04 再对齐）；CONTEXT.md 已补充 OpenCode 领域术语（`## OpenCode 外部数据源与对账` 4 条），`src/opencode/types.ts` 严格按 `spec.md §Protocol & Data Contract Types` 定义 `OpenCodeUsageRecord`/`OpenCodeMonthlyCostItem`/`OpenCodeCostsResult`/`WorkspaceInfo`
- 遗留 / 后续建议：SolidStart 端点 Function ID 与 header 名（当前用 `x-opencode-fn` + `cookie: auth`）为最小可用子集自洽实现，若 OpenCode 发布新 build 导致 ID 变更或真实 header 需为 `x-solidstart-*`，需在 02 增量同步联调阶段以真实抓包修正；19 位 scaled `totalCost`（1e-8）格式化由 WebUI 层负责，客户端仅透传
