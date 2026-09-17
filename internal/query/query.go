package query

import (
	"errors"
	"fmt"

	"github.com/heihei0299/token-analyzer/internal/codex"
	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/pi"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

var (
	ErrRequestsUnsupported = errors.New("Codex/all source does not support requests query")
	ErrInvalidQuery        = errors.New("invalid query")
	ErrPiOnlyUnsupported   = errors.New("Pi-only capability unsupported")
)

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

// Validate checks the complete query contract without opening the ledger.
// Callers that also perform Refresh should run it before Refresh so invalid
// input cannot create or maintain a database as a side effect.
func Validate(cfg Config, filter sessiondata.Filter, view sessiondata.View) error {
	source, err := querySource(cfg.Source, filter.Source)
	if err != nil {
		return err
	}
	if source != "pi" && view.Kind == sessiondata.ViewRequests {
		return ErrRequestsUnsupported
	}
	if err := validateView(view); err != nil {
		return err
	}
	return nil
}

func querySource(configSource, filterSource string) (string, error) {
	if filterSource != "" {
		return sessiondata.NormalizeSource(filterSource)
	}
	return sessiondata.NormalizeSource(configSource)
}

func validateView(view sessiondata.View) error {
	switch view.Kind {
	case sessiondata.ViewTotals, sessiondata.ViewMeta:
		if view.Page != 0 || view.Size != 0 || view.SortKey != "" || view.SortDir != "" {
			return fmt.Errorf("%w: view %s does not support pagination or sorting", ErrInvalidQuery, view.Kind)
		}
	case sessiondata.ViewSessions:
		if err := validatePagination(view); err != nil {
			return err
		}
		if err := validateSort(view.Kind, view.SortKey, view.SortDir); err != nil {
			return err
		}
	case sessiondata.ViewRequests:
		if err := validatePagination(view); err != nil {
			return err
		}
		if err := validateSort(view.Kind, view.SortKey, view.SortDir); err != nil {
			return err
		}
	case sessiondata.ViewGroups:
		if view.By != domain.GroupByModel && view.By != domain.GroupByCwd && view.By != domain.GroupByModelCwd {
			return fmt.Errorf("%w: 未知分组: %s（支持 model/cwd/model,cwd）", ErrInvalidQuery, view.By)
		}
		if view.Page != 0 || view.Size != 0 || view.SortKey != "" || view.SortDir != "" {
			return fmt.Errorf("%w: groups view does not support pagination or sorting", ErrInvalidQuery)
		}
	case sessiondata.ViewPeriod:
		if view.Period != domain.PeriodDay && view.Period != domain.PeriodWeek && view.Period != domain.PeriodMonth {
			return fmt.Errorf("%w: 未知周期: %s（支持 day/week/month）", ErrInvalidQuery, view.Period)
		}
		if view.Page != 0 || view.Size != 0 || view.SortKey != "" || view.SortDir != "" {
			return fmt.Errorf("%w: period view does not support pagination or sorting", ErrInvalidQuery)
		}
	default:
		return fmt.Errorf("%w: %s", ErrInvalidQuery, errUnknownView(string(view.Kind)))
	}
	return nil
}

func validatePagination(view sessiondata.View) error {
	if view.Page < 0 || view.Size < 0 || (view.Page == 0) != (view.Size == 0) {
		return fmt.Errorf("%w: page and size must both be omitted or positive", ErrInvalidQuery)
	}
	return nil
}

func validateSort(kind sessiondata.ViewKind, key, direction string) error {
	if direction != "" && direction != "asc" && direction != "desc" {
		return fmt.Errorf("%w: sortDir must be asc or desc", ErrInvalidQuery)
	}
	if key == "" {
		if direction != "" {
			return fmt.Errorf("%w: sortDir requires sortKey", ErrInvalidQuery)
		}
		return nil
	}
	common := map[string]bool{
		"requests": true, "input": true, "output": true, "cache": true,
		"cacheRead": true, "cacheWrite": true, "reasoning": true,
		"totalTokens": true, "cost": true, "cacheRate": true,
	}
	if common[key] {
		return nil
	}
	if kind == sessiondata.ViewSessions {
		switch key {
		case "sessionId", "displayName", "cwd", "model", "timestamp":
			return nil
		}
	}
	if kind == sessiondata.ViewRequests {
		switch key {
		case "sessionId", "displayName", "model", "timestamp":
			return nil
		}
	}
	return fmt.Errorf("%w: unknown sort key: %s", ErrInvalidQuery, key)
}

// Query 是统一 Go Query Engine：对任意 source 只读 normalized ledger。
// 调用方须先做 Refresh（source → ledger），Query 本身不执行
// discovery/parse/refresh，不写 DB，不理解任何原始文件格式。
func Query(cfg Config, filter sessiondata.Filter, view sessiondata.View) (*sessiondata.QueryResult, error) {
	if err := Validate(cfg, filter, view); err != nil {
		return nil, err
	}
	source, _ := querySource(cfg.Source, filter.Source)
	// 快照读取：只读打开，不建库、不建表、不写任何内容。
	// ledger 不存在说明尚未做过初次 Refresh，报错由调用方经 refresh 状态解释。
	database, err := db.OpenReadOnly(db.ResolveDbPathFromEnv(cfg.DBPath))
	if err != nil {
		return nil, err
	}
	defer database.Close()
	piRoot := ""
	if source == "pi" || source == "all" {
		resolved, err := pi.ResolveConfiguredSessionRoot(cfg.PiDir)
		if err != nil {
			return nil, err
		}
		if resolved.Root != "" {
			piRoot, err = db.CanonicalSourceRoot(resolved.Root)
			if err != nil {
				return nil, err
			}
		}
		if err := db.CheckSourceRoot(database, "pi", piRoot); err != nil {
			return nil, err
		}
	}
	switch source {
	case "pi":
		return queryPi(database, piRoot, filter, view, nil)
	case "codex":
		return queryCodex(database, codex.ResolveHome(cfg.CodexDir), filter, view)
	default:
		cfg.PiDir = piRoot
		return queryAll(database, cfg, filter, view)
	}
}

// QueryDetail 是 Pi 专属会话详情（ledger 派生）；Codex/All 由 server 层明确拒绝。
func QueryDetail(cfg Config, sessionID string) (*sessiondata.SessionDetailResult, error) {
	source, err := sessiondata.NormalizeSource(cfg.Source)
	if err != nil {
		return nil, err
	}
	if source != "pi" {
		return nil, fmt.Errorf("%w: 会话详情仅支持 Pi source", ErrPiOnlyUnsupported)
	}
	database, err := db.OpenReadOnly(db.ResolveDbPathFromEnv(cfg.DBPath))
	if err != nil {
		return nil, err
	}
	defer database.Close()
	resolved, err := pi.ResolveConfiguredSessionRoot(cfg.PiDir)
	if err != nil {
		return nil, err
	}
	if resolved.Root != "" {
		resolved.Root, err = db.CanonicalSourceRoot(resolved.Root)
		if err != nil {
			return nil, err
		}
	}
	if err := db.CheckSourceRoot(database, "pi", resolved.Root); err != nil {
		return nil, err
	}
	return queryPiDetail(database, sessionID)
}

func errUnknownView(kind string) error {
	return fmt.Errorf("unknown view kind: %s", kind)
}

func errSessionNotFound(sessionID string) error {
	return fmt.Errorf("%w: %s", sessiondata.ErrSessionNotFound, sessionID)
}
