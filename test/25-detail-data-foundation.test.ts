import { test } from "node:test";
import assert from "node:assert/strict";
import { join } from "node:path";
import { SessionData } from "../src/session-data.ts";
import { startWebServer } from "../src/server.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

// Seam-1: SessionFileData.parentSessionId 解析
test("Seam-1 parentSessionId 解析：header 含非空 parentSession 时提取，缺失/空串/非字符串时为 undefined", async () => {
  const sd = new SessionData();
  const dir = makeFixture({
    "2026-08-01T10-00-00-000Z_parent.jsonl": [
      sessionHeader({ id: "p1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 10 }) }),
    ],
    "2026-08-02T10-00-00-000Z_child.jsonl": [
      { ...sessionHeader({ id: "c1", timestamp: "2026-08-02T10:00:00.000Z", cwd: "/proj" }), parentSession: "p1" },
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 20 }) }),
    ],
    "2026-08-02T11-00-00-000Z_empty.jsonl": [
      { ...sessionHeader({ id: "e1", timestamp: "2026-08-02T11:00:00.000Z", cwd: "/proj" }), parentSession: "" },
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() }),
    ],
    "2026-08-02T12-00-00-000Z_num.jsonl": [
      { ...sessionHeader({ id: "n1", timestamp: "2026-08-02T12:00:00.000Z", cwd: "/proj" }), parentSession: 123 as unknown as string },
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() }),
    ],
  });
  try {
    const files = await sd.readSessionFiles(dir);
    const byId = new Map(files.map((f) => [f.sessionId, f]));
    assert.equal(byId.get("p1")!.parentSessionId, undefined, "缺失时 undefined");
    assert.equal(byId.get("c1")!.parentSessionId, "p1", "非空字符串提取");
    assert.equal(byId.get("e1")!.parentSessionId, undefined, "空串视为缺失");
    assert.equal(byId.get("n1")!.parentSessionId, undefined, "非字符串视为缺失");
    // isTask 仍按路径判断（此处文件均不在 /tasks/ 下，故 false），注释应已更新为“子代理 (isTask)”
  } finally {
    removeFixture(dir);
  }
});

// Seam-2: 归属判定 & 孤儿过滤
test("Seam-2 归属：children=filter(parentSessionId===parent.sessionId)，孤儿不归入", async () => {
  const sd = new SessionData();
  // 构造文件数组（不依赖磁盘，直接构造 SessionFileData）
  const files = [
    { sessionId: "p1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj", fileName: "p1.jsonl", isTask: false, items: [{ timestamp: "2026-08-01T10:01:00.000Z", model: "m1", usage: { input: 10, output: 5, cacheRead: 0, cacheWrite: 0, cost: { total: 0.01 } } }] },
    { sessionId: "c1", timestamp: "2026-08-02T10:00:00.000Z", cwd: "/proj", fileName: "c1.jsonl", isTask: true, parentSessionId: "p1", items: [{ timestamp: "2026-08-02T10:01:00.000Z", model: "m1", usage: { input: 20, output: 10, cacheRead: 0, cacheWrite: 0, cost: { total: 0.02 } } }] },
    { sessionId: "c2", timestamp: "2026-08-02T11:00:00.000Z", cwd: "/proj", fileName: "c2.jsonl", isTask: true, parentSessionId: "p1", items: [{ timestamp: "2026-08-02T11:01:00.000Z", model: "m1", usage: { input: 30, output: 15, cacheRead: 0, cacheWrite: 0, cost: { total: 0.03 } } }] },
    // 孤儿：指向不存在的 parent
    { sessionId: "orphan", timestamp: "2026-08-03T10:00:00.000Z", cwd: "/proj", fileName: "orphan.jsonl", isTask: true, parentSessionId: "missing", items: [{ timestamp: "2026-08-03T10:01:00.000Z", model: "m1", usage: { input: 40, output: 20, cacheRead: 0, cacheWrite: 0, cost: { total: 0.04 } } }] },
    // 无关会话
    { sessionId: "other", timestamp: "2026-08-04T10:00:00.000Z", cwd: "/proj", fileName: "other.jsonl", isTask: false, items: [{ timestamp: "2026-08-04T10:01:00.000Z", model: "m1", usage: { input: 50, output: 25, cacheRead: 0, cacheWrite: 0, cost: { total: 0.05 } } }] },
  ] as unknown as import("../src/session-data.ts").SessionFileData[];
  const detail: any = (sd as any).detailFromFiles(files, "p1");
  assert.equal(detail.session.sessionId, "p1");
  assert.equal(detail.children.length, 2, "仅 c1,c2 归属 p1");
  assert.deepEqual(detail.children.map((c:any)=>c.sessionId).sort(), ["c1","c2"]);
  assert.equal(detail.totals.childrenCount, 2);
  assert.equal(detail.meta.hasChildren, true);
  // 孤儿查询：不存在归属
  const otherDetail: any = (sd as any).detailFromFiles(files, "other");
  assert.equal(otherDetail.children.length, 0, "other 无子");
  assert.equal(otherDetail.meta.hasChildren, false);
  // 验证孤儿的 parentSessionId 不会导致被误归入 p1
  assert.ok(!detail.children.some((c:any)=>c.sessionId==="orphan"));
});

// Seam-3: 合并 totals（totalTokens = input+cacheRead+output、cacheRate 重算）
test("Seam-3 合并 totals：merged = main+children 求和后 finalize，totalTokens 与 cacheRate 重算", async () => {
  const sd = new SessionData();
  const files = [
    {
      sessionId: "p1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj", fileName: "p1.jsonl", isTask: false,
      items: [
        { timestamp: "2026-08-01T10:01:00.000Z", model: "m1", usage: { input: 100, output: 50, cacheRead: 200, cacheWrite: 5, reasoning: 10, cost: { total: 0.1 } } },
        { timestamp: "2026-08-01T10:02:00.000Z", model: "m1", usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, cost: { total: 0 } } },
      ],
    },
    {
      sessionId: "c1", timestamp: "2026-08-02T10:00:00.000Z", cwd: "/proj", fileName: "c1.jsonl", isTask: true, parentSessionId: "p1",
      items: [
        { timestamp: "2026-08-02T10:01:00.000Z", model: "m1", usage: { input: 30, output: 20, cacheRead: 10, cacheWrite: 2, reasoning: 5, cost: { total: 0.05 } } },
      ],
    },
    {
      sessionId: "c2", timestamp: "2026-08-02T11:00:00.000Z", cwd: "/proj", fileName: "c2.jsonl", isTask: true, parentSessionId: "p1",
      items: [
        { timestamp: "2026-08-02T11:01:00.000Z", model: "m1", usage: { input: 70, output: 30, cacheRead: 90, cacheWrite: 3, reasoning: 15, cost: { total: 0.08 } } },
      ],
    },
  ] as unknown as import("../src/session-data.ts").SessionFileData[];
  const detail: any = (sd as any).detailFromFiles(files, "p1");
  // main = p1 only: input 100, cacheRead 200, output 50 => totalTokens 350, requests 2
  assert.equal(detail.totals.main.input, 100);
  assert.equal(detail.totals.main.cacheRead, 200);
  assert.equal(detail.totals.main.output, 50);
  assert.equal(detail.totals.main.totalTokens, 350, "totalTokens = input+cacheRead+output (ADR-0002)");
  assert.equal(detail.totals.main.requests, 2);
  assert.equal(detail.totals.main.cacheWrite, 5);
  assert.equal(detail.totals.main.reasoning, 10);
  assert.equal(detail.totals.main.cost, 0.1);
  // cacheRate = cacheRead/(input+cacheRead) = 200/300 = 0.666...
  assert.ok(Math.abs(detail.totals.main.cacheRate - 200/300) < 1e-9);
  // merged = p1 + c1 + c2: input 200, cacheRead 300, output 100 => totalTokens 600
  assert.equal(detail.totals.merged.input, 200, "merged input =100+30+70");
  assert.equal(detail.totals.merged.cacheRead, 300, "merged cacheRead 200+10+90");
  assert.equal(detail.totals.merged.output, 100, "merged output 50+20+30");
  assert.equal(detail.totals.merged.totalTokens, 600);
  assert.equal(detail.totals.merged.requests, 4, "merged requests 2+1+1");
  assert.equal(detail.totals.merged.cacheWrite, 10, "5+2+3");
  assert.equal(detail.totals.merged.reasoning, 30, "10+5+15");
  assert.ok(Math.abs(detail.totals.merged.cost - 0.23) < 1e-9, "0.1+0.05+0.08");
  assert.ok(Math.abs(detail.totals.merged.cacheRate - 300/500) < 1e-9, "cacheRate 重算 300/(200+300)=0.6");
  assert.equal(detail.totals.childrenCount, 2);
  // 无子代理时 merged === main
  const solo = (sd as any).detailFromFiles([
    { sessionId: "solo", timestamp: "2026-08-05T10:00:00.000Z", cwd: "/proj", fileName: "solo.jsonl", isTask: false, items: [{ timestamp: "2026-08-05T10:01:00.000Z", model: "m1", usage: { input: 10, output: 5, cacheRead: 2, cacheWrite: 0, cost: { total: 0.01 } } }] },
  ] as any, "solo");
  assert.deepEqual(solo.totals.merged, solo.totals.main, "无子时 merged 等于 main");
  assert.equal(solo.totals.childrenCount, 0);
  assert.equal(solo.meta.hasChildren, false);
});

// Seam-4: 请求混排与 source 标记
test("Seam-4 请求混排：按 timestamp asc 排序，source/sourceSessionId/displayName 正确", async () => {
  const sd = new SessionData();
  const files = [
    {
      sessionId: "p1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj/alpha", fileName: "2026-08-01T10-00-00_p1.jsonl", isTask: false, firstUserText: "parent topic",
      items: [
        { timestamp: "2026-08-01T10:00:00.000Z", model: "m1", usage: { input: 10, output: 5, cacheRead: 0, cacheWrite: 0, cost: { total: 0.01 } } },
        { timestamp: "2026-08-01T10:10:00.000Z", model: "m1", usage: { input: 20, output: 10, cacheRead: 0, cacheWrite: 0, cost: { total: 0.02 } } },
      ],
    },
    {
      sessionId: "c1", timestamp: "2026-08-01T10:05:00.000Z", cwd: "/proj/alpha", fileName: "2026-08-01T10-05-00_c1.jsonl", isTask: true, parentSessionId: "p1", firstUserText: "child topic",
      items: [
        { timestamp: "2026-08-01T10:05:00.000Z", model: "m2", usage: { input: 30, output: 15, cacheRead: 0, cacheWrite: 0, cost: { total: 0.03 } } },
        { timestamp: "2026-08-01T10:15:00.000Z", model: "m2", usage: { input: 40, output: 20, cacheRead: 0, cacheWrite: 0, cost: { total: 0.04 } } },
      ],
    },
  ] as unknown as import("../src/session-data.ts").SessionFileData[];
  const detail: any = (sd as any).detailFromFiles(files, "p1");
  assert.equal(detail.requests.length, 4, "主2+子2=4");
  // 排序：10:00 main, 10:05 child, 10:10 main, 10:15 child
  const ts = detail.requests.map((r:any)=>r.timestamp);
  assert.deepEqual(ts, ["2026-08-01T10:00:00.000Z","2026-08-01T10:05:00.000Z","2026-08-01T10:10:00.000Z","2026-08-01T10:15:00.000Z"]);
  assert.deepEqual(detail.requests.map((r:any)=>r.source), ["main","child","main","child"]);
  assert.deepEqual(detail.requests.map((r:any)=>r.sourceSessionId), ["p1","c1","p1","c1"]);
  // displayName: 主请求应为 parent topic 的 displayName，子为 child topic 逻辑
  // 文件名为时间戳格式，故 displayName 取 firstUserText
  assert.equal(detail.requests[0].displayName, "parent topic");
  assert.equal(detail.requests[1].displayName, "child topic");
  assert.equal(detail.requests[2].displayName, "parent topic");
  assert.equal(detail.requests[3].displayName, "child topic");
  // 请求行的 model 与 totalTokens 透传
  assert.equal(detail.requests[0].model, "m1");
  assert.equal(detail.requests[1].model, "m2");
});

// Seam-5: 404 不存在会话
test("Seam-5 404：detailFromFiles/sessionId 不存在时抛 404，queryDetail 同样", async () => {
  const sd = new SessionData();
  const files = [
    { sessionId: "p1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj", fileName: "p1.jsonl", isTask: false, items: [] },
  ] as unknown as import("../src/session-data.ts").SessionFileData[];
  let threw = false;
  try {
    (sd as any).detailFromFiles(files, "missing");
  } catch (e:any) {
    threw = true;
    assert.equal(e.status, 404);
    assert.match(e.message, /会话不存在/);
  }
  assert.equal(threw, true, "应抛 404");

  // queryDetail 对真实目录的 404（通过临时 fixture）
  const dir = makeFixture({
    "2026-08-01T10-00-00-000Z_p1.jsonl": [
      sessionHeader({ id: "p1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/proj" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 10 }) }),
    ],
  });
  try {
    let threw2 = false;
    try {
      await (sd as any).queryDetail(dir, "nope");
    } catch (e:any) {
      threw2 = true;
      assert.equal(e.status, 404);
    }
    assert.equal(threw2, true, "queryDetail 不存在应抛 404");
    // 存在时不抛
    const ok = await (sd as any).queryDetail(dir, "p1");
    assert.equal(ok.session.sessionId, "p1");
    assert.equal(ok.meta.hasChildren, false);
  } finally {
    removeFixture(dir);
  }
});

// Seam-6: GET /api/sessions/:id/detail 端点
test("Seam-6 GET /api/sessions/:id/detail 返回 {session,children,totals,requests,meta}，复用 serialize，404/400 分支", async () => {
  const dir = makeFixture({
    // 主会话 p1：2 条请求（timestamp 顺序 10:00/10:10）
    // 主会话 p1：2 条请求（timestamp 顺序 10:00/10:10，第二条为失败重试，门控保留）
    "2026-08-01T10-00-00-000Z_p1.jsonl": [
      sessionHeader({ id: "p1", timestamp: "2026-08-01T10:00:00.000Z", cwd: "/home/shial/Project/alpha" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 100, output: 50, cacheRead: 200, cacheWrite: 5, cost: { total: 0.1 } }) }, { timestamp: "2026-08-01T10:00:00.000Z" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 0, output: 0, cacheRead: 0, cacheWrite: 0, cost: { total: 0 } }), stopReason: "error" }, { timestamp: "2026-08-01T10:10:00.000Z" }),
    ],
    // 子会话 c1 归属 p1
    "2026-08-02T10-00-00-000Z_c1.jsonl": [
      { ...sessionHeader({ id: "c1", timestamp: "2026-08-02T10:00:00.000Z", cwd: "/home/shial/Project/alpha" }), parentSession: "p1" },
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 30, output: 20, cacheRead: 10, cacheWrite: 2, cost: { total: 0.05 } }) }, { timestamp: "2026-08-01T10:05:00.000Z" }),
    ],
    // 孤儿 orphan 指向 missing，不应被 p1 合并
    "2026-08-03T10-00-00-000Z_orph.jsonl": [
      { ...sessionHeader({ id: "orphan", timestamp: "2026-08-03T10:00:00.000Z", cwd: "/home/shial/Project/alpha" }), parentSession: "missing" },
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 999, output: 999, cacheRead: 999, cacheWrite: 0, cost: { total: 9.99 } }) }, { timestamp: "2026-08-03T10:00:00.000Z" }),
    ],
    // 无子代理的独立会话 solo
    "2026-08-04T10-00-00-000Z_solo.jsonl": [
      sessionHeader({ id: "solo", timestamp: "2026-08-04T10:00:00.000Z", cwd: "/home/shial/Project/beta" }),
      messageEntry({ role: "assistant", model: "m1", usage: assistantUsage({ input: 10, output: 5, cacheRead: 2, cacheWrite: 0, cost: { total: 0.01 } }) }, { timestamp: "2026-08-04T10:00:00.000Z" }),
    ],
  });
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    // 正常：p1 含子代理
    const res = await fetch(new URL("/api/sessions/p1/detail", server.url));
    assert.equal(res.status, 200, "p1 detail 200");
    const body = await res.json() as any;
    // 顶层键
    assert.ok("session" in body && "children" in body && "totals" in body && "requests" in body && "meta" in body);
    // session enriched 含 displayName/fileName/cwdNorm/isTask 及 totals 字段
    assert.equal(body.session.sessionId, "p1");
    assert.equal(typeof body.session.displayName, "string");
    assert.equal(typeof body.session.fileName, "string");
    assert.equal(typeof body.session.cwdNorm, "string");
    assert.equal(body.session.isTask, false);
    for (const k of ["requests","input","output","cacheRead","cacheWrite","reasoning","totalTokens","cost","cacheRate"]) {
      assert.ok(k in body.session, `session 含指标 ${k}`);
    }
    // children
    assert.equal(body.children.length, 1);
    assert.equal(body.children[0].sessionId, "c1");
    assert.equal(body.children[0].isTask, false, "c1 文件不在 /tasks/ 下，isTask false（归属仅看 parentSessionId）");
    assert.equal(typeof body.children[0].displayName, "string");
    // totals：main vs merged 需不同，childrenCount 与 meta
    assert.equal(body.totals.childrenCount, 1);
    assert.equal(body.meta.hasChildren, true);
    assert.equal(body.totals.main.input, 100, "main input 100 (+0)");
    assert.equal(body.totals.merged.input, 130, "merged input 100+30");
    assert.equal(body.totals.merged.totalTokens, 130+210+70, "merged totalTokens input+cacheRead+output: p1(100+200+50) + c1(30+10+20)=410");
    assert.ok(body.totals.merged.requests > body.totals.main.requests, "merged requests > main");
    // requests 混排 + source 标记
    assert.equal(body.requests.length, 3, "p1两条 + c1一条=3");
    // 已按 timestamp asc
    const ts = body.requests.map((r:any)=>r.timestamp);
    const sorted = [...ts].sort((a,b)=>a.localeCompare(b));
    assert.deepEqual(ts, sorted, "requests 按 timestamp asc");
    // source 标记
    const mainCount = body.requests.filter((r:any)=>r.source==="main").length;
    const childCount = body.requests.filter((r:any)=>r.source==="child").length;
    assert.equal(mainCount, 2);
    assert.equal(childCount, 1);
    for (const r of body.requests) {
      assert.ok(r.source === "main" || r.source === "child");
      assert.equal(typeof r.sourceSessionId, "string");
      assert.equal(typeof r.displayName, "string");
      // request 行仍含 totals 指标
      for (const k of ["input","output","cacheRead","cacheWrite","reasoning","totalTokens","cost","cacheRate"]) {
        assert.ok(k in r, `request 含 ${k}`);
      }
    }
    // 孤儿不应影响 p1 的 merged
    assert.ok(!body.requests.some((r:any)=>r.input===999), "孤儿 999 不应混入 p1");

    // 无子代理场景：solo
    const resSolo = await fetch(new URL("/api/sessions/solo/detail", server.url));
    assert.equal(resSolo.status, 200);
    const soloBody = await resSolo.json() as any;
    assert.equal(soloBody.children.length, 0);
    assert.equal(soloBody.totals.childrenCount, 0);
    assert.equal(soloBody.meta.hasChildren, false);
    assert.deepEqual(soloBody.totals.merged, soloBody.totals.main, "无子时 merged==main");
    assert.equal(soloBody.requests.length, 1);
    assert.equal(soloBody.requests[0].source, "main");

    // 404：不存在
    const res404 = await fetch(new URL("/api/sessions/missing/detail", server.url));
    assert.equal(res404.status, 404);
    const b404 = await res404.json() as any;
    assert.ok(typeof b404.error === "string" && b404.error.length>0);
    assert.ok(typeof b404.detail === "string" && b404.detail.length>0);

    // 400：空 id（路径 /api/sessions//detail 或 query 版缺失）
    const res400a = await fetch(new URL("/api/sessions/detail?sessionId=", server.url));
    // 若实现了 query 版，则应 400；若未实现 query 版则 404；此处断言 400 或 404 均可，但优先 400
    // 我们强制 query 版必须 400
    assert.equal(res400a.status, 400, "空 sessionId 应 400");
    const b400a = await res400a.json() as any;
    assert.ok(typeof b400a.error === "string");

    // 兼容 query 版：GET /api/sessions/detail?sessionId=p1 同 REST
    const resQ = await fetch(new URL("/api/sessions/detail?sessionId=p1", server.url));
    assert.equal(resQ.status, 200);
    const bQ = await resQ.json() as any;
    assert.equal(bQ.session.sessionId, "p1");
    assert.equal(bQ.totals.childrenCount, 1);
  } finally {
    await server.close();
    removeFixture(dir);
  }
});
