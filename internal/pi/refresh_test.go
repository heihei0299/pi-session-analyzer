package pi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/db"
)

func TestRefreshHonorsProjectDirectoryLayoutWithoutRecursiveFallback(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("HOME", t.TempDir())

	root := t.TempDir()
	valid := filepath.Join(root, "project", "valid.jsonl")
	nested := filepath.Join(root, "project", "deeper", "nested.jsonl")
	content := `{"type":"session","id":"layout-session","timestamp":"2026-09-08T12:00:00Z","cwd":"/workspace"}
{"type":"message","id":"layout-request","timestamp":"2026-09-08T12:00:01Z","message":{"role":"assistant","model":"m","usage":{"input":2,"output":3}}}
`
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(valid, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := Refresh(database, root); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM proxy_request_logs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("only the canonical two-level file should be counted, got %d rows", count)
	}
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM session_log_sync`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("only the canonical file should receive a cursor, got %d", count)
	}
}
