package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRefreshStatusSnapshotIsSafeDuringConcurrentReads(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("HOME", t.TempDir())
	piDir := t.TempDir()
	piFile := filepath.Join(piDir, "project", "session_uuid1.jsonl")
	if err := os.MkdirAll(filepath.Dir(piFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(piFile, []byte(`{"type":"session","id":"uuid1","timestamp":"2026-09-17T10:00:00Z","cwd":"/pi"}
{"type":"message","id":"request-1","timestamp":"2026-09-17T10:05:00Z","message":{"role":"assistant","model":"m1","usage":{"input":10,"output":5},"stopReason":"stop"}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	notDir := filepath.Join(t.TempDir(), "not-a-codex-home")
	if err := os.WriteFile(notDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(piDir, Options{Source: "all", CodexDir: notDir, DBPath: filepath.Join(t.TempDir(), "ledger.db")})
	if err := srv.RefreshNow(); err == nil {
		t.Fatal("the invalid Codex home must produce an observable refresh error")
	}
	if code, body := getBody(t, srv.Handler(), "/api/meta?source=pi"); code != http.StatusOK || strings.Contains(body, "Codex 同步失败") {
		t.Fatalf("Pi metadata must filter out Codex refresh errors: status=%d body=%s", code, body)
	}
	if code, body := getBody(t, srv.Handler(), "/api/meta?source=codex"); code != http.StatusOK || !strings.Contains(body, "Codex 同步失败") {
		t.Fatalf("Codex metadata must expose its refresh error: status=%d body=%s", code, body)
	}
	if code, body := getBody(t, srv.Handler(), "/api/meta?source=all"); code != http.StatusOK || !strings.Contains(body, "Codex 同步失败") {
		t.Fatalf("all metadata must expose the Codex refresh error: status=%d body=%s", code, body)
	}

	stop := srv.StartWatch(time.Millisecond)
	defer stop()
	file, err := os.OpenFile(piFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"type":"message","id":"request-2","timestamp":"2026-09-17T10:06:00Z","message":{"role":"assistant","model":"m1","usage":{"input":3,"output":4},"stopReason":"stop"}}
`); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 12; j++ {
				_ = srv.RefreshNow()
			}
		}()
	}
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				for _, target := range []string{"/api/meta?source=pi", "/api/meta?source=codex", "/api/meta?source=all", "/api/db/meta"} {
					response := httptest.NewRecorder()
					srv.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
					if response.Code != http.StatusOK {
						t.Errorf("%s returned %d: %s", target, response.Code, response.Body.String())
					}
				}
			}
		}()
	}
	wg.Wait()
}
