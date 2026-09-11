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
	if err := os.WriteFile(filepath.Join(codexHome, "sessions", "rollout-2026-09-08T12-00-00-00000000-0000-7000-8000-000000000001.jsonl"), []byte(`{"timestamp":"2026-09-08T12:00:00Z","type":"session_meta","payload":{"session_id":"s1","id":"t1","cwd":"/workspace","model_provider":"openai"}}
{"timestamp":"2026-09-08T12:00:01Z","type":"token_usage_record","payload":{"response_id":"r1","usage":{"input_tokens":10,"output_tokens":5}}}
{"timestamp":"2026-09-08T12:00:02Z","type":"unknown_fixture_event","payload":{}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(t.TempDir(), sessiondata.NewSessionData(), Options{Source: "codex", CodexDir: codexHome, DBPath: filepath.Join(t.TempDir(), "ledger.db")})
	handler := srv.Handler()
	index := httptest.NewRecorder()
	handler.ServeHTTP(index, httptest.NewRequest(http.MethodGet, "/", nil))
	if index.Code != http.StatusOK || !strings.Contains(index.Body.String(), "source-selector") || !strings.Contains(index.Body.String(), "Codex") || !strings.Contains(index.Body.String(), `id="diagnostics"`) {
		t.Fatalf("WebUI source selector or diagnostics marker missing: status=%d", index.Code)
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
		Meta struct {
			Warnings []string `json:"warnings"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &totals); err != nil {
		t.Fatal(err)
	}
	if totals.Totals.Requests != 1 || totals.Totals.TotalTokens != 15 || totals.Totals.CostStatus != "unpriced" {
		t.Fatalf("unexpected totals: %+v", totals)
	}
	if len(totals.Meta.Warnings) == 0 || !strings.Contains(strings.Join(totals.Meta.Warnings, "\n"), "未知 Codex event type") {
		t.Fatalf("expected API diagnostics warning: %+v", totals.Meta.Warnings)
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

// 真实同形基线 fixture 经 HTTP 出口必须给出与上游一致的 token 口径，并把「未计入的覆盖率」当作诊断暴露。
func TestServerCodexBaselineFixtureExposesUpstreamSemanticsAndDiagnostics(t *testing.T) {
	srv := NewServer(t.TempDir(), sessiondata.NewSessionData(), Options{
		Source:   "codex",
		CodexDir: filepath.Join("..", "codex", "testdata", "codex-home"),
		DBPath:   filepath.Join(t.TempDir(), "ledger.db"),
	})
	handler := srv.Handler()

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/meta", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("meta status=%d body=%s", w.Code, w.Body.String())
	}
	var meta struct {
		Sources            []string `json:"sources"`
		Warnings           []string `json:"warnings"`
		UncountedSnapshots int      `json:"uncountedSnapshots"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if len(meta.Sources) != 1 || meta.Sources[0] != "codex" {
		t.Fatalf("meta must declare the codex source: %+v", meta)
	}
	if meta.UncountedSnapshots != 2 {
		t.Fatalf("uncounted snapshot coverage must be machine-readable through /api/meta: %+v", meta)
	}
	warnings := strings.Join(meta.Warnings, "\n")
	if !strings.Contains(warnings, "2 条 token_count 快照") || !strings.Contains(warnings, "未计入") {
		t.Fatalf("uncounted coverage must be visible through /api/meta: %+v", meta.Warnings)
	}
	if !strings.Contains(warnings, "notes.jsonl") {
		t.Fatalf("non-canonical file must be visible through /api/meta: %+v", meta.Warnings)
	}

	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/totals?source=codex", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("totals status=%d body=%s", w.Code, w.Body.String())
	}
	var totals struct {
		Totals struct {
			Requests    int     `json:"requests"`
			Input       float64 `json:"input"`
			CacheRead   float64 `json:"cacheRead"`
			TotalTokens float64 `json:"totalTokens"`
			CacheRate   float64 `json:"cacheRate"`
			CostStatus  string  `json:"costStatus"`
		} `json:"totals"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &totals); err != nil {
		t.Fatal(err)
	}
	// 期望值取自 fixture 自行声明的上游口径：totalTokens 14+5+15 = 34，input 18，cacheRead 2，cacheRate 0.1。
	if totals.Totals.Requests != 3 || totals.Totals.TotalTokens != 34 || totals.Totals.Input != 18 || totals.Totals.CacheRead != 2 || totals.Totals.CacheRate != 0.1 {
		t.Fatalf("codex HTTP totals must match upstream-declared usage: %+v", totals.Totals)
	}
	if totals.Totals.CostStatus != "unpriced" {
		t.Fatalf("codex cost must stay unpriced: %+v", totals.Totals)
	}
}
