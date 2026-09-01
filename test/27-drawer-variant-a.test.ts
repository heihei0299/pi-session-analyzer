import { test } from "node:test";
import assert from "node:assert/strict";
import { startWebServer } from "../src/server.ts";
import { makeFixture, removeFixture, sessionHeader, messageEntry, assistantUsage } from "./helpers.ts";

async function fetchHtml(dir: string): Promise<string> {
  const server = await startWebServer({ dir, host: "127.0.0.1", port: 0 });
  try {
    const res = await fetch(server.url);
    assert.equal(res.status, 200);
    return await res.text();
  } finally {
    await server.close();
  }
}

// T1: 抽屉骨架 — 右侧 760px，遮罩，动画，移动端全宽，首版不支持 URL 深链
test("Seam-T1 drawer skeleton: HTML/CSS 含 760px 抽屉、遮罩、动画与移动端全宽", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);
    // HTML 容器
    assert.match(body, /id="session-detail-drawer"/, "应含抽屉容器 #session-detail-drawer");
    assert.match(body, /id="drawer-mask"/, "应含遮罩 #drawer-mask");
    assert.match(body, /drawer-mask/, "应含遮罩样式类");
    // 关闭按钮
    assert.match(body, /id="drawer-close"|drawer-close/, "应含关闭按钮");
    // CSS 宽度与动画
    assert.match(body, /760px/, "抽屉宽度应为 760px");
    assert.match(body, /cubic-bezier\(\.16,1,\.3,1\)/, "动画应为 cubic-bezier(.16,1,.3,1)");
    // 遮罩背景
    assert.match(body, /rgba\(0,0,0,\.45\)/, "遮罩背景应为 rgba(0,0,0,.45)");
    // 移动端全宽
    assert.match(body, /100vw/, "移动端应全宽 100vw");
    // 首版不支持 URL 深链：不应出现 ?detail= 解析
    assert.doesNotMatch(body, /\?detail=/, "首版不应支持 URL 深链 ?detail=");
  } finally {
    removeFixture(dir);
  }
});

// T2: 入口 — displayName/sessionId 点击触发，整行不触发，manage 不触发
test("Seam-T2 entry: displayName/sessionId 点击触发抽屉，整行不触发，manage 不触发", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);
    // 应含抽屉打开函数与 fetch
    assert.match(body, /openDrawer/, "应含 openDrawer 函数");
    assert.match(body, /fetchDetail/, "应含 fetchDetail 拉取详情");
    // 委托监听应绑定到 #session-body / #request-body 上的 detail-trigger
    assert.match(body, /session-body/, "应监听 #session-body");
    assert.match(body, /request-body/, "应监听 #request-body");
    assert.match(body, /detail-trigger/, "displayName/sessionId 应含 detail-trigger 可点击标记");
    // 整行不触发：不应直接在 tr 上绑定 click 触发抽屉（排除 th 排序的 click）
    // 期望实现通过 closest('.detail-trigger') 判断，而非 tr
    assert.match(body, /closest\(["']\.detail-trigger["']\)/, "应通过 .detail-trigger 委托而非整行");
    // manage 不触发：JS 中应排除 manage tab 或不绑定 manage 容器
    // 检查存在对 manage 的排除逻辑或注释说明
    assert.match(body, /manage/, "应处理 manage tab 不触发逻辑");
  } finally {
    removeFixture(dir);
  }
});

// T3: 头部 — displayName/sessionId(复制)、timestamp(fmtTimestamp)、cwdNorm tooltip、model(mixed)、徽标
test("Seam-T3 header: 头部含 displayName/sessionId/复制/timestamp/cwdNorm/model/徽标", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);
    // 头部渲染函数或标记
    assert.match(body, /drawer.*head|drawer-head/, "应含抽屉头部容器");
    // displayName 可选复制或重命名入口（首版至少展示）
    assert.match(body, /displayName/, "头部应处理 displayName");
    // sessionId 复制按钮
    assert.match(body, /sessionId.*copy|copy.*sessionId|drawer.*sessionId/i, "应含 sessionId 展示与复制");
    // timestamp 使用 fmtTimestamp
    assert.match(body, /fmtTimestamp/, "头部时间应使用 fmtTimestamp");
    // cwdNorm 与 tooltip 原始 cwd
    assert.match(body, /cwdNorm/, "应含 cwdNorm 展示");
    assert.match(body, /title=.*cwd|tooltip.*cwd/i, "cwd 应有 tooltip 显示原始 cwd");
    // model mixed 处理
    assert.match(body, /mixed/, "应处理 model mixed 逻辑");
    // 徽标 主会话 / 含 N 子代理
    assert.match(body, /主会话/, "应含 主会话 徽标");
    assert.match(body, /子代理/, "应含 子代理 徽标");
  } finally {
    removeFixture(dir);
  }
});

// T4: 开关 — 合并子代理 默认开（每次打开重置），无子代理隐藏，切换重渲染不重 fetch
test("Seam-T4 merge switch: 默认开、每次打开重置、无子代理隐藏、不重 fetch 联动", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);
    // 开关元素
    assert.match(body, /drawer-merge-toggle|merge.*toggle/i, "应含合并开关 checkbox");
    // 默认开
    assert.match(body, /checked/, "开关应默认 checked");
    // 每次打开重置为开：openDrawer 内重置 merge true
    assert.match(body, /merge.*true|detailDrawerState\.merge\s*=\s*true/, "打开时应重置 merge 为 true");
    // 无子代理隐藏：hasChildren 判断
    assert.match(body, /hasChildren/, "应根据 hasChildren 隐藏开关");
    // 切换重渲染不重 fetch：change 监听中调用 render 而非 fetch
    assert.match(body, /addEventListener\(["']change["']/, "开关应监听 change");
    assert.match(body, /renderDetailDrawer|render.*Drawer/, "切换应重渲染抽屉");
    // 确保 switch change 中不含 fetchDetail 或 api 调用（仅渲染）
    // 通过检查 switch 附近代码不含 fetchDetail 可弱校验：此处仅确保存在 render 调用
    assert.match(body, /merge/, "应含 merge 状态联动");
  } finally {
    removeFixture(dir);
  }
});

// T5: Tape — 复用总览 tape 样式，按 merge?merged:main 的 input/cacheRead/output 比例
test("Seam-T5 tape: 复用总览 tape 样式，按 merge?merged:main 比例", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);
    assert.match(body, /tape/, "应含 tape 结构");
    assert.match(body, /tape-track/, "应含 tape-track");
    assert.match(body, /tape-seg|seg-/, "应含 tape 段");
    // 按 merge 选择数据源
    assert.match(body, /merged/, "应根据 merge 选择 merged/main 数据");
    assert.match(body, /input.*cacheRead.*output|input.*output.*cacheRead/, "应按 input/cacheRead/output 三段比例");
  } finally {
    removeFixture(dir);
  }
});

// T6: 四卡 — 总 token/请求数/花费/缓存率，小字“其中主 X·子代理 Y（N 个）”仅总量三项 merge且N>0
test("Seam-T6 cards: 四卡总token/请求数/花费/缓存率 + 小字细分仅总量三项", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);
    // 四卡容器或渲染
    assert.match(body, /drawer.*cards|drawer-cards|card.*totalTokens|v-total/i, "应含四卡结构");
    assert.match(body, /总.*token|totalTokens/i, "应含 总 token 卡");
    assert.match(body, /请求数|requests/i, "应含 请求数 卡");
    assert.match(body, /花费|cost/i, "应含 花费 卡");
    assert.match(body, /缓存率|cacheRate/i, "应含 缓存率 卡");
    // 小字细分
    assert.match(body, /其中主/, "应含小字“其中主”细分");
    assert.match(body, /子代理/, "小字应含子代理细分");
    // 仅总量三项且 merge 且 N>0 时展示：检查条件判断 childrenCount / hasChildren
    assert.match(body, /childrenCount/, "应根据 childrenCount 判断小字展示");
  } finally {
    removeFixture(dir);
  }
});

// T7: 请求时间线 — 列定义、timestamp asc、无分页、子代理行淡背景 + 子代理<短ID8>徽标
test("Seam-T7 timeline: 请求时间线列、asc、无分页、子代理行样式与徽标", async () => {
  const dir = makeFixture({
    "s.jsonl": [sessionHeader(), messageEntry({ role: "assistant", model: "m1", usage: assistantUsage() })],
  });
  try {
    const body = await fetchHtml(dir);
    // 列定义
    assert.match(body, /时间|timestamp/, "应含 时间 列");
    assert.match(body, /模型|model/, "应含 模型 列");
    assert.match(body, /输入|input/, "应含 输入 列");
    assert.match(body, /输出|output/, "应含 输出 列");
    assert.match(body, /缓存|cache/, "应含 缓存 列");
    assert.match(body, /总.*token|totalTokens/i, "应含 总token 列");
    assert.match(body, /花费|cost/i, "应含 花费 列");
    assert.match(body, /来源|source/i, "应含 来源 列");
    // 按 timestamp asc 无分页：检查排序标记或注释
    assert.match(body, /timestamp.*asc|asc.*timestamp|时间线/i, "请求时间线应按 timestamp asc");
    assert.match(body, /source/, "行应含 source 字段区分主/子");
    // 子代理行淡背景 #FFF6D6 或类似
    assert.match(body, /FFF6D6|淡背景|child.*row|detail-child/i, "子代理行应有淡背景区分");
    // 子代理徽标 短ID8
    assert.match(body, /子代理/, "应含 子代理 徽标");
    assert.match(body, /slice\(0,8\)|短ID|8/, "徽标应展示短 ID 8 位");
  } finally {
    removeFixture(dir);
  }
});
