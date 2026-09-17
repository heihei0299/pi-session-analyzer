package pi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/db"
)

func TestRefreshResolvedRejectsLexicalRootWithoutReparsing(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "pi-root")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink is not supported: %v", err)
	}
	database, err := db.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := RefreshResolved(database, ResolveResult{Root: link, Layout: LayoutProjectDirectories}); !errors.Is(err, db.ErrSourceRootMismatch) {
		t.Fatalf("a non-canonical symlink root must be rejected without re-resolution: %v", err)
	}
	var bindings int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM source_root_bindings`).Scan(&bindings); err != nil || bindings != 0 {
		t.Fatalf("rejected root must not be bound: bindings=%d err=%v", bindings, err)
	}
}

func TestRefreshResolvedKeepsCanonicalPhysicalRootAfterSymlinkSwap(t *testing.T) {
	targetA := t.TempDir()
	targetB := t.TempDir()
	link := filepath.Join(t.TempDir(), "pi-root")
	if err := os.Symlink(targetA, link); err != nil {
		t.Skipf("symlink is not supported: %v", err)
	}
	writeSession := func(root, id string) {
		project := filepath.Join(root, "project")
		if err := os.MkdirAll(project, 0o755); err != nil {
			t.Fatal(err)
		}
		content := fmt.Sprintf(`{"type":"session","id":%q,"timestamp":"2026-09-17T10:00:00Z","cwd":"/workspace"}
{"type":"message","id":%q,"timestamp":"2026-09-17T10:01:00Z","message":{"role":"assistant","model":"m","usage":{"input":1,"output":2},"stopReason":"stop"}}
`, id, id+"-request")
		if err := os.WriteFile(filepath.Join(project, id+".jsonl"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSession(targetA, "target-a")
	writeSession(targetB, "target-b")

	canonical, err := db.CanonicalSourceRoot(link)
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetB, link); err != nil {
		t.Skipf("symlink target swap is not supported: %v", err)
	}
	if _, err := RefreshResolved(database, ResolveResult{Root: canonical, Layout: LayoutProjectDirectories}); err != nil {
		t.Fatal(err)
	}
	var sessionID string
	if err := database.DB.QueryRow(`SELECT session_id FROM pi_sessions`).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	if sessionID != "target-a" {
		t.Fatalf("canonical refresh must stay on the originally resolved root: %q", sessionID)
	}
}

func TestRefreshResolvedSkipsSymlinkProjectAndFileEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	goodProject := filepath.Join(root, "project")
	if err := os.MkdirAll(goodProject, 0o755); err != nil {
		t.Fatal(err)
	}
	goodContent := `{"type":"session","id":"good","timestamp":"2026-09-17T10:00:00Z","cwd":"/workspace"}
{"type":"message","id":"good-request","timestamp":"2026-09-17T10:01:00Z","message":{"role":"assistant","model":"m","usage":{"input":1,"output":2},"stopReason":"stop"}}
`
	if err := os.WriteFile(filepath.Join(goodProject, "good.jsonl"), []byte(goodContent), 0o644); err != nil {
		t.Fatal(err)
	}
	evilContent := `{"type":"session","id":"evil","timestamp":"2026-09-17T10:00:00Z","cwd":"/evil"}
{"type":"message","id":"evil-request","timestamp":"2026-09-17T10:01:00Z","message":{"role":"assistant","model":"m","usage":{"input":100,"output":200},"stopReason":"stop"}}
`
	if err := os.WriteFile(filepath.Join(outside, "evil.jsonl"), []byte(evilContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "evilproj")); err != nil {
		t.Skipf("symlink is not supported: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "evil.jsonl"), filepath.Join(goodProject, "evil-link.jsonl")); err != nil {
		t.Skipf("symlink is not supported: %v", err)
	}
	canonical, err := db.CanonicalSourceRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := RefreshResolved(database, ResolveResult{Root: canonical, Layout: LayoutProjectDirectories}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM pi_sessions WHERE session_id = 'evil'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("symlink project/file escape must not be imported: evil=%d", count)
	}
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM pi_sessions WHERE session_id = 'good'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("pinned good file must still import: count=%d err=%v", count, err)
	}
}

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
