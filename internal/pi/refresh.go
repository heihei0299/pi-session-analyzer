package pi

import (
	"github.com/heihei0299/token-analyzer/internal/db"
)

// Refresh 负责 source → ledger 的全部同步：布局解析、文件枚举、增量导入。
// Query Engine 只读 ledger，生产查询前由调用方（server/CLI）先调 Refresh。
func Refresh(database *db.Database, piDir string) (SyncResult, error) {
	resolved, err := ResolveConfiguredSessionRoot(piDir)
	if err != nil {
		return SyncResult{}, err
	}
	if resolved.Root != "" {
		canonical, err := db.CanonicalSourceRoot(resolved.Root)
		if err != nil {
			return SyncResult{}, err
		}
		resolved.Root = canonical
	}
	return RefreshResolved(database, resolved)
}

// RefreshResolved 使用调用方已经 canonicalize 的 Pi root，后续只访问该 physical path。
func RefreshResolved(database *db.Database, resolved ResolveResult) (SyncResult, error) {
	if resolved.Root != "" {
		if err := db.BindSourceRoot(database, "pi", resolved.Root); err != nil {
			return SyncResult{}, err
		}
	}
	files := CollectPiJsonlFiles(resolved.Root, resolved.Layout)
	return SyncPiUsage(database, files)
}
