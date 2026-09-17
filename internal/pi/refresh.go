package pi

import (
	"fmt"
	"strings"

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

// RefreshResolved 使用调用方已经 canonicalize 的 Pi root，后续只访问该 pinned physical path。
// 不在绑定阶段重新解析丢弃 identity；候选文件逐个验证 containment 并拒绝 symlink。
func RefreshResolved(database *db.Database, resolved ResolveResult) (SyncResult, error) {
	if strings.TrimSpace(resolved.Root) == "" {
		has, err := db.HasPiHistory(database)
		if err != nil {
			return SyncResult{}, fmt.Errorf("check existing Pi history: %w", err)
		}
		if has {
			return SyncResult{}, fmt.Errorf("%w: Pi history has no root binding; explicit migration or a new ledger is required", db.ErrSourceRootBindingRequired)
		}
		return SyncPiUsage(database, nil)
	}
	if err := db.BindPinnedSourceRoot(database, "pi", resolved.Root); err != nil {
		return SyncResult{}, err
	}
	files := CollectPiJsonlFiles(resolved.Root, resolved.Layout)
	for _, f := range files {
		if err := VerifyPinnedSessionFile(resolved.Root, f); err != nil {
			return SyncResult{}, err
		}
	}
	return SyncPiUsage(database, files)
}
