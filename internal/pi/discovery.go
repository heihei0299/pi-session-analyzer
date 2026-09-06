package pi

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Layout string

const (
	LayoutFlat               Layout = "flat"
	LayoutProjectDirectories Layout = "projectDirectories"
)

type ResolveResult struct {
	Root   string
	Layout Layout
}

// CollectPiJsonlFiles 按布局枚举
func CollectPiJsonlFiles(root string, layout Layout) []string {
	var out []string
	if layout == LayoutFlat {
		entries, err := os.ReadDir(root)
		if err != nil {
			return out
		}
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".jsonl" {
				out = append(out, filepath.Join(root, e.Name()))
			}
		}
		return out
	}
	projects, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		projPath := filepath.Join(root, p.Name())
		files, err := os.ReadDir(projPath)
		if err != nil {
			continue
		}
		for _, f := range files {
			if !f.IsDir() && filepath.Ext(f.Name()) == ".jsonl" {
				out = append(out, filepath.Join(projPath, f.Name()))
			}
		}
	}
	return out
}

// GetPiNativeSessionDir 读取 pi native defaults 的 session_dir
func GetPiNativeSessionDir() string {
	candidates := []string{
		filepath.Join(os.Getenv("HOME"), ".pi", "agent", "settings.json"),
		filepath.Join(os.Getenv("HOME"), ".config", "pi", "config.json"),
	}
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var j map[string]interface{}
		if err := json.Unmarshal(b, &j); err != nil {
			continue
		}
		for _, k := range []string{"session_dir", "sessionDir", "session-dir"} {
			if v, ok := j[k]; ok {
				if s, ok := v.(string); ok && s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// ResolvePiSessionRoot 按优先级解析
func ResolvePiSessionRoot(envDb, defaultRoot, piConfig string) (ResolveResult, error) {
	if envDb != "" {
		if !filepath.IsAbs(envDb) {
			return ResolveResult{}, ErrRequiresProjectContext
		}
		return ResolveResult{Root: envDb, Layout: LayoutFlat}, nil
	}
	if piConfig != "" {
		if !filepath.IsAbs(piConfig) {
			return ResolveResult{}, ErrRequiresProjectContext
		}
		return ResolveResult{Root: piConfig, Layout: LayoutFlat}, nil
	}
	return ResolveResult{Root: defaultRoot, Layout: LayoutProjectDirectories}, nil
}
var ErrRequiresProjectContext = &resolveError{"400 PI_SESSION_DIR_REQUIRES_PROJECT_CONTEXT"}

type resolveError struct{ msg string }

func (e *resolveError) Error() string { return e.msg }
