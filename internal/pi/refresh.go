package pi

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/heihei0299/pi-session-anylize/internal/db"
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
	if len(files) == 0 {
		// 回退递归收集（与 TS oracle 在测试 fixture 下的行为一致）。
		files = collectJsonlRecursive(resolved.Root)
	}
	return SyncPiUsage(database, files)
}

func collectJsonlRecursive(root string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out
}
