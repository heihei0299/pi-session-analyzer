/**
 * Pi 会话发现（双布局 + 环境变量）
 * 对齐 cc-switch providers/pi.rs 的 resolve + collect 语义，直切无兼容
 */
import { readdirSync, statSync } from "node:fs";
import { isAbsolute, join } from "node:path";

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

/** 解析会话根，envDb 对应 PI_CODING_AGENT_SESSION_DIR */
export function resolvePiSessionRoot(opts: { envDb?: string; defaultRoot: string; piConfig?: string }): ResolveResult {
  const { envDb, defaultRoot, piConfig } = opts;
  const check = (v: string): boolean => isAbsolute(v);
  if (envDb !== undefined && envDb !== "") {
    if (!check(envDb)) throw new Error("PI_SESSION_DIR_REQUIRES_PROJECT_CONTEXT");
    return { root: envDb, layout: "flat" };
  }
  if (piConfig !== undefined && piConfig !== "") {
    if (!check(piConfig)) throw new Error("PI_SESSION_DIR_REQUIRES_PROJECT_CONTEXT");
    return { root: piConfig, layout: "flat" };
  }
  return { root: defaultRoot, layout: "projectDirectories" };
}
