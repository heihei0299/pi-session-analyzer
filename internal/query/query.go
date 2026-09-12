package query

import (
	"errors"
	"fmt"

	"github.com/heihei0299/token-analyzer/internal/codex"
	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

var ErrRequestsUnsupported = errors.New("Codex/all source does not support requests query")

type Config struct {
	PiDir    string
	CodexDir string
	DBPath   string
	Source   string
}

// SupportedSources 声明本后端能提供的数据源（能力声明，不代表本次查询的参与源）。
// 共享 WebUI 只用它决定源选择器的选项，不再自行假设后端支持什么。
func SupportedSources() []string {
	return []string{"pi", "codex"}
}

// Query 是统一 Go Query Engine：对任意 source 只读 normalized ledger。
// 调用方须先做 Refresh（source → ledger），Query 本身不执行
// discovery/parse/refresh，不写 DB，不理解任何原始文件格式。
func Query(cfg Config, filter sessiondata.Filter, view sessiondata.View) (*sessiondata.QueryResult, error) {
	source := cfg.Source
	if filter.Source != "" {
		source = filter.Source
	}
	if source == "" {
		source = "pi"
	}
	if source != "pi" && source != "codex" && source != "all" {
		return nil, fmt.Errorf("未知 source: %s（支持 pi|codex|all）", source)
	}
	if view.Kind == sessiondata.ViewRequests && source != "pi" {
		return nil, ErrRequestsUnsupported
	}
	// 快照读取：只读打开，不建库、不建表、不写任何内容。
	// ledger 不存在说明尚未做过初次 Refresh，报错由调用方经 refresh 状态解释。
	database, err := db.OpenReadOnly(db.ResolveDbPathFromEnv(cfg.DBPath))
	if err != nil {
		return nil, err
	}
	defer database.Close()
	switch source {
	case "pi":
		return queryPi(database, cfg.PiDir, filter, view, nil)
	case "codex":
		return queryCodex(database, codex.ResolveHome(cfg.CodexDir), filter, view)
	default:
		return queryAll(database, cfg, filter, view)
	}
}

// QueryDetail 是 Pi 专属会话详情（ledger 派生）；Codex/All 由 server 层明确拒绝。
func QueryDetail(cfg Config, sessionID string) (*sessiondata.SessionDetailResult, error) {
	database, err := db.OpenReadOnly(db.ResolveDbPathFromEnv(cfg.DBPath))
	if err != nil {
		return nil, err
	}
	defer database.Close()
	return queryPiDetail(database, sessionID)
}

func errUnknownView(kind string) error {
	return fmt.Errorf("unknown view kind: %s", kind)
}

func errSessionNotFound(sessionID string) error {
	return fmt.Errorf("%w: %s", sessiondata.ErrSessionNotFound, sessionID)
}
