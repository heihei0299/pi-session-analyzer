package pi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindSessionFileByHeaderIDReadsHeaderOnly(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "project", "named_uuid1.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"session","id":"uuid1","timestamp":"2026-09-10T00:00:00Z"}
not-json-body
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := FindSessionFileByHeaderID(root, LayoutProjectDirectories, "uuid1")
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("located path = %q, want %q", got, path)
	}
	if _, err := FindSessionFileByHeaderID(root, LayoutProjectDirectories, "missing"); err == nil {
		t.Fatal("missing session must return an error")
	}
}
