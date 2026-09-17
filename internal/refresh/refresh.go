// Package refresh 负责 source → normalized ledger 的全部同步（Refresh）。
//
// Query Engine（internal/query）只读已提交的 ledger 快照，从不执行
// discovery/parse/refresh，也不写 DB。运行时（server/CLI/watch）先调 Refresh
// 再调 query.Query，两者对同一 Config 映射到同一 domain 结果。
package refresh

import (
	"sync"
	"time"

	"github.com/heihei0299/token-analyzer/internal/codex"
	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/pi"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

// Config 与 query.Config 同构（分包避免 query 反向依赖 adapter）。
type Config struct {
	PiDir    string
	CodexDir string
	DBPath   string
	Source   string
}

// refreshMu 把进程内并发 refresh 串行化：多个触发源（HTTP watch、CLI、
// 手动）不会放大为重复同步。ledger 写本身幂等，串行只为省工。
var refreshMu sync.Mutex

// Refresh 按本次查询的数据源做统一同步：pi 来自 Pi 会话目录，
// codex/all 额外同步 Codex rollout。幂等，可失败重试。
// 失败不清除旧 ledger：sync 按文件事务提交，失败只影响本次增量。
func Refresh(cfg Config) error {
	source, err := sessiondata.NormalizeSource(cfg.Source)
	if err != nil {
		recordResult(err)
		return err
	}
	cfg.Source = source
	refreshMu.Lock()
	defer refreshMu.Unlock()
	err = refreshLocked(cfg)
	recordResult(err)
	return err
}

func refreshLocked(cfg Config) error {
	source := cfg.Source
	if source == "" {
		source = "pi"
	}
	var piRoot pi.ResolveResult
	if source == "pi" || source == "all" {
		resolved, err := pi.ResolveConfiguredSessionRoot(cfg.PiDir)
		if err != nil {
			return err
		}
		piRoot = resolved
		// An omitted Pi root is allowed for all-source Codex-only setups.
		if piRoot.Root == "" {
			if source == "pi" {
				return db.ValidateSourceRoot(piRoot.Root)
			}
		} else if err := db.ValidateSourceRoot(piRoot.Root); err != nil {
			return err
		}
	}
	database, err := db.Open(db.ResolveDbPathFromEnv(cfg.DBPath))
	if err != nil {
		return err
	}
	defer database.Close()
	if source == "pi" || source == "all" {
		if _, err := pi.RefreshResolved(database, piRoot); err != nil {
			return err
		}
	}
	if source == "codex" || source == "all" {
		if _, err := codex.SyncRollouts(database, codex.ResolveHome(cfg.CodexDir)); err != nil {
			return err
		}
	}
	return db.RollupAndPrune(database, time.Now(), db.DefaultRollupRetentionDays)
}
