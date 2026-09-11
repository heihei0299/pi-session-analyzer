/** TS/npm 后端只实际提供 Pi；能力声明与所有出口校验必须共用这一份。 */
const SUPPORTED_SOURCES = ["pi"] as const;

export const GO_EDITION_HINT =
  "仅 Go 原生版本支持 Codex 数据源；npm/TS 版本只提供 Pi。请改用 Go 版本（token-analyzer-go）。";

/** 供 /api/meta 声明后端能力；返回副本避免调用方修改常量。 */
export function supportedSources(): string[] {
  return [...SUPPORTED_SOURCES];
}
