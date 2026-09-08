package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverRolloutsFindsCanonicalFilesAndPrefersPlainSibling(t *testing.T) {
	home := t.TempDir()
	for _, dir := range []string{"sessions/project", "archived_sessions"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	plain := filepath.Join(home, "sessions", "project", "rollout-2026-09-08T12-00-00Z-thread-1.jsonl")
	compressed := plain + ".zst"
	archived := filepath.Join(home, "archived_sessions", "rollout-2026-09-07T12-00-00Z-thread-2_rollout-9.jsonl")
	for _, path := range []string{plain, compressed, archived} {
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "sessions", "ignore.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, diagnostics, err := DiscoverRollouts(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 logical rollouts, got %d: %+v", len(files), files)
	}
	if files[0].Path != archived || files[1].Path != plain {
		t.Fatalf("expected sorted archived/plain paths, got %+v", files)
	}
	if files[1].Compressed {
		t.Fatalf("plain sibling should win: %+v", files[1])
	}
	if diagnostics.Skipped == 0 {
		t.Fatalf("expected diagnostic for non-canonical file")
	}
}
