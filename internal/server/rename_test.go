package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heihei0299/token-analyzer/internal/db"
)

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
