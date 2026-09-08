package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-session-anylize/internal/sessiondata"
)

func TestServerCodexSourceQueries(t *testing.T) {
	codexHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(codexHome, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "sessions", "rollout-2026-09-08T12-00-00Z-thread-1.jsonl"), []byte(`{"timestamp":"2026-09-08T12:00:00Z","type":"session_meta","payload":{"session_id":"s1","id":"t1","cwd":"/workspace","model_provider":"openai"}}
{"timestamp":"2026-09-08T12:00:01Z","type":"token_usage_record","payload":{"response_id":"r1","usage":{"input_tokens":10,"output_tokens":5}}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(t.TempDir(), sessiondata.NewSessionData(), Options{Source: "codex", CodexDir: codexHome, DBPath: filepath.Join(t.TempDir(), "ledger.db")})
	handler := srv.Handler()
	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if index.Code != http.StatusOK || !strings.Contains(index.Body.String(), "source-selector") || !strings.Contains(index.Body.String(), "Codex") {
		t.Fatalf("WebUI source selector missing: status=%d", index.Code)
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/totals", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("totals status=%d body=%s", w.Code, w.Body.String())
	}
	var totals struct {
		Totals struct {
			Requests    int     `json:"requests"`
			TotalTokens float64 `json:"totalTokens"`
			CostStatus  string  `json:"costStatus"`
		} `json:"totals"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &totals); err != nil {
		t.Fatal(err)
	}
	if totals.Totals.Requests != 1 || totals.Totals.TotalTokens != 15 || totals.Totals.CostStatus != "unpriced" {
		t.Fatalf("unexpected totals: %+v", totals)
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/sessions", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"source":"codex"`) {
		t.Fatalf("sessions source missing: status=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/requests", nil))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "does not support requests") {
		t.Fatalf("requests should be unsupported: status=%d body=%s", w.Code, w.Body.String())
	}
}
