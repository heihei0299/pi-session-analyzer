/**
 * 费用回算（直切 cc-switch CostCalculator）
 */
export interface Pricing {
  inputCostPerMillion: string;
  outputCostPerMillion: string;
  cacheReadCostPerMillion: string;
  cacheCreationCostPerMillion: string;
}

export interface UsageForCost {
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  cost?: { total?: number };
}

function toNum(s: string): number {
  const n = Number(s);
  return Number.isFinite(n) ? n : 0;
}

export function costForRecord(usage: UsageForCost, pricing: Pricing | null): number {
  const reported = usage.cost?.total ?? 0;
  if (reported > 0) return reported;
  if (!pricing) return 0;
  const input = usage.input * toNum(pricing.inputCostPerMillion) / 1e6;
  const output = usage.output * toNum(pricing.outputCostPerMillion) / 1e6;
  const cacheRead = usage.cacheRead * toNum(pricing.cacheReadCostPerMillion) / 1e6;
  const cacheWrite = usage.cacheWrite * toNum(pricing.cacheCreationCostPerMillion) / 1e6;
  const total = input + output + cacheRead + cacheWrite;
  return total;
}
