import { encodePayload, decodeResponseText } from "./seroval.ts";
import type { OpenCodeCostsResult, OpenCodeUsageRecord, WorkspaceInfo } from "./types.ts";

const RPC_URL = "https://opencode.ai/_server";
const FN = {
  workspaces: "def39973159c7f0483d8793a822b8dbb10d067e12c65455fcb4608459ba0234f",
  monthlyCosts: "15702f3a12ff8bff357f8c2aa154a17e65b746d5f6b96adc9002c86ee0c15205",
  usageHistory: "bfd684bfc2e4eed05cd0b518f5e4eafd3f3376e3938abb9e536e7c03df831e5c",
} as const;

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
    const body = encodePayload(args);
    const headers: Record<string, string> = {
      "content-type": "application/json",
      "cookie": `auth=${this.auth}`,
      "x-opencode-fn": functionId,
    };
    let res: Response;
    try {
      // 15s 超时
      const signal = typeof AbortSignal !== "undefined" && typeof (AbortSignal as any).timeout === "function"
        ? (AbortSignal as any).timeout(15_000) as AbortSignal
        : undefined;
      res = await (this.fetchImpl as any)(RPC_URL, { method: "POST", headers, body, signal });
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
    const data = await this.rpc(FN.monthlyCosts, args);
    if (data == null) return { usage: [], keys: [] };
    if (typeof data === "object" && data !== null) {
      const obj = data as Record<string, unknown>;
      const usage = Array.isArray(obj.usage) ? obj.usage as OpenCodeCostsResult["usage"] : [];
      const keys = Array.isArray(obj.keys) ? obj.keys as OpenCodeCostsResult["keys"] : [];
      return { usage, keys };
    }
    return { usage: [], keys: [] };
  }

  async getUsageInfo(workspaceId: string, page: number): Promise<OpenCodeUsageRecord[]> {
    const data = await this.rpc(FN.usageHistory, [workspaceId, page]);
    if (data == null) return [];
    if (Array.isArray(data)) return data as OpenCodeUsageRecord[];
    if (typeof data === "object" && data !== null) {
      const obj = data as Record<string, unknown>;
      if (Array.isArray(obj.data)) return obj.data as OpenCodeUsageRecord[];
      if (Array.isArray(obj.records)) return obj.records as OpenCodeUsageRecord[];
      if (Array.isArray(obj.usage)) return obj.usage as OpenCodeUsageRecord[];
    }
    return [];
  }
}
