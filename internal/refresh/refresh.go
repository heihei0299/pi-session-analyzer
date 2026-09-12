// Package refresh 负责 source → normalized ledger 的全部同步（Refresh）。
//
// Query Engine（internal/query）只读已提交的 ledger 快照，从不执行
// discovery/parse/refresh，也不写 DB。生产查询前由调用方（server/CLI）
// 先调 Refresh，再调 query.Query，两者对同一 Config 映射到同一 domain 结果。
package refresh

import (
	"github.com/heihei0299/pi-session-anylize/internal/codex"
	"github.com/heihei0299/pi-session-anylize/internal/db"
	"github.com/heihei0299/pi-session-anylize/internal/pi"
)

// Config 与 query.Config 同构（分包避免 query 反向依赖 adapter）。
type Config struct {
	PiDir    string
	CodexDir string
	DBPath   string
	Source   string
}

// Refresh 按本次查询的数据源做统一同步：pi 来自 Pi 会话目录，
// codex/all 额外同步 Codex rollout。幂等，可失败重试。
func Refresh(cfg Config) error {
	source := cfg.Source
	if source == "" {
		source = "pi"
	}
	database, err := db.Open(db.ResolveDbPath(cfg.DBPath, ""))
	if err != nil {
		return err
	}
	defer database.Close()
	if source == "pi" || source == "all" {
		if _, err := pi.Refresh(database, cfg.PiDir); err != nil {
			return err
		}
	}
	if source == "codex" || source == "all" {
		if _, err := codex.SyncRollouts(database, codex.ResolveHome(cfg.CodexDir)); err != nil {
			return err
		}
	}
	return nil
}
