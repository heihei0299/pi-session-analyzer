package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// rolloutTimestampLayout 是上游 rollout 文件名里的秒级时间戳形态：日期与时间均以 '-' 分隔
// （例如 2026-09-09T18-45-16）。该时刻是上游的本地墙体时间，本适配只校验形态、不把它解释成时刻
// （时间归属一律用 rollout envelope 的 timestamp）。上游若改用其他粒度或分隔符，
// 这些文件会以「跳过非 canonical rollout」诊断现形，而不是被静默当作无关文件。
const rolloutTimestampLayout = "2006-01-02T15-04-05"

// classifyRolloutName 按上游 grammar 判定 Codex rollout 文件名，并区分压缩表示：
//
//	rollout-<秒级时间戳>-<thread id>[_<rollout id>].jsonl[.zst]
//
// 秒后可带历史形态的可选 'Z'；thread id / rollout id 只要求非空、不含路径分隔符与空白，
// 且 revert 形态至多一个 '_' 分段（归属一律以 session_meta 为准，文件名只用于发现、排序与物理 rollout 的 PhysicalID）。
func classifyRolloutName(name string) (compressed bool, canonical bool) {
	stem := name
	if strings.HasSuffix(stem, ".zst") {
		compressed = true
		stem = strings.TrimSuffix(stem, ".zst")
	}
	if !strings.HasSuffix(stem, ".jsonl") {
		return false, false
	}
	stem = strings.TrimSuffix(stem, ".jsonl")
	rest, found := strings.CutPrefix(stem, "rollout-")
	if !found || len(rest) <= len(rolloutTimestampLayout) {
		return false, false
	}
	if _, err := time.Parse(rolloutTimestampLayout, rest[:len(rolloutTimestampLayout)]); err != nil {
		return false, false
	}
	ids := strings.TrimPrefix(rest[len(rolloutTimestampLayout):], "Z")
	ids, found = strings.CutPrefix(ids, "-")
	if !found {
		return false, false
	}
	segments := strings.Split(ids, "_")
	if len(segments) > 2 {
		return false, false
	}
	for _, id := range segments {
		if id == "" || strings.ContainsAny(id, `/\ `) {
			return false, false
		}
	}
	return compressed, true
}

// isRolloutArtifact 判定一个文件名是否「看起来是 rollout 产物」：上游前缀开头的文件即便扩展名
// 不受支持（例如压缩表示换名）也必须以诊断现形，否则上游一改命名就会静默漏算。
func isRolloutArtifact(name string) bool {
	return strings.HasPrefix(name, "rollout-") ||
		strings.HasSuffix(name, ".jsonl") ||
		strings.HasSuffix(name, ".jsonl.zst")
}

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
	roots := 0
	for _, rootName := range []string{"sessions", "archived_sessions"} {
		root := filepath.Join(home, rootName)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		roots++
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				diagnostics.warn("无法读取 %s: %v", path, walkErr)
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			name := entry.Name()
			if !isRolloutArtifact(name) {
				return nil
			}
			compressed, canonical := classifyRolloutName(name)
			if !canonical {
				diagnostics.Skipped++
				diagnostics.warn("跳过非 canonical Codex rollout: %s", path)
				return nil
			}
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
	if roots == 0 {
		diagnostics.warn("Codex 目录下未找到 sessions/archived_sessions: %s", home)
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
