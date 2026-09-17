package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestPaginationOverflowAndEmptyPagesAreSafe(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("HOME", t.TempDir())
	piDir := t.TempDir()
	project := filepath.Join(piDir, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("uuid%d", i)
		content := fmt.Sprintf(`{"type":"session","id":%q,"timestamp":"2026-09-17T10:%02d:00Z","cwd":"/pi"}
{"type":"message","id":%q,"timestamp":"2026-09-17T10:%02d:30Z","message":{"role":"assistant","model":"m1","usage":{"input":10,"output":5},"stopReason":"stop"}}
`, id, i, id+"-request", i)
		if err := os.WriteFile(filepath.Join(project, "session_"+id+".jsonl"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	srv := NewServer(piDir, Options{CodexDir: t.TempDir(), DBPath: filepath.Join(t.TempDir(), "ledger.db")})
	mustRefreshNow(t, srv)
	maxInt := int(^uint(0) >> 1)
	for _, view := range []string{"sessions", "requests"} {
		for _, target := range []string{
			"/api/" + view + "?page=1",
			"/api/" + view + "?size=2",
			"/api/" + view + "?page=0&size=0",
			"/api/" + view + "?page=-1&size=2",
			"/api/" + view + "?page=1&size=0",
		} {
			response := httptest.NewRecorder()
			srv.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("%s returned %d, want 400: %s", target, response.Code, response.Body.String())
			}
		}

		for _, test := range []struct {
			page, size int
			rows       int
		}{
			{page: 1, size: 2, rows: 2},
			{page: 2, size: 2, rows: 1},
			{page: 1, size: maxInt, rows: 3},
			{page: 2, size: maxInt, rows: 0},
			{page: maxInt / 2, size: 3, rows: 0},
			{page: maxInt, size: maxInt, rows: 0},
		} {
			target := "/api/" + view + "?page=" + strconv.Itoa(test.page) + "&size=" + strconv.Itoa(test.size)
			response := httptest.NewRecorder()
			srv.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("%s returned %d: %s", target, response.Code, response.Body.String())
			}
			var result struct {
				Rows  []json.RawMessage `json:"rows"`
				Total int               `json:"total"`
				Page  int               `json:"page"`
				Size  int               `json:"size"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatalf("%s returned invalid JSON: %v", target, err)
			}
			if len(result.Rows) != test.rows || result.Total != 3 || result.Page != test.page || result.Size != test.size {
				t.Fatalf("%s returned rows=%d total=%d page=%d size=%d, want rows=%d total=3 page=%d size=%d", target, len(result.Rows), result.Total, result.Page, result.Size, test.rows, test.page, test.size)
			}
			if test.page == 1 && test.size == 2 {
				repeated := httptest.NewRecorder()
				srv.Handler().ServeHTTP(repeated, httptest.NewRequest(http.MethodGet, target, nil))
				if repeated.Code != http.StatusOK || repeated.Body.String() != response.Body.String() {
					t.Fatalf("repeated %s changed pagination response", target)
				}
			}
		}

		target := "/api/" + view + "?sortKey=sessionId&sortDir=asc&page=1&size=2"
		response := httptest.NewRecorder()
		srv.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", target, response.Code, response.Body.String())
		}
		var sorted struct {
			Rows []map[string]any `json:"rows"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &sorted); err != nil {
			t.Fatal(err)
		}
		if len(sorted.Rows) != 2 || sorted.Rows[0]["sessionId"] != "uuid1" {
			t.Fatalf("%s did not preserve explicit ordering: %+v", target, sorted.Rows)
		}
	}
}
