package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-session-anylize/internal/db"
	"github.com/heihei0299/pi-session-anylize/internal/sessiondata"
)

func mustRefreshNow(t *testing.T, srv *Server) {
	t.Helper()
	if err := srv.RefreshNow(); err != nil {
		t.Fatalf("initial refresh: %v", err)
	}
}

// ledgerDump 快照可观测的同步状态：游标表全行 + 各账本行数。
func ledgerDump(t *testing.T, dbPath string) string {
	t.Helper()
	database, err := db.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var sb strings.Builder
	rows, err := database.DB.Query(`SELECT file_path, last_modified, last_line_offset, last_synced_at, last_byte_offset, last_tail_fingerprint, diagnostics_summary FROM session_log_sync ORDER BY file_path`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var fp, summary string
		var a, b, c, d, e int64
		if err := rows.Scan(&fp, &a, &b, &c, &d, &e, &summary); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&sb, "%s|%d|%d|%d|%d|%d|%s\n", fp, a, b, c, d, e, summary)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"proxy_request_logs", "session_usage_dedup", "pi_sessions", "source_sessions"} {
		var n int
		if err := database.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&sb, "%s=%d\n", table, n)
	}
	return sb.String()
}

func getBody(t *testing.T, handler http.Handler, target string) (int, string) {
	t.Helper()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w.Code, w.Body.String()
}

// 连续 GET 只读快照：不推进游标、不改账本；新写入要等下一次 Refresh 才可见。
func TestServerGetsDoNotAdvanceLedger(t *testing.T) {
	piDir := t.TempDir()
	piFile := filepath.Join(piDir, "s1.jsonl")
	if err := os.WriteFile(piFile, []byte(`{"type":"session","id":"s1","timestamp":"2026-09-10T00:00:00Z","cwd":"/w"}
{"type":"message","id":"a1","timestamp":"2026-09-10T01:00:00Z","message":{"role":"assistant","model":"m","usage":{"input":1,"output":2}},"stopReason":"stop"}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	codexHome := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	srv := NewServer(piDir, sessiondata.NewSessionData(), Options{Source: "all", CodexDir: codexHome, DBPath: dbPath})
	mustRefreshNow(t, srv)
	handler := srv.Handler()

	endpoints := []string{"/api/totals?source=all", "/api/sessions?source=all", "/api/meta?source=all", "/api/db/meta"}
	bodies := make([]string, len(endpoints))
	for i, ep := range endpoints {
		code, body := getBody(t, handler, ep)
		if code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", ep, code, body)
		}
		bodies[i] = body
	}
	dump := ledgerDump(t, dbPath)
	if !strings.Contains(bodies[0], `"requests":1`) {
		t.Fatalf("expected one synced request: %s", bodies[0])
	}

	// 连续 GET：响应与账本都纹丝不动。
	for i, ep := range endpoints {
		code, body := getBody(t, handler, ep)
		if code != http.StatusOK || body != bodies[i] {
			t.Fatalf("%s must serve identical snapshot: %q vs %q", ep, bodies[i], body)
		}
	}
	if again := ledgerDump(t, dbPath); again != dump {
		t.Fatalf("GETs must not advance ledger cursors:\n%s\nvs\n%s", dump, again)
	}

	// 源文件追加后、Refresh 之前，快照保持不变。
	f, err := os.OpenFile(piFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("{\"type\":\"message\",\"id\":\"a2\",\"timestamp\":\"2026-09-10T02:00:00Z\",\"message\":{\"role\":\"assistant\",\"model\":\"m\",\"usage\":{\"input\":10,\"output\":20}},\"stopReason\":\"stop\"}\n")
	_ = f.Close()
	code, body := getBody(t, handler, "/api/totals?source=all")
	if code != http.StatusOK || body != bodies[0] {
		t.Fatalf("snapshot must stay stale until refresh: %q vs %q", bodies[0], body)
	}
	if again := ledgerDump(t, dbPath); again != dump {
		t.Fatalf("GET after source change must not sync implicitly:\n%s\nvs\n%s", dump, again)
	}

	// 下一次 Refresh 后新数据可见。
	mustRefreshNow(t, srv)
	code, body = getBody(t, handler, "/api/totals?source=all")
	if code != http.StatusOK || !strings.Contains(body, `"requests":2`) {
		t.Fatalf("refresh must publish appended usage: status=%d body=%s", code, body)
	}
}

// Refresh 失败保留上一成功 snapshot，并经 meta/db-meta 暴露错误。
func TestServerRefreshFailureKeepsSnapshotAndExposesError(t *testing.T) {
	piDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(piDir, "s1.jsonl"), []byte(`{"type":"session","id":"s1","timestamp":"2026-09-10T00:00:00Z","cwd":"/w"}
{"type":"message","id":"a1","timestamp":"2026-09-10T01:00:00Z","message":{"role":"assistant","model":"m","usage":{"input":1,"output":2}},"stopReason":"stop"}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	notDir := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(notDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	srv := NewServer(piDir, sessiondata.NewSessionData(), Options{Source: "all", CodexDir: notDir, DBPath: dbPath})
	if err := srv.RefreshNow(); err == nil {
		t.Fatal("refresh against a non-directory codex home must fail")
	}
	handler := srv.Handler()

	code, body := getBody(t, handler, "/api/totals?source=pi")
	if code != http.StatusOK || !strings.Contains(body, `"requests":1`) {
		t.Fatalf("failed refresh must keep last good snapshot: status=%d body=%s", code, body)
	}
	// pi 源同步成功：pi 查询不被 codex 失败污染。
	if strings.Contains(body, "同步失败") {
		t.Fatalf("pi totals must not carry codex refresh failure: %s", body)
	}
	// 组合源查询经 meta 暴露 codex 失败。
	code, allBody := getBody(t, handler, "/api/totals?source=all")
	if code != http.StatusOK || !strings.Contains(allBody, "Codex 同步失败") {
		t.Fatalf("all totals meta must expose the codex refresh failure: status=%d body=%s", code, allBody)
	}
	code, dbMeta := getBody(t, handler, "/api/db/meta")
	if code != http.StatusOK || !strings.Contains(dbMeta, "lastRefreshError") || strings.Contains(dbMeta, `"lastRefreshError":null`) {
		t.Fatalf("db meta must expose the refresh failure: status=%d body=%s", code, dbMeta)
	}
	dump := ledgerDump(t, dbPath)
	if !strings.Contains(dump, "proxy_request_logs=1") {
		t.Fatalf("pi rows synced before the codex failure must survive:\n%s", dump)
	}
	// 失败后继续 GET 不扩大损伤。
	code, body2 := getBody(t, handler, "/api/totals?source=pi")
	if code != http.StatusOK || body2 != body {
		t.Fatalf("post-failure GETs must serve stable snapshot")
	}
	if again := ledgerDump(t, dbPath); again != dump {
		t.Fatalf("post-failure GETs must not touch the ledger")
	}
}
