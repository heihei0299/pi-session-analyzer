import { encodePayload, decodeResponseText } from "./seroval.ts";
import type { OpenCodeCostsResult, OpenCodeUsageRecord, WorkspaceInfo } from "./types.ts";

const RPC_URL = "https://opencode.ai/_server";
const FN = {
  workspaces: "def39973159c7f0483d8793a822b8dbb10d067e12c65455fcb4608459ba0234f",
  monthlyCosts: "15702f3a12ff8bff357f8c2aa154a17e65b746d5f6b96adc9002c86ee0c15205",
  usageHistory: "bfd684bfc2e4eed05cd0b518f5e4eafd3f3376e3938abb9e536e7c03df831e5c",
} as const;

let rpcInstance = 0;
type FetchImpl = typeof fetch;

export class OpenCodeClient {
  private auth: string;
  private fetchImpl: FetchImpl;

  constructor(authOrOpts: string | { auth: string; fetchImpl?: FetchImpl }, fetchImpl?: FetchImpl) {
    if (typeof authOrOpts === "string") {
      this.auth = authOrOpts;
      this.fetchImpl = (fetchImpl ?? (globalThis.fetch as unknown as FetchImpl));
    } else {
      this.auth = authOrOpts.auth;
      this.fetchImpl = (authOrOpts.fetchImpl ?? (globalThis.fetch as unknown as FetchImpl));
    }
    if (!this.auth) {
      // 允许空 auth 构造，调用时再抛友好错误（便于测试 auth 校验）
    }
  }

  private async rpc(functionId: string, args: unknown[]): Promise<unknown> {
    if (!this.auth) {
      throw new Error("认证失效: 缺少 OpenCode auth，请设置 OPENCODE_AUTH 或传入 --auth（凭证过期/缺失）");
    }
    const rawAuth = this.auth.trim();
    // .zshrc 中 OPENCODE_AUTH 可能是 "auth=Fe26..." 或纯 token，兼容两种；同时支持 "auth=...; oc_locale=zh" 的完整 cookie 串
    const cookieBase = rawAuth.startsWith("auth=") || rawAuth.includes("auth=") ? rawAuth : `auth=${rawAuth}`;
    // 若单独的 oc_locale 环境变量存在且 cookie 中未包含，则追加（SolidStart 需 oc_locale cookie）
    const ocLocale = (typeof process !== "undefined" ? (process.env as Record<string, string | undefined>).oc_locale ?? (process.env as Record<string, string | undefined>).OC_LOCALE : undefined);
    const cookieHeader = ocLocale && !cookieBase.includes("oc_locale") ? `${cookieBase}; oc_locale=${ocLocale}` : cookieBase;
    // hermes 实证：usage/costs 用 GET ?id&args=[WRK,page] + X-Server-Id/Instance + Referer workspace/usage，才能翻页到 150+
    const isUsageOrCosts = functionId === FN.usageHistory || functionId === FN.monthlyCosts;
    if (isUsageOrCosts) {
      const jsonArgs = JSON.stringify(args);
      const getUrl = `${RPC_URL}?id=${encodeURIComponent(functionId)}&args=${encodeURIComponent(jsonArgs)}`;
      const headers: Record<string, string> = {
        "cookie": cookieHeader,
        "X-Server-Id": functionId,
        "X-Server-Instance": `server-fn:${functionId === FN.monthlyCosts ? 0 : 1}`,
        "Referer": `https://opencode.ai/workspace/${String(args[0] ?? "")}/usage`,
        "Accept": "application/json",
        "User-Agent": "Mozilla/5.0",
      };
      const doFetch = async (method: string, url: string, bodyOrNull: string | null, h: Record<string, string>): Promise<Response> => {
        const signal = typeof AbortSignal !== "undefined" && typeof (AbortSignal as any).timeout === "function" ? (AbortSignal as any).timeout(15_000) as AbortSignal : undefined;
        const init: RequestInit = { method, headers: h, signal } as RequestInit;
        if (bodyOrNull !== null) (init as Record<string, unknown>).body = bodyOrNull;
        return (this.fetchImpl as unknown as (url: string, init: RequestInit) => Promise<Response>)(url, init);
      };
      let res: Response;
      try {
        res = await doFetch("GET", getUrl, null, headers);
      } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        if (e instanceof DOMException && (e.name === "AbortError" || e.name === "TimeoutError")) throw new Error(`网络超时: 请求 OpenCode 超时（15s），请检查网络或稍后重试 — ${msg}`);
        if (msg.toLowerCase().includes("abort") || msg.toLowerCase().includes("timeout")) throw new Error(`网络超时: 请求 OpenCode 超时，请检查网络 — ${msg}`);
        throw new Error(`网络超时: 网络请求失败 — ${msg}`);
      }
      if (!res.ok) {
        if (res.status === 401 || res.status === 403) throw new Error(`认证失效: OpenCode 凭证已过期或无效（HTTP ${res.status}），请刷新 auth cookie（凭证过期）`);
        if (res.status === 404) throw new Error(`请求失败: OpenCode 返回 HTTP 404（Function ID 可能已随前端发版更换，或工作区不存在）。请先确认 OPENCODE_AUTH/OPENCODE_WORKSPACE_ID 已配置；若已配置仍 404，需更新 src/opencode/client.ts 中 3 个 FN 哈希（见 spec Further Notes）`);
        if (res.status >= 500) throw new Error(`服务器错误: OpenCode 服务异常（HTTP ${res.status}），请稍后重试`);
        throw new Error(`请求失败: OpenCode 返回 HTTP ${res.status}`);
      }
      const text = await (res as unknown as { text: () => Promise<string> }).text();
      return decodeResponseText(text);
    }
    const body = encodePayload(args);
    const headers: Record<string, string> = {
      "content-type": "application/json",
      "cookie": cookieHeader,
      "X-Server-Id": functionId,
      "X-Server-Instance": `server-fn:${rpcInstance++}`,
      "Referer": "https://opencode.ai/",
      "Origin": "https://opencode.ai",
      "Accept": "*/*",
    };
    let res: Response;
    const doFetch = async (method: string, url: string, bodyOrNull: string | null, headers: Record<string, string>): Promise<Response> => {
      const signal = typeof AbortSignal !== "undefined" && typeof (AbortSignal as any).timeout === "function"
        ? (AbortSignal as any).timeout(15_000) as AbortSignal
        : undefined;
      const init: RequestInit = { method, headers, signal } as RequestInit;
      if (bodyOrNull !== null) (init as Record<string, unknown>).body = bodyOrNull;
      return (this.fetchImpl as unknown as (url: string, init: RequestInit) => Promise<Response>)(url, init);
    };
    try {
      res = await doFetch("POST", RPC_URL, body, headers);
      if (!res.ok && res.status === 500) {
        const getUrl = `${RPC_URL}?id=${encodeURIComponent(functionId)}&args=${encodeURIComponent(body)}`;
        const getHeaders: Record<string, string> = {
          "cookie": headers["cookie"],
          "X-Server-Id": headers["X-Server-Id"],
          "X-Server-Instance": `server-fn:${rpcInstance++}`,
        };
        const retry = await doFetch("GET", getUrl, null, getHeaders);
        // 仅当 GET 成功时采用，否则保留原 500 供上层统一抛错
        if (retry.ok) res = retry;
      }
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      // AbortError / TimeoutError 归为网络超时
      if (e instanceof DOMException && (e.name === "AbortError" || e.name === "TimeoutError")) {
        throw new Error(`网络超时: 请求 OpenCode 超时（15s），请检查网络或稍后重试 — ${msg}`);
      }
      if (msg.toLowerCase().includes("abort") || msg.toLowerCase().includes("timeout")) {
        throw new Error(`网络超时: 请求 OpenCode 超时，请检查网络 — ${msg}`);
      }
      throw new Error(`网络超时: 网络请求失败 — ${msg}`);
    }

    if (!res.ok) {
      if (res.status === 401 || res.status === 403) {
        throw new Error(`认证失效: OpenCode 凭证已过期或无效（HTTP ${res.status}），请刷新 auth cookie（凭证过期）`);
      }
      if (res.status === 404) {
        throw new Error(`请求失败: OpenCode 返回 HTTP 404（Function ID 可能已随前端发版更换，或工作区不存在）。请先确认 OPENCODE_AUTH/OPENCODE_WORKSPACE_ID 已配置；若已配置仍 404，需更新 src/opencode/client.ts 中 3 个 FN 哈希（见 spec Further Notes）`);
      }
      if (res.status >= 500) {
        throw new Error(`服务器错误: OpenCode 服务异常（HTTP ${res.status}），请稍后重试`);
      }
      throw new Error(`请求失败: OpenCode 返回 HTTP ${res.status}`);
    }

    const text = await (res as any).text();
    const decoded = decodeResponseText(text);
    return decoded;
  }

  async getWorkspaces(): Promise<WorkspaceInfo[]> {
    const data = await this.rpc(FN.workspaces, []);
    if (data == null) return [];
    if (Array.isArray(data)) return data as WorkspaceInfo[];
    if (typeof data === "object" && data !== null) {
      const obj = data as Record<string, unknown>;
      if (Array.isArray(obj.workspaces)) return obj.workspaces as WorkspaceInfo[];
      if (Array.isArray(obj.data)) return obj.data as WorkspaceInfo[];
    }
    return [];
  }

  async getCosts(workspaceId: string, year: number, month: number, tzOffset?: number): Promise<OpenCodeCostsResult> {
    const args: unknown[] = tzOffset !== undefined ? [workspaceId, year, month, tzOffset] : [workspaceId, year, month];
    try {
      const data = await this.rpc(FN.monthlyCosts, args);
      if (data == null) return { usage: [], keys: [] };
      if (typeof data === "object" && data !== null) {
        const obj = data as Record<string, unknown>;
        const usage = Array.isArray(obj.usage) ? obj.usage as OpenCodeCostsResult["usage"] : [];
        const keys = Array.isArray(obj.keys) ? obj.keys as OpenCodeCostsResult["keys"] : [];
        return { usage, keys };
      }
      return { usage: [], keys: [] };
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      if (msg.includes("网络超时") || msg.includes("认证失效")) throw e;
      // 成本聚合在部分环境下 via _server 500（HTTPError），降级为空，不阻断同步/WebUI
      return { usage: [], keys: [] };
    }
  }

  private async fetchHtml(path: string): Promise<string> {
    const rawAuth = this.auth.trim();
    const cookieBase = rawAuth.startsWith("auth=") || rawAuth.includes("auth=") ? rawAuth : `auth=${rawAuth}`;
    const ocLocale = (typeof process !== "undefined" ? (process.env as Record<string, string | undefined>).oc_locale ?? (process.env as Record<string, string | undefined>).OC_LOCALE : undefined);
    const cookieHeader = ocLocale && !cookieBase.includes("oc_locale") ? `${cookieBase}; oc_locale=${ocLocale}` : cookieBase;
    const url = `https://opencode.ai${path}`;
    const res = await (this.fetchImpl as unknown as (url: string, init: RequestInit) => Promise<Response>)(url, {
      method: "GET",
      headers: { cookie: cookieHeader, "user-agent": "Mozilla/5.0" },
    });
    if (!res.ok) throw new Error(`HTML fetch ${path} HTTP ${res.status}`);
    return (res as unknown as { text: () => Promise<string> }).text();
  }

  private parseUsageHtml(html: string): OpenCodeUsageRecord[] {
    try {
      const usagePos = html.indexOf("usage.list");
      const searchFrom = usagePos !== -1 ? html.slice(usagePos) : html;
      const m = searchFrom.match(/\$R\[\d+\]=\[([\s\S]*?)\]\)/);
      const arrContent = m?.[1];
      if (arrContent) {
        const arrayStr = `[${arrContent}]`;
        const $R: unknown[] = [];
        // eslint-disable-next-line no-new-func
        const fn = new Function("$R", `return ${arrayStr}`);
        const result = fn($R) as unknown[];
        const records = (Array.isArray(result) ? result : []).filter(Boolean) as Record<string, unknown>[];
        const out: OpenCodeUsageRecord[] = records.map((r) => {
          const rec = r as Record<string, unknown>;
          const toIso = (v: unknown) => (v instanceof Date ? (v as Date).toISOString() : typeof v === "string" ? v : String(v ?? ""));
          return {
            id: String(rec.id ?? ""),
            workspaceID: String((rec as Record<string, unknown>).workspaceID ?? (rec as Record<string, unknown>).workspaceId ?? ""),
            timeCreated: toIso(rec.timeCreated),
            timeUpdated: toIso((rec as Record<string, unknown>).timeUpdated ?? rec.timeCreated),
            timeDeleted: (rec.timeDeleted as string | null) ?? null,
            model: String(rec.model ?? ""),
            provider: String(rec.provider ?? ""),
            inputTokens: typeof rec.inputTokens === "number" ? rec.inputTokens : 0,
            outputTokens: typeof rec.outputTokens === "number" ? rec.outputTokens : 0,
            reasoningTokens: typeof rec.reasoningTokens === "number" ? rec.reasoningTokens : null,
            cacheReadTokens: typeof rec.cacheReadTokens === "number" ? rec.cacheReadTokens : null,
            cacheWrite5mTokens: typeof rec.cacheWrite5mTokens === "number" ? rec.cacheWrite5mTokens : null,
            cacheWrite1hTokens: typeof rec.cacheWrite1hTokens === "number" ? rec.cacheWrite1hTokens : null,
            cost: typeof rec.cost === "number" ? rec.cost : 0,
            keyID: String((rec as Record<string, unknown>).keyID ?? (rec as Record<string, unknown>).keyId ?? ""),
            sessionID: ((rec as Record<string, unknown>).sessionID as string | null) ?? null,
            enrichment: (rec.enrichment as unknown) ?? null,
          } as OpenCodeUsageRecord;
        });
        const seen = new Set<string>();
        const dedup: OpenCodeUsageRecord[] = [];
        for (const rec of out) if (rec.id && !seen.has(rec.id)) { seen.add(rec.id); dedup.push(rec); }
        dedup.sort((a, b) => Date.parse(b.timeCreated) - Date.parse(a.timeCreated));
        if (dedup.length > 0) return dedup;
      }
    } catch {
      // 忽略，走正则回退
    }
    // 回退：正则提取（兼容旧 HTML 或 $R 解析失败）
    const out: OpenCodeUsageRecord[] = [];
    const normalized = html.replace(/\$R\[\d+\]=new Date\("([^"]+)"\)/g, '"$1"');
    const re = /\{id:"(usg_[^"]+)"[\s\S]*?workspaceID:"[^"]+"[\s\S]*?timeCreated:"[^"]+"[\s\S]*?cost:[^,}]+[\s\S]*?\}/g;
    let m: RegExpExecArray | null;
    while ((m = re.exec(normalized)) !== null) {
      const raw = m[0];
      try {
        const jsonStr = raw
          .replace(/([{,]\s*)([a-zA-Z0-9_]+)\s*:/g, '$1"$2":')
          .replace(/:([a-zA-Z]+)([,\}])/g, (a, v, tail) => {
            if (v === "null" || v === "true" || v === "false") return `:${v}${tail}`;
            return `:"${v}"${tail}`;
          })
          .replace(/'/g, '"');
        const obj = JSON.parse(jsonStr) as Record<string, unknown>;
        const rec: OpenCodeUsageRecord = {
          id: String(obj.id ?? ""),
          workspaceID: String(obj.workspaceID ?? obj.workspaceId ?? ""),
          timeCreated: String(obj.timeCreated ?? ""),
          timeUpdated: String(obj.timeUpdated ?? obj.timeCreated ?? ""),
          timeDeleted: (obj.timeDeleted as string | null) ?? null,
          model: String(obj.model ?? ""),
          provider: String(obj.provider ?? ""),
          inputTokens: typeof obj.inputTokens === "number" ? obj.inputTokens : 0,
          outputTokens: typeof obj.outputTokens === "number" ? obj.outputTokens : 0,
          reasoningTokens: typeof obj.reasoningTokens === "number" ? obj.reasoningTokens : null,
          cacheReadTokens: typeof obj.cacheReadTokens === "number" ? obj.cacheReadTokens : null,
          cacheWrite5mTokens: typeof obj.cacheWrite5mTokens === "number" ? obj.cacheWrite5mTokens : null,
          cacheWrite1hTokens: typeof obj.cacheWrite1hTokens === "number" ? obj.cacheWrite1hTokens : null,
          cost: typeof obj.cost === "number" ? obj.cost : 0,
          keyID: String(obj.keyID ?? obj.keyId ?? ""),
          sessionID: (obj.sessionID as string | null) ?? null,
          enrichment: (obj.enrichment as unknown) ?? null,
        } as OpenCodeUsageRecord;
        if (rec.id) out.push(rec);
      } catch {
        // 忽略
      }
    }
    const seen = new Set<string>();
    const dedup: OpenCodeUsageRecord[] = [];
    for (const r of out) if (!seen.has(r.id)) { seen.add(r.id); dedup.push(r); }
    dedup.sort((a, b) => Date.parse(b.timeCreated) - Date.parse(a.timeCreated));
    return dedup;
  }

  async getUsageInfo(workspaceId: string, page: number): Promise<OpenCodeUsageRecord[]> {
    try {
      const data = await this.rpc(FN.usageHistory, [workspaceId, page]);
      if (data == null) return [];
      let arr: unknown[] | null = null;
      if (Array.isArray(data)) arr = data as unknown[];
      else if (typeof data === "object" && data !== null) {
        const obj = data as Record<string, unknown>;
        if (Array.isArray(obj.data)) arr = obj.data as unknown[];
        else if (Array.isArray(obj.records)) arr = obj.records as unknown[];
        else if (Array.isArray(obj.usage)) arr = obj.usage as unknown[];
      }
      if (arr) {
        const filtered = (arr as Record<string, unknown>[]).filter((r) => typeof r.id === "string" && (r.id as string).startsWith("usg_"));
        return filtered as unknown as OpenCodeUsageRecord[];
      }
      return [];
    } catch (e) {
      if (page !== 0) return [];
      try {
        const html = await this.fetchHtml(`/workspace/${workspaceId}/usage`);
        const records = this.parseUsageHtml(html);
        if (records.length > 0) return records;
      } catch {
        // 忽略
      }
      throw e;
    }
  }
}
