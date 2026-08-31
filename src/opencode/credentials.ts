/**
 * OpenCode 凭证加载器 — CLI > process.env > .env
 * ponytail: 手写 .env 解析（KEY=VALUE 去引号，忽略注释），零依赖
 */
import { readFileSync } from "node:fs";
import { join } from "node:path";

export interface Credentials {
  auth: string;
  workspace?: string;
  dataDir: string;
}

export interface CredentialsInput {
  auth?: string;
  workspace?: string;
  dataDir?: string;
}

function parseEnvContent(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (!line || line.startsWith("#")) continue;
    // 去掉 export 前缀
    const stripped = line.startsWith("export ") ? line.slice(7).trim() : line;
    const eq = stripped.indexOf("=");
    if (eq === -1) continue;
    const key = stripped.slice(0, eq).trim();
    let val = stripped.slice(eq + 1).trim();
    // 去引号：支持引号包裹后带行内注释的场景
    if (val.startsWith('"') || val.startsWith("'")) {
      const quote = val[0];
      // 寻找对应的闭合引号（不考虑转义的简单版：取第一个同符号引号）
      let endIdx = -1;
      for (let qi = 1; qi < val.length; qi++) {
        if (val[qi] === quote && val[qi - 1] !== "\\") { endIdx = qi; break; }
      }
      if (endIdx !== -1) {
        val = val.slice(1, endIdx);
        val = val.replace(/\\n/g, "\n").replace(/\\"/g, '"').replace(/\\'/g, "'").replace(/\\\\/g, "\\");
      } else if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
        val = val.slice(1, -1);
        val = val.replace(/\\n/g, "\n").replace(/\\"/g, '"').replace(/\\'/g, "'").replace(/\\\\/g, "\\");
      } else {
        const hash = val.indexOf(" #");
        if (hash !== -1) val = val.slice(0, hash).trim();
      }
    } else {
      // 未引号时，去掉行内注释？仅当 # 前有空格时
      const hash = val.indexOf(" #");
      if (hash !== -1) val = val.slice(0, hash).trim();
    }
    if (!key) continue;
    out[key] = val;
  }
  return out;
}

function tryReadEnvFile(filePath: string): Record<string, string> {
  try {
    const txt = readFileSync(filePath, "utf8");
    return parseEnvContent(txt);
  } catch {
    return {};
  }
}

/**
 * 读取 .env 文件集合，优先级低→高合并（后者覆盖前者）
 * 候选：cwd/.env, dataDir/.env（若不同）
 */
function loadDotEnvRecord(dataDir?: string): Record<string, string> {
  const candidates: string[] = [];
  const cwdEnv = join(process.cwd(), ".env");
  candidates.push(cwdEnv);
  if (dataDir) {
    const dEnv = join(dataDir, ".env");
    if (dEnv !== cwdEnv) candidates.push(dEnv);
    // 也尝试 dataDir 上一级（项目根）？
    // 若 dataDir 为 data/opencode，尝试 cwd 即可已覆盖
  }
  // 额外尝试项目根 .env（与 cwd 相同则已包含）
  // 也尝试从 cwd 向上查找？最小实现：仅 cwd 与 dataDir
  const merged: Record<string, string> = {};
  for (const p of candidates) {
    const rec = tryReadEnvFile(p);
    Object.assign(merged, rec);
  }
  return merged;
}

/**
 * 优先级合并：CLI > env
 * env 来自 process.env 与 .env 合并后的记录
 */
export function resolveCredentials(
  cli: CredentialsInput,
  env: Record<string, string | undefined>,
): { auth?: string; workspace?: string; dataDir?: string } {
  const auth = cli.auth ?? env.OPENCODE_AUTH ?? undefined;
  const workspace = cli.workspace ?? env.OPENCODE_WORKSPACE_ID ?? env.OPENCODE_WORKSPACE ?? undefined;
  const dataDir = cli.dataDir ?? env.OPENCODE_DATA_DIR ?? undefined;
  return { auth, workspace, dataDir };
}

/**
 * 完整凭证加载：CLI > process.env > .env 文件
 * 缺少 auth 时抛友好错误
 */
export function loadCredentials(opts?: CredentialsInput): Credentials {
  const cliAuth = opts?.auth;
  const cliWorkspace = opts?.workspace;
  const cliDataDir = opts?.dataDir;

  const dot = loadDotEnvRecord(cliDataDir);
  // 合并：dot 低，process.env 高
  const merged: Record<string, string | undefined> = { ...dot };
  for (const [k, v] of Object.entries(process.env)) {
    if (v !== undefined) merged[k] = v;
  }

  const resolved = resolveCredentials(
    { auth: cliAuth, workspace: cliWorkspace, dataDir: cliDataDir },
    merged,
  );

  const auth = resolved.auth;
  if (!auth) {
    throw new Error("缺少认证信息，请设置 OPENCODE_AUTH 环境变量或传 --auth");
  }
  const workspace = resolved.workspace;
  const dataDir = resolved.dataDir ?? dot.OPENCODE_DATA_DIR ?? "data/opencode";

  return { auth, workspace, dataDir };
}

// 供测试直接断言 .env 解析
export const _internal = { parseEnvContent, tryReadEnvFile, loadDotEnvRecord };
