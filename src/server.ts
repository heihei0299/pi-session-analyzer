/**
 * 零依赖 HTTP 服务器（serve 子命令）。
 * 路由分发：/ 与 /index.html → 单 HTML 内联前端；/api/* → API 层（api.ts）；
 * 其余路径 → 404 统一 JSON 错误体 { error, detail }。
 * 生命周期：EADDRINUSE → reject 友好消息；close() 优雅关闭。
 */
import { createServer, type Server, type ServerResponse } from "node:http";
import { readFileSync } from "node:fs";
import { handleApi } from "./api.ts";

const HTML = readFileSync(new URL("./webui.html", import.meta.url), "utf8");

export interface WebServerOptions {
  dir: string;
  host?: string;
  port?: number;
}

export interface WebServer {
  /** 访问 URL（http://host:port/） */
  url: string;
  /** 实际监听端口（port 0 时为系统分配端口） */
  port: number;
  /** 优雅关闭服务器 */
  close(): Promise<void>;
}

/** 启动 Web 服务器；端口被占用时 reject 友好消息（「端口 X 已被占用，可用 --port 更换」） */
export function startWebServer(options: WebServerOptions): Promise<WebServer> {
  const dir = options.dir;
  const host = options.host ?? "127.0.0.1";
  const port = options.port ?? 50080;
  return new Promise((resolve, reject) => {
    const server: Server = createServer((req, res) => {
      // 兜底：请求处理器意外 rejection 只记录，不使进程崩溃
      handleRequest(req.method ?? "GET", req.url ?? "/", dir, req, res).catch((err) => {
        console.error("request handler error:", err);
        if (!res.headersSent) send(res, 500, "application/json; charset=utf-8", JSON.stringify({ error: "Internal Server Error", detail: "服务器内部错误" }));
      });
    });
    server.on("error", (err: NodeJS.ErrnoException) => {
      if (err.code === "EADDRINUSE") {
        reject(new Error(`端口 ${port} 已被占用，可用 --port 更换`));
      } else {
        reject(err);
      }
    });
    server.listen(port, host, () => {
      const actualPort = (server.address() as { port: number }).port;
      resolve({
        url: `http://${host}:${actualPort}/`,
        port: actualPort,
        close: () =>
          new Promise<void>((done, fail) => {
            server.closeIdleConnections?.();
            server.close((err) => (err ? fail(err) : done()));
          }),
      });
    });
  });
}

async function handleRequest(method: string, pathname: string, dir: string, req: import("node:http").IncomingMessage, res: ServerResponse): Promise<void> {
  const url = new URL(pathname, "http://localhost");
  const p = url.pathname;

  if (p === "/" || p === "/index.html") {
    send(res, 200, "text/html; charset=utf-8", HTML);
    return;
  }
  if (p.startsWith("/api/")) {
    const rawBody = await readBody(req);
    const { status, body } = await handleApi(method, p, url.searchParams, dir, rawBody);
    send(res, status, "application/json; charset=utf-8", JSON.stringify(body));
    return;
  }
  send(res, 404, "application/json; charset=utf-8", JSON.stringify({ error: "Not Found", detail: `路径不存在: ${p}` }));
}

/** 读取请求体（UTF-8 文本；读取失败按空串处理） */
function readBody(req: import("node:http").IncomingMessage): Promise<string> {
  return new Promise((resolve) => {
    let data = "";
    req.on("data", (chunk: Buffer) => {
      data += chunk.toString("utf8");
    });
    req.on("end", () => resolve(data));
    req.on("error", () => resolve(""));
  });
}

function send(
  res: ServerResponse,
  status: number,
  contentType: string,
  body: string,
): void {
  res.writeHead(status, { "content-type": contentType });
  res.end(body);
}
