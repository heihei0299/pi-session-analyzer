package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heihei0299/token-analyzer/internal/db"
)

func TestServerRenameRejectsMismatchedRootBeforeFilesystemRename(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("HOME", t.TempDir())
	boundRoot := t.TempDir()
	requestedRoot := t.TempDir()
	piFile := filepath.Join(requestedRoot, "project", "PiSession_uuid1.jsonl")
	if err := os.MkdirAll(filepath.Dir(piFile), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"session","id":"uuid1","timestamp":"2026-08-01T10:00:00Z","cwd":"/pi/project"}
{"type":"message","id":"request-1","timestamp":"2026-08-01T10:05:00Z","message":{"role":"assistant","model":"m1","usage":{"input":10,"output":5},"stopReason":"stop"}}
`
	if err := os.WriteFile(piFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(piFile, stale, stale); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(piFile)
	if err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.BindSourceRoot(database, "pi", boundRoot); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(requestedRoot, Options{DBPath: dbPath})
	request := httptest.NewRequest(http.MethodPost, "/api/sessions/rename", strings.NewReader(`{"sessionId":"uuid1","name":"NewName"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	srv.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "source root mismatch") {
		t.Fatalf("rename must reject a mismatched root before mutation: status=%d body=%s", response.Code, response.Body.String())
	}
	after, err := os.ReadFile(piFile)
	if err != nil {
		t.Fatalf("mismatched-root rename must keep the original file: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("mismatched-root rename must not change file contents")
	}
	if _, err := os.Stat(filepath.Join(requestedRoot, "project", "NewName_uuid1.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("mismatched-root rename must not create a new file: %v", err)
	}
}

func TestServerRenameRejectsMissingBindingBeforeFilesystemRename(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("HOME", t.TempDir())
	piDir := t.TempDir()
	piFile := filepath.Join(piDir, "project", "PiSession_uuid1.jsonl")
	if err := os.MkdirAll(filepath.Dir(piFile), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"session","id":"uuid1","timestamp":"2026-08-01T10:00:00Z","cwd":"/pi/project"}
{"type":"message","id":"request-1","timestamp":"2026-08-01T10:05:00Z","message":{"role":"assistant","model":"m1","usage":{"input":10,"output":5},"stopReason":"stop"}}
`
	if err := os.WriteFile(piFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(piFile, stale, stale); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(piFile)
	if err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(piDir, Options{DBPath: dbPath})
	request := httptest.NewRequest(http.MethodPost, "/api/sessions/rename", strings.NewReader(`{"sessionId":"uuid1","name":"NewName"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	srv.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "source root binding missing") {
		t.Fatalf("rename must reject an unbound root before mutation: status=%d body=%s", response.Code, response.Body.String())
	}
	after, err := os.ReadFile(piFile)
	if err != nil {
		t.Fatalf("unbound-root rename must keep the original file: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("unbound-root rename must not change file contents")
	}
	if _, err := os.Stat(filepath.Join(piDir, "project", "NewName_uuid1.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("unbound-root rename must not create a new file: %v", err)
	}
}

func TestServerRenameRejectsUnavailableRootBeforeFilesystemRename(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("HOME", t.TempDir())
	invalidRoot := filepath.Join(t.TempDir(), "pi-file")
	if err := os.WriteFile(invalidRoot, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	boundRoot := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.BindSourceRoot(database, "pi", boundRoot); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(invalidRoot, Options{DBPath: dbPath})
	request := httptest.NewRequest(http.MethodPost, "/api/sessions/rename", strings.NewReader(`{"sessionId":"uuid1","name":"NewName"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	srv.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "source root unavailable") {
		t.Fatalf("rename must reject an unavailable root before lookup: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestServerRenameReportsSnapshotVerificationFailureAfterFilesystemRename(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("HOME", t.TempDir())
	piDir := t.TempDir()
	piFile := filepath.Join(piDir, "project", "PiSession_uuid1.jsonl")
	if err := os.MkdirAll(filepath.Dir(piFile), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"session","id":"uuid1","timestamp":"2026-08-01T10:00:00Z","cwd":"/pi/project"}
{"type":"message","id":"request-1","timestamp":"2026-08-01T10:05:00Z","message":{"role":"assistant","model":"m1","usage":{"input":10,"output":5}}}
`
	if err := os.WriteFile(piFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(piFile, stale, stale); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	srv := NewServer(piDir, Options{CodexDir: t.TempDir(), DBPath: dbPath})
	mustRefreshNow(t, srv)
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.Exec(`CREATE TRIGGER remove_renamed_session AFTER INSERT ON session_log_sync BEGIN DELETE FROM pi_sessions WHERE session_id = 'uuid1'; END`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/sessions/rename", strings.NewReader(`{"sessionId":"uuid1","name":"NewName"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	srv.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "snapshot verification") {
		t.Fatalf("snapshot verification failure must be explicit: status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(piDir, "project", "NewName_uuid1.jsonl")); err != nil {
		t.Fatalf("filesystem rename must be kept after verification failure: %v", err)
	}
	if _, err := os.Stat(piFile); !os.IsNotExist(err) {
		t.Fatalf("old path must stay renamed, err=%v", err)
	}
}

func TestServerRenameRejectsSymlinkTargetSwitchBeforeFilesystemRename(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("HOME", t.TempDir())
	targetA := t.TempDir()
	targetB := t.TempDir()
	link := filepath.Join(t.TempDir(), "pi-root")
	if err := os.Symlink(targetA, link); err != nil {
		t.Skipf("symlink is not supported: %v", err)
	}
	piFile := filepath.Join(targetB, "project", "PiSession_uuid1.jsonl")
	if err := os.MkdirAll(filepath.Dir(piFile), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"session","id":"uuid1","timestamp":"2026-09-17T10:00:00Z","cwd":"/pi/project"}
{"type":"message","id":"request-1","timestamp":"2026-09-17T10:05:00Z","message":{"role":"assistant","model":"m1","usage":{"input":10,"output":5},"stopReason":"stop"}}
`
	if err := os.WriteFile(piFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(piFile, stale, stale); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.BindSourceRoot(database, "pi", targetA); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetB, link); err != nil {
		t.Skipf("symlink target swap is not supported: %v", err)
	}

	srv := NewServer(link, Options{DBPath: dbPath})
	request := httptest.NewRequest(http.MethodPost, "/api/sessions/rename", strings.NewReader(`{"sessionId":"uuid1","name":"NewName"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	srv.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "source root mismatch") {
		t.Fatalf("symlink target switch must reject rename before lookup: status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.ReadFile(piFile); err != nil {
		t.Fatalf("switched target file must remain untouched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetB, "project", "NewName_uuid1.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("switched target must not be renamed: %v", err)
	}
}

func TestServerRenameReportsRefreshFailureAfterFilesystemRename(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("HOME", t.TempDir())
	piDir := t.TempDir()
	piFile := filepath.Join(piDir, "project", "PiSession_uuid1.jsonl")
	if err := os.MkdirAll(filepath.Dir(piFile), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"session","id":"uuid1","timestamp":"2026-08-01T10:00:00Z","cwd":"/pi/project"}
{"type":"message","id":"request-1","timestamp":"2026-08-01T10:05:00Z","message":{"role":"assistant","model":"m1","usage":{"input":10,"output":5}}}
`
	if err := os.WriteFile(piFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-10 * time.Minute)
	if err := os.Chtimes(piFile, stale, stale); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	srv := NewServer(piDir, Options{CodexDir: t.TempDir(), DBPath: dbPath})
	mustRefreshNow(t, srv)
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.DB.Exec(`CREATE TRIGGER fail_rename_refresh BEFORE INSERT ON session_log_sync BEGIN SELECT RAISE(ABORT, 'injected rename refresh failure'); END`); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/sessions/rename", strings.NewReader(`{"sessionId":"uuid1","name":"NewName"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	srv.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "文件已改名") || !strings.Contains(response.Body.String(), "snapshot refresh failed") {
		t.Fatalf("rename refresh failure must be explicit: status=%d body=%s", response.Code, response.Body.String())
	}

	newPath := filepath.Join(piDir, "project", "NewName_uuid1.jsonl")
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("filesystem rename must be kept after refresh failure: %v", err)
	}
	if _, err := os.Stat(piFile); !os.IsNotExist(err) {
		t.Fatalf("old path must stay renamed, err=%v", err)
	}
}
