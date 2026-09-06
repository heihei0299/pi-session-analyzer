/**
 * Pi 会话发现（双布局 + 环境变量）
 * 对齐 cc-switch providers/pi.rs 的 resolve + collect 语义，直切无兼容
 */
import { readdirSync, statSync, readFileSync, existsSync } from "node:fs";
import { isAbsolute, join } from "node:path";
import { homedir } from "node:os";
export type Layout = "flat" | "projectDirectories";

export interface ResolveResult {
  root: string;
  layout: Layout;
}

/** 收集 JSONL 文件，layout 决定深度 */
export function collectPiJsonlFiles(root: string, layout: Layout): string[] {
  const out: string[] = [];
  if (layout === "flat") {
    let entries: import("node:fs").Dirent[];
    try {
      entries = readdirSync(root, { withFileTypes: true });
    } catch {
      return out;
    }
    for (const e of entries) {
      if (e.isFile() && e.name.endsWith(".jsonl")) out.push(join(root, e.name));
    }
    out.sort();
    return out;
  }
  // projectDirectories: root/<project>/*.jsonl 两层
  let projects: import("node:fs").Dirent[];
  try {
    projects = readdirSync(root, { withFileTypes: true });
  } catch {
    return out;
  }
  for (const p of projects) {
    if (!p.isDirectory()) continue;
    const projPath = join(root, p.name);
    let files: import("node:fs").Dirent[];
    try {
      files = readdirSync(projPath, { withFileTypes: true });
    } catch {
      continue;
    }
    for (const f of files) {
      if (f.isFile() && f.name.endsWith(".jsonl")) out.push(join(projPath, f.name));
    }
  }
  out.sort();
  return out;
}

/** 读取 pi native defaults 的 session_dir（若 pi 配置了自定义路径） */
export function getPiNativeSessionDir(): string | undefined {
  // 约定：pi 的会话目录优先读 pi 配置文件中的 session_dir，若无则返回 undefined 走默认
  // 当前 pi 在 ~/.pi/agent/settings.json 未暴露 session_dir，保留扩展点：尝试读取环境或配置文件
  const candidates = [
    join(homedir(), ".pi", "agent", "settings.json"),
    join(homedir(), ".config", "pi", "config.json"),
  ];
  for (const p of candidates) {
    try {
      if (!existsSync(p)) continue;
      const raw = readFileSync(p, "utf8");
      const j = JSON.parse(raw) as Record<string, unknown>;
      const v = (j.session_dir as string) ?? (j.sessionDir as string) ?? (j["session-dir"] as string) ?? undefined;
      if (typeof v === "string" && v.trim() !== "") return v.trim();
    } catch {
      continue;
    }
  }
  return undefined;
}

/** 解析会话根，envDb 对应 PI_CODING_AGENT_SESSION_DIR */
export function resolvePiSessionRoot(opts: { envDb?: string; defaultRoot: string; piConfig?: string }): ResolveResult {
  const { envDb, defaultRoot, piConfig } = opts;
  const check = (v: string): boolean => isAbsolute(v);
  if (envDb !== undefined && envDb !== "") {
    if (!check(envDb)) throw new Error("400 PI_SESSION_DIR_REQUIRES_PROJECT_CONTEXT");
    return { root: envDb, layout: "flat" };
  }
  if (piConfig !== undefined && piConfig !== "") {
    if (!check(piConfig)) throw new Error("400 PI_SESSION_DIR_REQUIRES_PROJECT_CONTEXT");
    return { root: piConfig, layout: "flat" };
  }
  return { root: defaultRoot, layout: "projectDirectories" };
}
