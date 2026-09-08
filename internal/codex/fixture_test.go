package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyntheticFixtureCoversPlainAndArchiveRollouts(t *testing.T) {
	home := filepath.Join("testdata", "codex-home")
	files, diagnostics, err := DiscoverRollouts(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || diagnostics.Skipped != 0 {
		t.Fatalf("unexpected synthetic fixture discovery: files=%+v diagnostics=%+v", files, diagnostics)
	}
	for _, file := range files {
		parsed, _, err := ParseRollout(file)
		if err != nil {
			t.Fatal(err)
		}
		if len(parsed.Usage) == 0 {
			t.Fatalf("fixture rollout has no reliable usage: %s", file.Path)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "archived_sessions")); err != nil {
		t.Fatal(err)
	}
}
