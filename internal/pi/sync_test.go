package pi

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/db"
)

func tempDirPi() string {
	dir, _ := os.MkdirTemp("", "ta-pi-")
	return dir
}

func writePiFile(dir, name, content string) string {
	path := filepath.Join(dir, name)
	os.WriteFile(path, []byte(content), 0644)
	return path
}

func TestPiFileRevisionComplete(t *testing.T) {
	dir := tempDirPi()
	defer os.RemoveAll(dir)
	header := `{"type":"session","version":3,"id":"sess-a","timestamp":"2026-07-31T01:55:30.577Z","cwd":"/tmp"}`
	entry := `{"type":"message","id":"a1","timestamp":"2026-07-31T01:58:29.810Z","message":{"role":"assistant","provider":"p","model":"m","usage":{"input":10,"output":5,"cacheRead":3,"cacheWrite":2,"totalTokens":20,"cost":{"total":0.1}},"stopReason":"stop"}}`
	path := writePiFile(dir, "s.jsonl", header+"\n"+entry+"\n")
	rev, err := PiFileRevisionOf(path)
	if err != nil || !rev.Complete || rev.FileSize == 0 {
		t.Fatalf("revision failed %+v %v", rev, err)
	}
	// append
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString(entry + "\n")
	f.Close()
	rev2, _ := PiFileRevisionOf(path)
	if rev.TailFingerprint == rev2.TailFingerprint {
		t.Fatalf("fingerprint should change after append")
	}
}

func TestPiFileRevisionIncomplete(t *testing.T) {
	dir := tempDirPi()
	defer os.RemoveAll(dir)
	header := `{"type":"session","version":3,"id":"sess-a","timestamp":"2026-07-31T01:55:30.577Z","cwd":"/tmp"}`
	entry := `{"type":"message","id":"a1","timestamp":"2026-07-31T01:58:29.810Z","message":{"role":"assistant","provider":"p","model":"m","usage":{"input":10,"output":5,"cacheRead":3,"cacheWrite":2,"totalTokens":20,"cost":{"total":0.1}},"stopReason":"stop"}}`
	path := writePiFile(dir, "s.jsonl", header+"\n"+entry) // no newline
	rev, _ := PiFileRevisionOf(path)
	if rev.Complete {
		t.Fatalf("should be incomplete without newline")
	}
}

func TestSyncIncrementalLifecycle(t *testing.T) {
	dir := tempDirPi()
	dbDir := tempDirPi()
	defer os.RemoveAll(dir)
	defer os.RemoveAll(dbDir)
	dbPath := filepath.Join(dbDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db %v", err)
	}
	defer database.Close()
	header := `{"type":"session","version":3,"id":"sess-1","timestamp":"2026-07-31T01:55:30.577Z","cwd":"/tmp"}`
	entry1 := `{"type":"message","id":"a1","timestamp":"2026-07-31T01:58:29.810Z","message":{"role":"assistant","provider":"p","model":"m","usage":{"input":10,"output":5,"cacheRead":3,"cacheWrite":2,"totalTokens":20,"cost":{"total":0.1}},"stopReason":"stop"}}`
	entry2 := `{"type":"message","id":"a2","timestamp":"2026-07-31T01:58:30.810Z","message":{"role":"assistant","provider":"p","model":"m","usage":{"input":10,"output":5,"cacheRead":3,"cacheWrite":2,"totalTokens":20,"cost":{"total":0.1}},"stopReason":"stop"}}`
	path := writePiFile(dir, "s.jsonl", header+"\n"+entry1+"\n")
	r1, err := SyncPiUsage(database, []string{path})
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if r1.Imported != 1 {
		t.Fatalf("first import should be 1 got %d", r1.Imported)
	}
	r2, err := SyncPiUsage(database, []string{path})
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if r2.Imported != 0 {
		t.Fatalf("second import should be 0 got %d", r2.Imported)
	}
	// append
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	f.WriteString(entry2 + "\n")
	f.Close()
	r3, err := SyncPiUsage(database, []string{path})
	if err != nil {
		t.Fatalf("append sync: %v", err)
	}
	if r3.Imported != 1 {
		t.Fatalf("third import should be 1 got %d", r3.Imported)
	}
	// partial line must not advance the cursor or import
	f, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for partial: %v", err)
	}
	f.WriteString(`{"type":"message","id":"partial"`)
	f.Close()
	r4, err := SyncPiUsage(database, []string{path})
	if err != nil {
		t.Fatalf("partial sync: %v", err)
	}
	if r4.Imported != 0 {
		t.Fatalf("partial line must not import, got %d", r4.Imported)
	}
	// completing the partial line imports exactly that message
	f, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for completion: %v", err)
	}
	f.WriteString(`,"timestamp":"2026-07-31T01:58:31.810Z","message":{"role":"assistant","provider":"p","model":"m","usage":{"input":1,"output":1,"cacheRead":0,"cacheWrite":0,"reasoning":0,"cost":{"total":0.01}},"stopReason":"stop"}}` + "\n")
	f.Close()
	r5, err := SyncPiUsage(database, []string{path})
	if err != nil {
		t.Fatalf("completion sync: %v", err)
	}
	if r5.Imported != 1 {
		t.Fatalf("completed partial line should import 1, got %d", r5.Imported)
	}
	// same-size replacement keeps idempotency through the ledger:
	// corrupt the trailing newline-adjacent byte so the tail fingerprint changes
	// while the file size stays identical.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat for rewrite: %v", err)
	}
	replacement := make([]byte, info.Size())
	copy(replacement, mustReadFile(t, path))
	replacement[len(replacement)-2] = ' '
	if err := os.WriteFile(path, replacement, 0644); err != nil {
		t.Fatalf("rewrite %v", err)
	}
	r6, err := SyncPiUsage(database, []string{path})
	if err != nil {
		t.Fatalf("rewrite sync: %v", err)
	}
	if r6.Imported != 0 {
		t.Fatalf("same-size rewrite must stay idempotent, got %d", r6.Imported)
	}
	// truncate rescans what remains without double counting
	contents := mustReadFile(t, path)
	newline := bytes.IndexByte(contents, '\n')
	if err := os.WriteFile(path, contents[:newline+1], 0644); err != nil {
		t.Fatalf("truncate %v", err)
	}
	r7, err := SyncPiUsage(database, []string{path})
	if err != nil {
		t.Fatalf("truncate sync: %v", err)
	}
	if r7.Imported != 0 {
		t.Fatalf("truncate rescan must not double count, got %d", r7.Imported)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestPiSyncRollsBackUsageFailureAndRetries(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	header := `{"type":"session","id":"rollback-usage","timestamp":"2026-07-31T01:55:30.577Z","cwd":"/tmp"}`
	entry := `{"type":"message","id":"usage-failure","timestamp":"2026-07-31T01:58:29.810Z","message":{"role":"assistant","provider":"p","model":"m","usage":{"input":10,"output":5},"stopReason":"stop"}}`
	path := writePiFile(dir, "s.jsonl", header+"\n"+entry+"\n")
	if _, err := database.DB.Exec(`CREATE TRIGGER fail_pi_usage BEFORE INSERT ON proxy_request_logs BEGIN SELECT RAISE(ABORT, 'injected usage failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncPiUsage(database, []string{path}); err == nil {
		t.Fatal("usage failure must be returned")
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM proxy_request_logs`,
		`SELECT COUNT(*) FROM session_usage_dedup`,
		`SELECT COUNT(*) FROM pi_sessions`,
	} {
		var count int
		if err := database.DB.QueryRow(query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("failed transaction left rows for %q: %d", query, count)
		}
	}
	// 事务回滚后只允许留 diagnostics-only 行：cursor/semantics 不推进，不伪装成成功 revision。
	var cursor, semantics int
	var summary string
	if err := database.DB.QueryRow(`SELECT last_byte_offset, sync_semantics_version, diagnostics_summary FROM session_log_sync WHERE file_path = ?`, path).Scan(&cursor, &semantics, &summary); err != nil {
		t.Fatal(err)
	}
	if cursor != 0 || semantics != 0 || !strings.Contains(summary, "同步") {
		t.Fatalf("mutation failure must persist diagnostics without a cursor: cursor=%d semantics=%d summary=%s", cursor, semantics, summary)
	}
	if _, err := database.DB.Exec(`DROP TRIGGER fail_pi_usage`); err != nil {
		t.Fatal(err)
	}
	result, err := SyncPiUsage(database, []string{path})
	if err != nil {
		t.Fatalf("retry should succeed: %v", err)
	}
	if result.Imported != 1 {
		t.Fatalf("retry should import exactly one record: %+v", result)
	}
}

func TestPiSyncRollsBackCursorFailureAndRetries(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	header := `{"type":"session","id":"rollback-cursor","timestamp":"2026-07-31T01:55:30.577Z","cwd":"/tmp"}`
	entry := `{"type":"message","id":"cursor-failure","timestamp":"2026-07-31T01:58:29.810Z","message":{"role":"assistant","provider":"p","model":"m","usage":{"input":10,"output":5},"stopReason":"stop"}}`
	path := writePiFile(dir, "s.jsonl", header+"\n"+entry+"\n")
	if _, err := database.DB.Exec(`CREATE TRIGGER fail_pi_cursor BEFORE INSERT ON session_log_sync BEGIN SELECT RAISE(ABORT, 'injected cursor failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncPiUsage(database, []string{path}); err == nil {
		t.Fatal("cursor failure must be returned")
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM proxy_request_logs`,
		`SELECT COUNT(*) FROM session_usage_dedup`,
		`SELECT COUNT(*) FROM pi_sessions`,
		`SELECT COUNT(*) FROM session_log_sync`,
	} {
		var count int
		if err := database.DB.QueryRow(query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("cursor failure left rows for %q: %d", query, count)
		}
	}
	if _, err := database.DB.Exec(`DROP TRIGGER fail_pi_cursor`); err != nil {
		t.Fatal(err)
	}
	result, err := SyncPiUsage(database, []string{path})
	if err != nil {
		t.Fatalf("retry should succeed: %v", err)
	}
	if result.Imported != 1 {
		t.Fatalf("retry should import exactly one record: %+v", result)
	}
	var count int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM proxy_request_logs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("retry should leave one usage row, got %d", count)
	}
}

func TestPiSyncRescansLegacyCursorAndRepairsLedger(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	header := `{"type":"session","id":"legacy-session","timestamp":"2026-07-31T01:55:30.577Z","cwd":"/tmp"}`
	entry := `{"type":"message","id":"legacy-request","timestamp":"2026-07-31T01:58:29.810Z","message":{"role":"assistant","provider":"p","model":"m","usage":{"input":10,"output":10}}}`
	path := writePiFile(dir, "s.jsonl", header+"\n"+entry+"\n")
	if _, err := SyncPiUsage(database, []string{path}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.Exec(`UPDATE proxy_request_logs SET output_tokens = 1 WHERE request_id LIKE 'pi_session:%'`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.Exec(`UPDATE session_log_sync SET sync_semantics_version = 1 WHERE file_path = ?`, path); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncPiUsage(database, []string{path}); err != nil {
		t.Fatal(err)
	}
	var output, count int
	if err := database.DB.QueryRow(`SELECT output_tokens, COUNT(*) FROM proxy_request_logs`).Scan(&output, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 || output != 10 {
		t.Fatalf("legacy cursor must trigger a repair rescan: count=%d output=%d", count, output)
	}
	var version int
	if err := database.DB.QueryRow(`SELECT sync_semantics_version FROM session_log_sync WHERE file_path = ?`, path).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version == 1 {
		t.Fatal("successful repair must write the current sync semantics version")
	}
}

func TestPiSyncStoresPlatformFileBase(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	path := filepath.Join(dir, "platform-session.jsonl")
	content := `{"type":"session","id":"platform-session","timestamp":"2026-07-31T01:55:30.577Z","cwd":"/tmp"}
{"type":"message","id":"platform-request","timestamp":"2026-07-31T01:58:29.810Z","message":{"role":"assistant","provider":"p","model":"m","usage":{"input":1,"output":1}},"stopReason":"stop"}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncPiUsage(database, []string{path}); err != nil {
		t.Fatal(err)
	}
	var fileName, displayName string
	if err := database.DB.QueryRow(`SELECT file_name, display_name FROM pi_sessions WHERE session_id = ?`, "platform-session").Scan(&fileName, &displayName); err != nil {
		t.Fatal(err)
	}
	if fileName != filepath.Base(path) || displayName == path {
		t.Fatalf("Pi metadata must use the platform file base: fileName=%q displayName=%q path=%q", fileName, displayName, path)
	}
}

func TestPiSyncReplacesRequestAcrossRefreshes(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	header := `{"type":"session","id":"replace-session","timestamp":"2026-07-31T01:55:30.577Z","cwd":"/tmp"}`
	partial := `{"type":"message","id":"replace-request","timestamp":"2026-07-31T01:58:29.810Z","message":{"role":"assistant","provider":"p","model":"m","usage":{"input":10,"output":10}}}`
	final := `{"type":"message","id":"replace-request","timestamp":"2026-07-31T01:58:29.810Z","message":{"role":"assistant","provider":"p","model":"m","usage":{"input":10,"output":20},"stopReason":"stop"}}`
	path := writePiFile(dir, "s.jsonl", header+"\n"+partial+"\n")
	first, err := SyncPiUsage(database, []string{path})
	if err != nil || first.Imported != 1 {
		t.Fatalf("initial sync should import one partial record: %+v %v", first, err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(final + "\n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := SyncPiUsage(database, []string{path})
	if err != nil {
		t.Fatalf("replacement sync failed: %v", err)
	}
	if second.Imported != 1 {
		t.Fatalf("replacement should count as one imported update: %+v", second)
	}
	var count, output int
	var stopReason string
	if err := database.DB.QueryRow(`SELECT COUNT(*), COALESCE(MAX(output_tokens), 0), COALESCE(MAX(stop_reason), '') FROM proxy_request_logs`).Scan(&count, &output, &stopReason); err != nil {
		t.Fatal(err)
	}
	if count != 1 || output != 20 || stopReason != "stop" {
		t.Fatalf("ledger should contain the final request once: count=%d output=%d stop=%q", count, output, stopReason)
	}
	var dedupCount int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM session_usage_dedup`).Scan(&dedupCount); err != nil {
		t.Fatal(err)
	}
	if dedupCount != 1 {
		t.Fatalf("replacement must not duplicate dedup rows: %d", dedupCount)
	}
	third, err := SyncPiUsage(database, []string{path})
	if err != nil {
		t.Fatalf("repeated replacement sync failed: %v", err)
	}
	if third.Imported != 0 {
		t.Fatalf("repeated refresh should be idempotent: %+v", third)
	}
}
