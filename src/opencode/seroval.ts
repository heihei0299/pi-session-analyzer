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
  // 1) 优先处理 SolidStart JS chunk（self.$R）—— /_server 成功响应为 JS 而非纯 JSON，需 JS 求值提取
  if (trimmed.includes("self.$R") || trimmed.includes("$R[0]=")) {
    try {
      const dataStart = trimmed.indexOf("$R[0]=");
      if (dataStart !== -1) {
        const after = trimmed.slice(dataStart + 6);
        const endIdx = after.indexOf(")($R");
        if (endIdx !== -1) {
          const dataStr = after.slice(0, endIdx);
          // dataStr 可能是 `[$R[1]={id:...},$R[2]={...}]` 或 `{usage:[...],keys:[...]}` 等 JS 字面量，需在 $R 上下文中求值
          // 使用 Function 在受控 $R 数组上求值，避免直接 eval 全局污染
          const $R: unknown[] = [];
          // eslint-disable-next-line no-new-func
          const fn = new Function("$R", `return ${dataStr}`);
          const result = fn($R);
          // 对于 $R[1]= 形式，$R 数组本身也可能被填充，若 result 为数组且含 undefined 首位，需过滤
          if (Array.isArray(result)) {
            // 若数组含 $R 赋值产生的稀疏，取非空
            const compact = result.filter((v) => v !== undefined && v !== null);
            // 若 compact 为空但 $R[1..] 有值，回退到 $R
            if (compact.length === 0 && ($R as unknown[]).length > 1) {
              const fromR = ($R as unknown[]).slice(1).filter((v) => v !== undefined && v !== null);
              if (fromR.length > 0) return fromR.length === 1 ? fromR[0] : fromR;
            }
            return compact.length > 0 ? compact : result;
          }
          if (result !== undefined && result !== null) return result;
          // 回退：若 result 为空但 $R 有值
          if (($R as unknown[]).length > 1) {
            const fromR = ($R as unknown[]).slice(1).filter((v) => v !== undefined);
            if (fromR.length === 1) return fromR[0];
            if (fromR.length > 1) return fromR;
          }
        }
      }
    } catch {
      // 忽略，走后续 JSON 分支
    }
    // 兼容旧 HTML 嵌入的 usage.list 场景（非 _server，直接是 HTML 中的 $R 初始化），尝试提取 usage.list 的 JS 数组
    // 此分支已在 _server 成功时覆盖，剩余情况走 JSON
  }
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
