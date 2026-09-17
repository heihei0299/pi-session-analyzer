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

func TestServerRejectsInvalidQueryParametersBeforeOpeningLedger(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	srv := NewServer(t.TempDir(), Options{DBPath: dbPath})
	for _, target := range []string{
		"/api/totals?source=unknown",
		"/api/sessions?page=bad&size=10",
		"/api/sessions?page=0&size=10",
		"/api/sessions?page=1&size=10&sortDir=sideways",
		"/api/groups?by=nope",
		"/api/period?period=nope",
	} {
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s returned %d: %s", target, w.Code, w.Body.String())
		}
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("invalid requests must not create a ledger: %v", err)
	}
}

func TestAggregateHandlersRejectUnsupportedQueryOptions(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	piDir := t.TempDir()
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.BindSourceRoot(database, "pi", piDir); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(piDir, Options{DBPath: dbPath})
	for _, target := range []string{
		"/api/totals?page=1&size=10",
		"/api/groups?by=model&sortKey=model",
		"/api/period?period=day&sortDir=asc",
		"/api/meta?page=1&size=10",
	} {
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s returned %d, want 400: %s", target, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "unsupported query option") {
			t.Fatalf("%s returned unstable error: %s", target, w.Body.String())
		}
	}
}

func TestRenameRejectsOversizedAndEmptyNamesBeforeFilesystemWork(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	srv := NewServer(t.TempDir(), Options{DBPath: dbPath})
	largeName := strings.Repeat("a", int(maxRenameBodyBytes))
	largeBody := `{"sessionId":"missing","name":"` + largeName + `"}`
	for _, body := range []string{largeBody, `{"sessionId":"missing","name":"////"}`} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sessions/rename", strings.NewReader(body))
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("rename body returned %d: %s", w.Code, w.Body.String())
		}
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("invalid rename requests must not create a ledger: %v", err)
	}
}
