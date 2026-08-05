/**
 * 聚合模型：总消耗量窗口指标。
 * 统计口径（spec 决策锚）：仅 type=message && role=assistant 且携带 usage 的消息；
 * 总 token 按组件和计算；花费直接累加 cost.total；缓存率先求和分子分母再除。
 */

/** 一条计入口径的 usage（assistant 消息的 message.usage） */
export interface Usage {
  input?: unknown;
  output?: unknown;
  cacheRead?: unknown;
  cacheWrite?: unknown;
  reasoning?: unknown;
  totalTokens?: unknown;
  cost?: { total?: unknown };
}

export interface Totals {
  /** 请求数 = 带 usage 的 assistant 消息数（含全 0 失败消息） */
  requests: number;
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  reasoning: number;
  /** 总 token = input + output + cacheRead + cacheWrite（组件和） */
  totalTokens: number;
  /** 花费 = Σ usage.cost.total */
  cost: number;
  /** 缓存率 = cacheRead / (input + cacheRead + cacheWrite)，分母 0 记 0 */
  cacheRate: number;
}

export function emptyTotals(): Totals {
  return {
    requests: 0,
    input: 0,
    output: 0,
    cacheRead: 0,
    cacheWrite: 0,
    reasoning: 0,
    totalTokens: 0,
    cost: 0,
    cacheRate: 0,
  };
}

/** 累加一条计入口径的 usage（assistant 消息的 message.usage） */
export function addUsage(totals: Totals, usage: Usage): void {
  totals.requests += 1;
  totals.input += toFiniteNumber(usage.input);
  totals.output += toFiniteNumber(usage.output);
  totals.cacheRead += toFiniteNumber(usage.cacheRead);
  totals.cacheWrite += toFiniteNumber(usage.cacheWrite);
  totals.reasoning += toFiniteNumber(usage.reasoning);
  totals.cost += toFiniteNumber(usage.cost?.total);
}

/** 聚合收尾：总 token 按组件和、缓存率按分子和/分母和 */
export function finalizeTotals(totals: Totals): void {
  totals.totalTokens = totals.input + totals.output + totals.cacheRead + totals.cacheWrite;
  const denominator = totals.input + totals.cacheRead + totals.cacheWrite;
  totals.cacheRate = denominator === 0 ? 0 : totals.cacheRead / denominator;
}

/** 非有限数字一律按 0 处理（字段缺失 / undefined / null / 非数值） */
function toFiniteNumber(v: unknown): number {
  return typeof v === "number" && Number.isFinite(v) ? v : 0;
}
