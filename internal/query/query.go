package query

import (
	"errors"
	"fmt"

	"github.com/heihei0299/pi-session-anylize/internal/codex"
	"github.com/heihei0299/pi-session-anylize/internal/db"
	"github.com/heihei0299/pi-session-anylize/internal/sessiondata"
)

var ErrRequestsUnsupported = errors.New("Codex/all source does not support requests query")

type Config struct {
	PiDir    string
	CodexDir string
	DBPath   string
	Source   string
}

func Query(sd *sessiondata.SessionData, cfg Config, filter sessiondata.Filter, view sessiondata.View) (*sessiondata.QueryResult, error) {
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
	if source == "pi" {
		result, err := sd.Query(cfg.PiDir, filter, view)
		if err == nil && view.Kind == sessiondata.ViewMeta && filter.Source != "" && result.Meta != nil {
			result.Meta.Sources = []string{"pi"}
		}
		return result, err
	}
	if source == "codex" {
		return queryCodex(sd, cfg, filter, view)
	}

	codexFiles, diagnostics, err := loadCodex(cfg)
	if err != nil {
		return nil, err
	}
	piFiles, err := sd.ReadSessionFilesCached(cfg.PiDir)
	if err != nil {
		return nil, err
	}
	files := append(withSource(piFiles, "pi"), codexFiles...)
	result, err := sd.QueryFiles(cfg.PiDir, files, filter, view)
	if err != nil {
		return nil, err
	}
	attachMeta(sd, cfg.PiDir, files, result, diagnostics)
	return result, nil
}

func queryCodex(sd *sessiondata.SessionData, cfg Config, filter sessiondata.Filter, view sessiondata.View) (*sessiondata.QueryResult, error) {
	database, err := db.Open(db.ResolveDbPath(cfg.DBPath, ""))
	if err != nil {
		return nil, err
	}
	defer database.Close()
	files, diagnostics, err := syncAndLoad(database, cfg.CodexDir)
	if err != nil {
		return nil, err
	}
	result, err := sd.QueryFiles(codex.ResolveHome(cfg.CodexDir), files, filter, view)
	if err != nil {
		return nil, err
	}
	attachMeta(sd, codex.ResolveHome(cfg.CodexDir), files, result, diagnostics)
	if result.Meta != nil && len(result.Meta.Sources) == 0 {
		result.Meta.Sources = []string{"codex"}
	}
	return result, nil
}

func loadCodex(cfg Config) ([]*sessiondata.SessionFileData, codex.Diagnostics, error) {
	database, err := db.Open(db.ResolveDbPath(cfg.DBPath, ""))
	if err != nil {
		return nil, codex.Diagnostics{}, err
	}
	defer database.Close()
	return syncAndLoad(database, cfg.CodexDir)
}

func syncAndLoad(database *db.Database, codexDir string) ([]*sessiondata.SessionFileData, codex.Diagnostics, error) {
	home := codex.ResolveHome(codexDir)
	rollouts, discoveryDiagnostics, err := codex.DiscoverRollouts(home)
	if err != nil {
		return nil, discoveryDiagnostics, err
	}
	physicalIDs := make(map[string]bool, len(rollouts))
	for _, rollout := range rollouts {
		physicalIDs[rollout.PhysicalID] = true
	}
	syncResult, err := codex.SyncRollouts(database, home)
	if err != nil {
		return nil, syncResult.Diagnostics, err
	}
	files, err := codex.LoadSessionFiles(database, physicalIDs)
	return files, syncResult.Diagnostics, err
}

func withSource(files []*sessiondata.SessionFileData, source string) []*sessiondata.SessionFileData {
	out := make([]*sessiondata.SessionFileData, 0, len(files))
	for _, file := range files {
		copy := *file
		copy.Source = source
		out = append(out, &copy)
	}
	return out
}

func attachMeta(sd *sessiondata.SessionData, dir string, files []*sessiondata.SessionFileData, result *sessiondata.QueryResult, diagnostics codex.Diagnostics) {
	metaResult, err := sd.QueryFiles(dir, files, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewMeta})
	if err != nil || metaResult.Meta == nil {
		return
	}
	meta := metaResult.Meta
	meta.Warnings = append(meta.Warnings, diagnostics.Warnings...)
	meta.UncountedSnapshots += diagnostics.UncountedSnapshots
	if len(meta.Sources) == 0 && len(files) > 0 {
		meta.Sources = []string{"codex"}
	}
	result.Meta = meta
}
