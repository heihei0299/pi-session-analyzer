package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var rolloutName = regexp.MustCompile(`^rollout-[0-9]{4}-[0-9]{2}-[0-9]{2}T[^/]+Z-[^/]+(?:_[^/]+)?\.jsonl(?:\.zst)?$`)

type RolloutFile struct {
	Path       string `json:"path"`
	PhysicalID string `json:"physicalId"`
	Compressed bool   `json:"compressed"`
}

type Diagnostics struct {
	Warnings  []string `json:"warnings,omitempty"`
	Skipped   int      `json:"skipped,omitempty"`
	Conflicts int      `json:"conflicts,omitempty"`
}

func (d *Diagnostics) warn(format string, args ...any) {
	d.Warnings = append(d.Warnings, fmt.Sprintf(format, args...))
}

func DiscoverRollouts(home string) ([]RolloutFile, Diagnostics, error) {
	var diagnostics Diagnostics
	info, err := os.Stat(home)
	if os.IsNotExist(err) {
		diagnostics.warn("Codex 目录不存在: %s", home)
		return nil, diagnostics, nil
	}
	if err != nil {
		return nil, diagnostics, err
	}
	if !info.IsDir() {
		return nil, diagnostics, fmt.Errorf("Codex 路径不是目录: %s", home)
	}

	chosen := make(map[string]RolloutFile)
	for _, rootName := range []string{"sessions", "archived_sessions"} {
		root := filepath.Join(home, rootName)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				diagnostics.warn("无法读取 %s: %v", path, walkErr)
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".jsonl") && !strings.HasSuffix(name, ".jsonl.zst") {
				return nil
			}
			if !rolloutName.MatchString(name) {
				diagnostics.Skipped++
				diagnostics.warn("跳过非 canonical Codex rollout: %s", path)
				return nil
			}
			compressed := strings.HasSuffix(name, ".jsonl.zst")
			physical := path
			if compressed {
				physical = strings.TrimSuffix(physical, ".zst")
			}
			current := RolloutFile{Path: path, PhysicalID: physical, Compressed: compressed}
			previous, exists := chosen[physical]
			if !exists || (previous.Compressed && !compressed) {
				chosen[physical] = current
			}
			return nil
		})
		if err != nil {
			return nil, diagnostics, err
		}
	}

	files := make([]RolloutFile, 0, len(chosen))
	for _, file := range chosen {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, diagnostics, nil
}

func ResolveHome(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	if value := strings.TrimSpace(os.Getenv("CODEX_HOME")); value != "" {
		return value
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".codex"
	}
	return filepath.Join(home, ".codex")
}
