package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFingerprintDetectsSameSizeRewriteWithRestoredMtime(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "sessions", "2026", "09", "09", "rollout-2026-09-09T18-45-16-thread-fixture.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"a\":1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := Fingerprint(home)
	if err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"b\":1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, stat.ModTime(), stat.ModTime()); err != nil {
		t.Fatal(err)
	}
	after, err := Fingerprint(home)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("same-size rewrite with restored mtime must change the Codex fingerprint")
	}
}
