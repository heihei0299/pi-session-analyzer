import { test } from "node:test";
import assert from "node:assert/strict";

// T1 Seroval 编码器：encodePayload(args: unknown[]) => string
import { encodePayload, decodeStreamChunk } from "../src/opencode/seroval.ts";
import { OpenCodeClient } from "../src/opencode/client.ts";

test("T1 encodePayload 空数组 produces seroval envelope with l:0 and empty a", () => {
  const out = encodePayload([]);
  const parsed = JSON.parse(out) as any;
  // 独立字面量期望：严格匹配 spec 格式 {t:{t:9,i:0,l:N,a:[...]},f:31,m:[]}
  assert.equal(parsed.t.t, 9);
  assert.equal(parsed.t.i, 0);
  assert.equal(parsed.t.l, 0);
  assert.deepEqual(parsed.t.a, []);
  assert.equal(parsed.f, 31);
  assert.deepEqual(parsed.m, []);
});

test("T1 encodePayload 多参数 preserves length and values", () => {
  const args: unknown[] = [1, "hello", { x: 1 }, null];
  const out = encodePayload(args);
  const parsed = JSON.parse(out) as any;
  assert.equal(parsed.t.t, 9);
  assert.equal(parsed.t.l, 4);
  assert.deepEqual(parsed.t.a, [1, "hello", { x: 1 }, null]);
  assert.equal(parsed.f, 31);
});

test("T1 encodePayload 字符串与数字混合 reversibly encoded", () => {
  const args = ["wrk_abc123", 2026, 8];
  const out = encodePayload(args);
  // 独立期望：输出为 string 且 JSON 可解析
  assert.equal(typeof out, "string");
  const parsed = JSON.parse(out) as any;
  assert.equal(parsed.t.l, 3);
  assert.equal(parsed.t.a[0], "wrk_abc123");
  assert.equal(parsed.t.a[1], 2026);
  assert.equal(parsed.t.a[2], 8);
});

// T2 Seroval 解码器：decodeStreamChunk(chunk: string) => unknown
test("T2 decodeStreamChunk 解码 Seroval 包裹的数组数据", () => {
  const payload = [{ id: "usg_1", model: "m1", cost: 100 }, { id: "usg_2", model: "m2", cost: 200 }];
  // 服务端返回形如 encodePayload([payload])
  const chunk = encodePayload([payload]);
  const decoded = decodeStreamChunk(chunk) as unknown[];
  assert.deepEqual(decoded, payload);
});

test("T2 decodeStreamChunk 解码单对象负载", () => {
  const obj = { usage: [{ date: "2026-08-01", model: "x-preview", totalCost: 891912946 }], keys: [] };
  const chunk = encodePayload([obj]);
  const decoded = decodeStreamChunk(chunk) as any;
  assert.deepEqual(decoded.usage[0].date, "2026-08-01");
  assert.equal(decoded.usage[0].totalCost, 891912946);
});

test("T2 decodeStreamChunk 处理 SSE 前缀 data:", () => {
  const payload = [{ id: "usg_sse" }];
  const chunk = `data: ${encodePayload([payload])}`;
  const decoded = decodeStreamChunk(chunk) as unknown[];
  assert.deepEqual(decoded, payload);
});

test("T2 decodeStreamChunk 处理普通 JSON（无 Seroval 包裹）", () => {
  const chunk = JSON.stringify({ hello: "world", count: 3 });
  const decoded = decodeStreamChunk(chunk) as any;
  assert.equal(decoded.hello, "world");
  assert.equal(decoded.count, 3);
});

test("T2 decodeStreamChunk 空 chunk 返回 null", () => {
  assert.equal(decodeStreamChunk("   "), null);
  assert.equal(decodeStreamChunk(""), null);
});

test("T2 encode→decode 往返一致性（独立字面量验证）", () => {
  const args: unknown[] = ["wrk_test", 42];
  const encoded = encodePayload(args);
  // 解码采用 encodePayload([args]) 的包裹形式模拟服务端： l:1 包单个 args 数组
  const serverChunk = encodePayload([args]);
  const decoded = decodeStreamChunk(serverChunk) as unknown[];
  assert.deepEqual(decoded, args);
  // 同时验证 encoded 本身包含正确信封
  const parsed = JSON.parse(encoded) as any;
  assert.equal(parsed.t.l, 2);
  assert.deepEqual(parsed.t.a, args);
});

// ---------- Helpers for client mock ----------
function mockResponse(data: unknown, status = 200, ok = status >= 200 && status < 300) {
  const serovalText = encodePayload([data]);
  return {
    ok,
    status,
    headers: { get: (_k: string) => "application/json" } as any,
    async text() { return serovalText; },
    async json() { return data; },
  };
}
function makeMockFetch(impl: (url: string | URL, init?: RequestInit) => unknown) {
  return impl as unknown as typeof fetch;
}

// T3 OpenCodeClient.getWorkspaces(): Promise<WorkspaceInfo[]> 无参（用 auth cookie），返回工作区列表
test("T3 getWorkspaces 成功返回工作区列表（mock fetch）", async () => {
  const workspaces = [
    { id: "wrk_aaa", name: "团队A" },
    { id: "wrk_bbb", name: "团队B", slug: "team-b" },
  ];
  let capturedUrl = "";
  let capturedInit: RequestInit | undefined;
  const mockFetch = makeMockFetch(async (url, init) => {
    capturedUrl = String(url);
    capturedInit = init as RequestInit;
    return mockResponse(workspaces) as any;
  });
  const client = new OpenCodeClient({ auth: "test_cookie_value", fetchImpl: mockFetch });
  const result = await client.getWorkspaces();
  // 独立字面量期望
  assert.equal(result.length, 2);
  assert.equal(result[0].id, "wrk_aaa");
  assert.equal(result[0].name, "团队A");
  assert.equal(result[1].id, "wrk_bbb");
  // 验证请求：POST 到 _server，带 cookie 与 function id
  assert.match(capturedUrl, /opencode\.ai\/_server/);
  assert.equal((capturedInit?.method ?? "POST").toUpperCase(), "POST");
  const headers = (capturedInit?.headers ?? {}) as Record<string, string>;
  const cookieHeader = (headers["cookie"] ?? headers["Cookie"] ?? "") as string;
  assert.match(cookieHeader, /auth=test_cookie_value/);
  const contentType = (headers["content-type"] ?? headers["Content-Type"] ?? "") as string;
  assert.match(contentType, /application\/json/);
});

test("T3 getWorkspaces 构造函数支持字符串形式 auth", async () => {
  const workspaces = [{ id: "wrk_str", name: "str-auth" }];
  const mockFetch = makeMockFetch(async () => mockResponse(workspaces) as any);
  const client = new OpenCodeClient("my_cookie_str", mockFetch);
  const result = await client.getWorkspaces();
  assert.equal(result[0].id, "wrk_str");
  assert.equal(result[0].name, "str-auth");
});

test("T3 getWorkspaces 空列表返回空数组", async () => {
  const mockFetch = makeMockFetch(async () => mockResponse([]) as any);
  const client = new OpenCodeClient({ auth: "c", fetchImpl: mockFetch });
  const result = await client.getWorkspaces();
  assert.deepEqual(result, []);
});

// T4 OpenCodeClient.getCosts(workspaceId, year, month, tzOffset?) => MonthlyCostsResult
test("T4 getCosts 成功返回月度成本聚合", async () => {
  const costsResult = {
    usage: [
      { date: "2026-08-01", model: "x-preview-f-free", totalCost: 891912946, keyId: "key_1", plan: "lite" },
      { date: "2026-08-02", model: "deepseek-v4-flash", totalCost: 12345678, keyId: "key_1", plan: "lite" },
    ],
    keys: [{ id: "key_1", name: "默认" }],
  };
  let capturedArgs: unknown[] | null = null;
  const mockFetch = makeMockFetch(async (url, init) => {
    const body = String((init as any)?.body ?? "");
    if (body) {
      const parsed = JSON.parse(body) as any;
      capturedArgs = parsed.t.a as unknown[];
    } else {
      const u = new URL(String(url));
      const argsParam = u.searchParams.get("args");
      if (argsParam) capturedArgs = JSON.parse(argsParam) as unknown[];
    }
    return mockResponse(costsResult) as any;
  });
  const client = new OpenCodeClient({ auth: "cost_cookie", fetchImpl: mockFetch });
  const result = await client.getCosts("wrk_cost_test", 2026, 8);
  // 独立字面量断言
  assert.equal(result.usage.length, 2);
  assert.equal(result.usage[0].date, "2026-08-01");
  assert.equal(result.usage[0].model, "x-preview-f-free");
  assert.equal(result.usage[0].totalCost, 891912946);
  assert.equal(result.keys[0].id, "key_1");
  // 验证携带 workspaceId, year, month（GET args 或 POST body）
  assert.ok(capturedArgs !== null);
  assert.equal((capturedArgs as unknown[]).length, 3); // 未传 tzOffset 时为 3 参
  assert.equal((capturedArgs as unknown[])[0], "wrk_cost_test");
  assert.equal((capturedArgs as unknown[])[1], 2026);
  assert.equal((capturedArgs as unknown[])[2], 8);
});

test("T4 getCosts 支持 tzOffset 可选参数", async () => {
  const costsResult = { usage: [], keys: [] };
  let capturedArgs: unknown[] | null = null;
  const mockFetch = makeMockFetch(async (url, init) => {
    const body = String((init as any)?.body ?? "");
    if (body) {
      const parsed = JSON.parse(body) as any;
      capturedArgs = parsed.t.a as unknown[];
    } else {
      const u = new URL(String(url));
      const argsParam = u.searchParams.get("args");
      if (argsParam) capturedArgs = JSON.parse(argsParam) as unknown[];
    }
    return mockResponse(costsResult) as any;
  });
  const client = new OpenCodeClient({ auth: "c", fetchImpl: mockFetch });
  const result = await client.getCosts("wrk_123", 2026, 7, -480);
  assert.deepEqual(result.usage, []);
  assert.ok(capturedArgs !== null);
  assert.equal((capturedArgs as unknown[]).length, 4);
  assert.equal((capturedArgs as unknown[])[0], "wrk_123");
  assert.equal((capturedArgs as unknown[])[1], 2026);
  assert.equal((capturedArgs as unknown[])[2], 7);
  assert.equal((capturedArgs as unknown[])[3], -480);
});

test("T4 getCosts 空数据返回空 usage", async () => {
  const mockFetch = makeMockFetch(async () => mockResponse({ usage: [], keys: [] }) as any);
  const client = new OpenCodeClient({ auth: "c", fetchImpl: mockFetch });
  const result = await client.getCosts("wrk_empty", 2026, 1);
  assert.deepEqual(result.usage, []);
  assert.deepEqual(result.keys, []);
});

// T5 OpenCodeClient.getUsageInfo(workspaceId, page) => UsageRecord[]
test("T5 getUsageInfo 成功返回分页使用历史", async () => {
  const records = [
    {
      id: "usg_abc123",
      workspaceID: "wrk_test",
      timeCreated: "2026-08-20T01:23:45.000Z",
      timeUpdated: "2026-08-20T01:23:45.000Z",
      timeDeleted: null,
      model: "x-preview-f-free",
      provider: "inf.oa-compat",
      inputTokens: 1200,
      outputTokens: 800,
      reasoningTokens: 200,
      cacheReadTokens: 100,
      cacheWrite5mTokens: 10,
      cacheWrite1hTokens: null,
      cost: 0.0023,
      keyID: "key_1",
      sessionID: "sess_001",
      enrichment: null,
    },
    {
      id: "usg_def456",
      workspaceID: "wrk_test",
      timeCreated: "2026-08-20T02:00:00.000Z",
      timeUpdated: "2026-08-20T02:00:00.000Z",
      timeDeleted: null,
      model: "deepseek-v4-flash",
      provider: "inf.oa-compat",
      inputTokens: 500,
      outputTokens: 300,
      reasoningTokens: null,
      cacheReadTokens: null,
      cacheWrite5mTokens: null,
      cacheWrite1hTokens: null,
      cost: 0.001,
      keyID: "key_1",
      sessionID: null,
      enrichment: null,
    },
  ];
  let capturedArgs: unknown[] | null = null;
  const mockFetch = makeMockFetch(async (url, init) => {
    const body = String((init as any)?.body ?? "");
    if (body) {
      const parsed = JSON.parse(body) as any;
      capturedArgs = parsed.t.a as unknown[];
    } else {
      const u = new URL(String(url));
      const argsParam = u.searchParams.get("args");
      if (argsParam) capturedArgs = JSON.parse(argsParam) as unknown[];
    }
    return mockResponse(records) as any;
  });
  const client = new OpenCodeClient({ auth: "usage_cookie", fetchImpl: mockFetch });
  const result = await client.getUsageInfo("wrk_test", 0);
  assert.equal(result.length, 2);
  assert.equal(result[0].id, "usg_abc123");
  assert.equal(result[0].model, "x-preview-f-free");
  assert.equal(result[0].inputTokens, 1200);
  assert.equal(result[0].sessionID, "sess_001");
  assert.equal(result[1].id, "usg_def456");
  assert.equal(result[1].model, "deepseek-v4-flash");
  // 验证请求包含 workspaceId 与 page
  assert.ok(capturedArgs !== null);
  assert.equal((capturedArgs as unknown[]).length, 2);
  assert.equal((capturedArgs as unknown[])[0], "wrk_test");
  assert.equal((capturedArgs as unknown[])[1], 0);
});

test("T5 getUsageInfo 分页 page=1 正确传递", async () => {
  let capturedArgs: unknown[] | null = null;
  const mockFetch = makeMockFetch(async (url, init) => {
    const body = String((init as any)?.body ?? "");
    if (body) {
      const parsed = JSON.parse(body) as any;
      capturedArgs = parsed.t.a as unknown[];
    } else {
      const u = new URL(String(url));
      const argsParam = u.searchParams.get("args");
      if (argsParam) capturedArgs = JSON.parse(argsParam) as unknown[];
    }
    return mockResponse([]) as any;
  });
  const client = new OpenCodeClient({ auth: "c", fetchImpl: mockFetch });
  const result = await client.getUsageInfo("wrk_test", 1);
  assert.deepEqual(result, []);
  assert.ok(capturedArgs !== null);
  assert.equal((capturedArgs as unknown[])[1], 1);
});

test("T5 getUsageInfo 空页返回空数组", async () => {
  const mockFetch = makeMockFetch(async () => mockResponse([]) as any);
  const client = new OpenCodeClient({ auth: "c", fetchImpl: mockFetch });
  const result = await client.getUsageInfo("wrk_test", 5);
  assert.deepEqual(result, []);
});

// T6 异常处理：401/403/500/网络超时 抛友好错误（message 含“认证失效/网络超时/凭证过期”等诊断语）
test("T6 401 认证失效抛友好错误（凭证过期）", async () => {
  const mockFetch = makeMockFetch(async () => ({ ok: false, status: 401, async text() { return ""; } }) as any);
  const client = new OpenCodeClient({ auth: "bad_cookie", fetchImpl: mockFetch });
  await assert.rejects(() => client.getWorkspaces(), (e: Error) => {
    assert.match(e.message, /认证失效|凭证过期/);
    return true;
  });
});

test("T6 403 认证失效抛友好错误", async () => {
  const mockFetch = makeMockFetch(async () => ({ ok: false, status: 403, async text() { return ""; } }) as any);
  const client = new OpenCodeClient({ auth: "bad_cookie", fetchImpl: mockFetch });
  await assert.rejects(() => client.getCosts("wrk", 2026, 8), (e: Error) => {
    assert.match(e.message, /认证失效|凭证过期/);
    return true;
  });
});

test("T6 500 服务器错误抛友好错误", async () => {
  const mockFetch = makeMockFetch(async () => ({ ok: false, status: 500, async text() { return ""; } }) as any);
  const client = new OpenCodeClient({ auth: "c", fetchImpl: mockFetch });
  await assert.rejects(() => client.getUsageInfo("wrk", 0), (e: Error) => {
    assert.match(e.message, /服务器错误|服务器/);
    return true;
  });
});

test("T6 网络异常抛网络超时友好错误", async () => {
  const mockFetch = makeMockFetch(async () => { throw new Error("fetch failed"); });
  const client = new OpenCodeClient({ auth: "c", fetchImpl: mockFetch });
  await assert.rejects(() => client.getWorkspaces(), (e: Error) => {
    assert.match(e.message, /网络超时/);
    return true;
  });
});

test("T6 超时 AbortError 抛网络超时", async () => {
  const mockFetch = makeMockFetch(async () => { throw new DOMException("The operation was aborted", "AbortError"); });
  const client = new OpenCodeClient({ auth: "c", fetchImpl: mockFetch });
  await assert.rejects(() => client.getCosts("wrk", 2026, 8), (e: Error) => {
    assert.match(e.message, /网络超时/);
    return true;
  });
});

test("T6 缺少 auth 直接抛认证失效（凭证过期）", async () => {
  const mockFetch = makeMockFetch(async () => mockResponse([]) as any);
  const client = new OpenCodeClient({ auth: "", fetchImpl: mockFetch });
  await assert.rejects(() => client.getWorkspaces(), (e: Error) => {
    assert.match(e.message, /认证失效|凭证过期/);
    return true;
  });
});

test("T6 decodeStreamChunk 非 Seroval JSON 仍正常解码（鲁棒性）", () => {
  const plainJson = JSON.stringify([{ id: "plain" }]);
  // 模拟服务端偶尔直接返回 plain JSON 而非 Seroval envelope，mock 直接返回 plain text
  const data = [{ id: "plain" }];
  // decode 层已覆盖 plain JSON，客户端层亦应兼容
  const decoded = decodeStreamChunk(plainJson) as any[];
  assert.equal(decoded[0].id, "plain");
  void data;
});
