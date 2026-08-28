/**
 * @deprecated 薄 shim — 真相在 src/session-data.ts（SessionData 深模块）。
 * 保留 1 版本以兼容旧 import 路径 `from "./analyze.ts"`；新代码请 `from "./session-data.ts"`。
 * 本文件不含实现，仅 re-export 默认单例的委托。
 */
export type { SessionFileData, Filter, View, TimeRange, SessionTimeRange, MessageTimeRange, SessionRowEnriched, RequestRowEnriched } from "./session-data.ts";
export {
  collectJsonlFiles,
  parseUtcTimestamp,
  parseTimestamp,
  normalizeCwd,
  periodKey,
  groupRowsFromFiles,
  periodRowsFromFiles,
  filterFiles,
  totalsFromFiles,
  sessionRowsFromFiles,
  requestRowsFromFiles,
  analyzeFile,
  readSessionFiles,
  readSessionFilesCached,
  __setFileLoaderForTest,
  defaultSessionData,
} from "./session-data.ts";
export { SessionData } from "./session-data.ts";
