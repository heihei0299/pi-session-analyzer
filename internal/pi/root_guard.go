package pi

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func isSymlinkPath(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// containmentBase resolves the pinned root to its physical path. Pinned roots
// are already canonical, but resolving again keeps the check correct when the
// root path itself has symlinked ancestors (e.g. macOS temp directories).
func containmentBase(pinnedRoot string) (string, error) {
	resolved, err := filepath.EvalSymlinks(pinnedRoot)
	if err != nil {
		return "", fmt.Errorf("resolve pinned root %q: %v", pinnedRoot, err)
	}
	return filepath.Clean(resolved), nil
}

func escapesRoot(base, path string) bool {
	rel, err := filepath.Rel(base, filepath.Clean(path))
	return err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// VerifyPinnedSessionFile ensures a candidate session file stays on the pinned
// physical root: symlink files/projects are refused and the resolved path must
// remain lexically within the pinned root. Callers must invoke it on the same
// pinned root used for binding, immediately before file reads or renames.
func VerifyPinnedSessionFile(pinnedRoot, candidate string) error {
	if strings.TrimSpace(pinnedRoot) == "" || strings.TrimSpace(candidate) == "" {
		return fmt.Errorf("pinned root containment failed for %q", candidate)
	}
	base, err := containmentBase(pinnedRoot)
	if err != nil {
		return err
	}
	if isSymlinkPath(candidate) {
		return fmt.Errorf("refuse symlink file %q", candidate)
	}
	if parent := filepath.Dir(candidate); filepath.Clean(parent) != base && isSymlinkPath(parent) {
		return fmt.Errorf("refuse symlink project %q", parent)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return fmt.Errorf("resolve candidate %q: %v", candidate, err)
	}
	if escapesRoot(base, resolved) {
		return fmt.Errorf("candidate %q escapes pinned root %q", candidate, pinnedRoot)
	}
	parentResolved, err := filepath.EvalSymlinks(filepath.Dir(candidate))
	if err != nil {
		return fmt.Errorf("resolve candidate parent %q: %v", candidate, err)
	}
	if escapesRoot(base, parentResolved) {
		return fmt.Errorf("candidate parent %q escapes pinned root %q", candidate, pinnedRoot)
	}
	return nil
}

// VerifyPinnedTarget ensures a not-yet-existing rename target stays in the same
// pinned project directory without following a swapped symlink parent.
func VerifyPinnedTarget(pinnedRoot, target string) error {
	if strings.TrimSpace(pinnedRoot) == "" || strings.TrimSpace(target) == "" {
		return fmt.Errorf("pinned root containment failed for %q", target)
	}
	base, err := containmentBase(pinnedRoot)
	if err != nil {
		return err
	}
	if isSymlinkPath(target) {
		return fmt.Errorf("refuse symlink target %q", target)
	}
	parent := filepath.Dir(target)
	if filepath.Clean(parent) != base && isSymlinkPath(parent) {
		return fmt.Errorf("refuse symlink project %q", parent)
	}
	if escapesRoot(base, target) {
		return fmt.Errorf("target %q escapes pinned root %q", target, pinnedRoot)
	}
	parentResolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return fmt.Errorf("resolve target parent %q: %v", target, err)
	}
	if escapesRoot(base, parentResolved) {
		return fmt.Errorf("target parent %q escapes pinned root %q", target, pinnedRoot)
	}
	return nil
}
