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

func TestServerEndpoints(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "token-analyzer-server-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建一个会话文件（修改时间设置为 10 分钟前，避免触发活跃保护）
	sessionFile := filepath.Join(tmpDir, "MySession_uuid123.jsonl")
	content := `{"type":"session","id":"uuid123","timestamp":"2026-08-01T10:00:00Z","cwd":"/my/project"}
{"type":"message","timestamp":"2026-08-01T10:05:00Z","message":{"role":"assistant","model":"gpt-4","usage":{"input":100,"output":50,"cacheRead":20,"cost":{"total":0.01}}}}
`
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write session file: %v", err)
	}
	tenMinAgo := time.Now().Add(-10 * time.Minute)
	_ = os.Chtimes(sessionFile, tenMinAgo, tenMinAgo)

	sd := sessiondata.NewSessionData()
	srv := NewServer(tmpDir, sd)
	handler := srv.Handler()

	// 1. 验证 GET / 返回内嵌 HTML
	reqIndex := httptest.NewRequest("GET", "/", nil)
	wIndex := httptest.NewRecorder()
	handler.ServeHTTP(wIndex, reqIndex)
	if wIndex.Code != http.StatusOK {
		t.Fatalf("expected 200 for index, got %d", wIndex.Code)
	}
	if !strings.Contains(wIndex.Body.String(), "<!DOCTYPE html>") {
		t.Fatalf("expected HTML in index response")
	}

	// 2. 验证 GET /api/totals
	reqTotals := httptest.NewRequest("GET", "/api/totals", nil)
	wTotals := httptest.NewRecorder()
	handler.ServeHTTP(wTotals, reqTotals)
	if wTotals.Code != http.StatusOK {
		t.Fatalf("expected 200 for totals, got %d: %s", wTotals.Code, wTotals.Body.String())
	}
	var resTotals sessiondata.QueryResult
	if err := json.Unmarshal(wTotals.Body.Bytes(), &resTotals); err != nil {
		t.Fatalf("failed to parse totals json: %v", err)
	}
	if resTotals.Totals.Requests != 1 || resTotals.Totals.TotalTokens != 170 { // 100 in + 20 cacheRead + 50 out = 170
		t.Errorf("unexpected totals: %+v", resTotals.Totals)
	}

	// 3. 验证 POST /api/sessions/rename
	renamePayload := `{"sessionId":"uuid123","name":"NewName"}`
	reqRename := httptest.NewRequest("POST", "/api/sessions/rename", strings.NewReader(renamePayload))
	reqRename.Header.Set("Content-Type", "application/json")
	wRename := httptest.NewRecorder()
	handler.ServeHTTP(wRename, reqRename)
	if wRename.Code != http.StatusOK {
		t.Fatalf("expected 200 for rename, got %d: %s", wRename.Code, wRename.Body.String())
	}

	expectedNewPath := filepath.Join(tmpDir, "NewName_uuid123.jsonl")
	if _, err := os.Stat(expectedNewPath); err != nil {
		t.Fatalf("renamed file does not exist: %s", expectedNewPath)
	}

	// 4. OpenCode 已抽离，token-analyzer 不再注册其 API。
	reqOpenCode := httptest.NewRequest("GET", "/api/opencode/costs?year=2026&month=8", nil)
	wOpenCode := httptest.NewRecorder()
	handler.ServeHTTP(wOpenCode, reqOpenCode)
	if wOpenCode.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for extracted OpenCode API, got %d", wOpenCode.Code)
	}

	// 5. 验证 GET /api/db/meta
	reqDbMeta := httptest.NewRequest("GET", "/api/db/meta", nil)
	wDbMeta := httptest.NewRecorder()
	handler.ServeHTTP(wDbMeta, reqDbMeta)
	if wDbMeta.Code != http.StatusOK {
		t.Fatalf("expected 200 for db meta, got %d: %s", wDbMeta.Code, wDbMeta.Body.String())
	}
	var resDbMeta map[string]any
	if err := json.Unmarshal(wDbMeta.Body.Bytes(), &resDbMeta); err != nil {
		t.Fatalf("failed to parse db meta json: %v", err)
	}
	if resDbMeta["schemaVersion"] != float64(2) {
		t.Errorf("expected schemaVersion 2, got %v", resDbMeta["schemaVersion"])
	}
}
