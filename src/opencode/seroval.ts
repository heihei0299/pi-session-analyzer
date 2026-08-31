/**
 * Seroval 最小可用子集：SolidStart RPC 序列化/反序列化
 * 格式: { t: { t:9, i:0, l:N, a:[...] }, f:31, m:[] } — 与 spec 一致（spec 描述的 envelope）
 * 若无法 100% 复刻真实 SolidStart 细节，至少保证本项目 encode/decode 自洽且 mock 响应可验证。
 */

export function encodePayload(args: unknown[]): string {
  return JSON.stringify({ t: { t: 9, i: 0, l: args.length, a: args }, f: 31, m: [] });
}

/**
 * 解析 chunked 响应流中的单 chunk，提取数据对象。
 * 支持：
 *  - 纯 JSON
 *  - SSE 前缀 "data: ..."
 *  - Seroval envelope {t:{t:9,i:0,l:N,a:[...]},f:31,m:[]} → 自动 unwrap a
 *  - 非 envelope 的 plain object/array → 原样返回
 */
export function decodeStreamChunk(chunk: string): unknown {
  let text = chunk.trim();
  if (text === "") return null;
  if (text.startsWith("data:")) text = text.slice(5).trim();

  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    // 尝试截取首尾 JSON 子串（容忍多余前缀/换行包裹）
    const start = text.indexOf("{");
    const end = text.lastIndexOf("}");
    const startArr = text.indexOf("[");
    let jsonStart = -1;
    let jsonEnd = -1;
    // 选择最早的 { 或 [
    if (start !== -1 && startArr !== -1) jsonStart = Math.min(start, startArr);
    else jsonStart = start !== -1 ? start : startArr;
    const endObj = text.lastIndexOf("}");
    const endArr = text.lastIndexOf("]");
    jsonEnd = Math.max(endObj, endArr);
    if (jsonStart !== -1 && jsonEnd !== -1 && jsonEnd > jsonStart) {
      try {
        parsed = JSON.parse(text.slice(jsonStart, jsonEnd + 1));
      } catch {
        return null;
      }
    } else {
      return null;
    }
  }

  // unwrap seroval envelope
  if (parsed !== null && typeof parsed === "object" && !Array.isArray(parsed)) {
    const obj = parsed as Record<string, unknown>;
    const t = obj.t as Record<string, unknown> | undefined;
    if (t !== null && typeof t === "object" && !Array.isArray(t) && "a" in t && Array.isArray((t as any).a)) {
      const a = (t as any).a as unknown[];
      const l = (t as any).l as number | undefined;
      // 若 l === 1，a[0] 即为业务数据；若 l===0 返回 null；多参数返回整个 a
      if (l === 0) return null;
      if (l === 1) return a[0];
      return a;
    }
  }
  return parsed;
}

/** 辅助：解码整段响应文本（可能多行 chunk）→ 合并首个非空数据 */
export function decodeResponseText(text: string): unknown {
  const trimmed = text.trim();
  if (trimmed === "") return null;
  // 按行拆分，逐块解码，取最后非空结果（与 SolidStart 串流最后更新为准）
  const lines = trimmed.split("\n").map((l) => l.trim()).filter((l) => l.length > 0);
  let last: unknown = null;
  let found = false;
  for (const line of lines) {
    const v = decodeStreamChunk(line);
    if (v !== null && v !== undefined) {
      last = v;
      found = true;
    }
  }
  if (found) return last;
  // 若无换行但整体可解析
  return decodeStreamChunk(trimmed);
}
