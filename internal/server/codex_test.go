package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	// meta.sources 是后端能力声明：Go 版同时提供 pi 与 codex。
	if len(meta.Sources) != 2 || meta.Sources[0] != "pi" || meta.Sources[1] != "codex" {
		t.Fatalf("meta must declare backend capabilities: %+v", meta)
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

// 会话详情与重命名只覆盖 Pi 会话：Codex/All 必须明确 unsupported，既不 404，也不写进 Pi 目录。
func TestServerRejectsDetailAndRenameForNonPiSources(t *testing.T) {
	piDir := t.TempDir()
	piFile := filepath.Join(piDir, "PiSession_uuid1.jsonl")
	content := `{"type":"session","id":"uuid1","timestamp":"2026-08-01T10:00:00Z","cwd":"/pi/project"}
{"type":"message","timestamp":"2026-08-01T10:05:00Z","message":{"role":"assistant","model":"m1","usage":{"input":10,"output":5}}}
`
	if err := os.WriteFile(piFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-10 * time.Minute)
	_ = os.Chtimes(piFile, stale, stale)

	srv := NewServer(piDir, sessiondata.NewSessionData(), Options{
		Source:   "pi",
		CodexDir: filepath.Join("..", "codex", "testdata", "codex-home"),
		DBPath:   filepath.Join(t.TempDir(), "ledger.db"),
	})
	handler := srv.Handler()

	for _, source := range []string{"codex", "all"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/sessions/uuid1/detail?source="+source, nil))
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Unsupported") || !strings.Contains(w.Body.String(), "仅覆盖 Pi 会话") {
			t.Fatalf("source=%s detail must be explicitly unsupported: status=%d body=%s", source, w.Code, w.Body.String())
		}

		body := `{"sessionId":"uuid1","name":"should-not-be-written"}`
		w = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sessions/rename?source="+source, strings.NewReader(body))
		req.Header.Set("content-type", "application/json")
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Unsupported") || !strings.Contains(w.Body.String(), "仅覆盖 Pi 会话") {
			t.Fatalf("source=%s rename must be explicitly unsupported: status=%d body=%s", source, w.Code, w.Body.String())
		}
		if _, err := os.Stat(piFile); err != nil {
			t.Fatalf("source=%s rename must not touch Pi files: %v", source, err)
		}
		if _, err := os.Stat(filepath.Join(piDir, "should-not-be-written.jsonl")); err == nil {
			t.Fatalf("source=%s rename must not create Pi files", source)
		}
	}
	// 来源拒绝必须先于参数校验：Codex/All 请求即使缺 id 或 body 非法，也应先看到 unsupported。
	wMissingID := httptest.NewRecorder()
	handler.ServeHTTP(wMissingID, httptest.NewRequest(http.MethodGet, "/api/sessions/detail?source=codex", nil))
	if wMissingID.Code != http.StatusBadRequest || !strings.Contains(wMissingID.Body.String(), "Unsupported") {
		t.Fatalf("codex detail must report unsupported before missing id: status=%d body=%s", wMissingID.Code, wMissingID.Body.String())
	}
	wBadBody := httptest.NewRecorder()
	reqBadBody := httptest.NewRequest(http.MethodPost, "/api/sessions/rename?source=all", strings.NewReader("{not-json"))
	reqBadBody.Header.Set("content-type", "application/json")
	handler.ServeHTTP(wBadBody, reqBadBody)
	if wBadBody.Code != http.StatusBadRequest || !strings.Contains(wBadBody.Body.String(), "Unsupported") {
		t.Fatalf("all rename must report unsupported before body validation: status=%d body=%s", wBadBody.Code, wBadBody.Body.String())
	}

	// Pi 默认路径不受影响：同一会话的详情照常可读。
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/sessions/uuid1/detail", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"sessionId":"uuid1"`) {
		t.Fatalf("pi detail must keep working: status=%d body=%s", w.Code, w.Body.String())
	}

	// serve --source codex 的默认值同样适用：请求不带 source 也必须拒绝。
	codexDefault := NewServer(piDir, sessiondata.NewSessionData(), Options{
		Source:   "codex",
		CodexDir: filepath.Join("..", "codex", "testdata", "codex-home"),
		DBPath:   filepath.Join(t.TempDir(), "ledger.db"),
	})
	w = httptest.NewRecorder()
	codexDefault.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/sessions/uuid1/detail", nil))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Unsupported") {
		t.Fatalf("serve --source codex must reject detail without an explicit source: status=%d body=%s", w.Code, w.Body.String())
	}

	// 显式 source=pi 时，即使 serve 默认 source=codex，Pi 详情仍必须可用（WebUI 固定携带 source=pi）。
	w = httptest.NewRecorder()
	codexDefault.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/sessions/uuid1/detail?source=pi", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"sessionId":"uuid1"`) {
		t.Fatalf("serve --source codex + explicit source=pi must keep Pi detail working: status=%d body=%s", w.Code, w.Body.String())
	}
}

// 服务端出口必须让 source 切换、能力声明、拒绝路径和空目录诊断全部可观察。
func TestServerSourceSwitchingCapabilitiesAndEmptyDir(t *testing.T) {
	piDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(piDir, "pi_s1.jsonl"), []byte(`{"type":"session","id":"pi-1","timestamp":"2026-09-08T12:00:00Z","cwd":"/pi"}
{"type":"message","timestamp":"2026-09-08T12:00:01Z","message":{"role":"assistant","model":"pi-model","usage":{"input":2,"output":3}}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	codexHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(codexHome, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "sessions", "rollout-2026-09-08T12-00-00-00000000-0000-7000-8000-000000000001.jsonl"), []byte(`{"timestamp":"2026-09-08T12:00:00Z","type":"session_meta","payload":{"session_id":"s1","id":"t1","cwd":"/workspace","model_provider":"openai"}}
{"timestamp":"2026-09-08T12:00:01Z","type":"token_usage_record","payload":{"response_id":"r1","usage":{"input_tokens":10,"output_tokens":5}}}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(piDir, sessiondata.NewSessionData(), Options{
		Source:   "pi",
		CodexDir: codexHome,
		DBPath:   filepath.Join(t.TempDir(), "ledger.db"),
	})
	handler := srv.Handler()

	type totalsPayload struct {
		Totals struct {
			Requests    int     `json:"requests"`
			TotalTokens float64 `json:"totalTokens"`
		} `json:"totals"`
	}
	type metaPayload struct {
		Sources []string `json:"sources"`
	}
	cases := []struct {
		source   string
		requests int
		tokens   float64
	}{
		{"pi", 1, 5},
		{"codex", 1, 15},
		{"all", 2, 20},
	}
	for _, tc := range cases {
		t.Run("switching_"+tc.source, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/totals?source="+tc.source, nil))
			if w.Code != http.StatusOK {
				t.Fatalf("source=%s totals status=%d body=%s", tc.source, w.Code, w.Body.String())
			}
			var payload totalsPayload
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Totals.Requests != tc.requests || payload.Totals.TotalTokens != tc.tokens {
				t.Fatalf("source=%s totals=%+v", tc.source, payload.Totals)
			}

			metaRec := httptest.NewRecorder()
			handler.ServeHTTP(metaRec, httptest.NewRequest(http.MethodGet, "/api/meta?source="+tc.source, nil))
			if metaRec.Code != http.StatusOK {
				t.Fatalf("source=%s meta status=%d body=%s", tc.source, metaRec.Code, metaRec.Body.String())
			}
			var meta metaPayload
			if err := json.Unmarshal(metaRec.Body.Bytes(), &meta); err != nil {
				t.Fatal(err)
			}
			if len(meta.Sources) != 2 || meta.Sources[0] != "pi" || meta.Sources[1] != "codex" {
				t.Fatalf("source=%s meta.sources must declare backend capabilities: %+v", tc.source, meta.Sources)
			}
		})
	}

	for _, source := range []string{"codex", "all"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/requests?source="+source, nil))
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "does not support requests") {
			t.Fatalf("source=%s requests must be explicitly unsupported: status=%d body=%s", source, w.Code, w.Body.String())
		}
	}

	// 空 Codex 目录只应产生诊断警告，不应把整个服务查询变成 5xx。
	emptyHome := t.TempDir()
	emptySrv := NewServer(piDir, sessiondata.NewSessionData(), Options{
		Source:   "codex",
		CodexDir: emptyHome,
		DBPath:   filepath.Join(t.TempDir(), "ledger.db"),
	})
	emptyHandler := emptySrv.Handler()
	w := httptest.NewRecorder()
	emptyHandler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/meta", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("empty codex meta must not fail: status=%d body=%s", w.Code, w.Body.String())
	}
	var meta struct {
		Sources  []string `json:"sources"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(meta.Warnings, "\n"), "未找到 sessions/archived_sessions") {
		t.Fatalf("empty codex dir must produce a readable warning: %+v", meta.Warnings)
	}
	if len(meta.Sources) != 2 || meta.Sources[0] != "pi" || meta.Sources[1] != "codex" {
		t.Fatalf("empty codex meta.sources must still declare backend capabilities: %+v", meta.Sources)
	}
	w = httptest.NewRecorder()
	emptyHandler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/totals", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("empty codex totals must not fail: status=%d body=%s", w.Code, w.Body.String())
	}
}
