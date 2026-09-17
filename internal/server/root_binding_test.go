package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/db"
)

func TestServerDetailMapsRootConflictAndKeepsNotFound(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("HOME", t.TempDir())
	boundRoot := t.TempDir()
	requestedRoot := t.TempDir()
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

	conflicting := NewServer(requestedRoot, Options{DBPath: dbPath})
	for _, target := range []string{
		"/api/sessions/detail?sessionId=missing",
		"/api/sessions/missing/detail",
	} {
		w := httptest.NewRecorder()
		conflicting.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "source root mismatch") {
			t.Fatalf("%s must map root conflict to 409: status=%d body=%s", target, w.Code, w.Body.String())
		}
	}

	matching := NewServer(boundRoot, Options{DBPath: dbPath})
	for _, target := range []string{
		"/api/sessions/detail?sessionId=missing",
		"/api/sessions/missing/detail",
	} {
		w := httptest.NewRecorder()
		matching.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "Not Found") {
			t.Fatalf("%s must preserve missing-session 404: status=%d body=%s", target, w.Code, w.Body.String())
		}
	}

	if _, err := os.Stat(filepath.Join(boundRoot, "unexpected-write")); !os.IsNotExist(err) {
		t.Fatalf("detail lookup must not write Pi root: %v", err)
	}
}
