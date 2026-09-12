package pi

import (
	"os"

	"github.com/heihei0299/token-analyzer/internal/db"
)

// Refresh 负责 source → ledger 的全部同步：布局解析、文件枚举、增量导入。
// Query Engine 只读 ledger，生产查询前由调用方（server/CLI）先调 Refresh。
func Refresh(database *db.Database, piDir string) (SyncResult, error) {
	resolved, err := ResolvePiSessionRoot(
		os.Getenv("PI_CODING_AGENT_SESSION_DIR"),
		piDir,
		GetPiNativeSessionDir(),
	)
	if err != nil {
		return SyncResult{}, err
	}
	files := CollectPiJsonlFiles(resolved.Root, resolved.Layout)
	return SyncPiUsage(database, files)
}
