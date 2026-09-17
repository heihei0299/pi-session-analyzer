package pi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/db"
)

func appendPiLine(path, line string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}

func TestPiCommitFailurePersistsDiagnosticsThroughIndependentConnection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	dir := t.TempDir()
	path := writePiFile(dir, "commit-failure.jsonl", `{"type":"session","id":"commit-failure","timestamp":"2026-09-17T10:00:00Z","cwd":"/tmp"}
{"type":"message","id":"first","timestamp":"2026-09-17T10:01:00Z","message":{"role":"assistant","model":"m","usage":{"input":2,"output":3},"stopReason":"stop"}}
`)
	if _, err := SyncPiUsage(database, []string{path}); err != nil {
		t.Fatalf("baseline import must succeed: %v", err)
	}
	if err := appendPiLine(path, `{"type":"message","id":"second","timestamp":"2026-09-17T10:02:00Z","message":{"role":"assistant","model":"m","usage":{"input":5,"output":7},"stopReason":"stop"}}`); err != nil {
		t.Fatal(err)
	}
	injectCommitFailureForTest = func(file string) bool { return file == path }
	defer func() { injectCommitFailureForTest = nil }()
	if _, err := SyncPiUsage(database, []string{path}); err == nil || !strings.Contains(err.Error(), "提交") {
		t.Fatalf("commit failure must be returned: %v", err)
	}
	injectCommitFailureForTest = nil
	stored, err := LoadDiagnostics(database, dir)
	if err != nil || !strings.Contains(strings.Join(stored.Warnings, "\n"), "提交") {
		t.Fatalf("commit failure must be replayable through LoadDiagnostics: %+v %v", stored, err)
	}
	var rows, cursor int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM proxy_request_logs WHERE session_id = 'commit-failure'`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("commit failure must retain old valid rows: rows=%d err=%v", rows, err)
	}
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM session_log_sync WHERE file_path = ? AND last_byte_offset > 0`, path).Scan(&cursor); err != nil || cursor != 1 {
		t.Fatalf("commit failure must retain the old cursor: cursor=%d err=%v", cursor, err)
	}
	result, err := SyncPiUsage(database, []string{path})
	if err != nil || result.Imported != 1 {
		t.Fatalf("retry must import the pending row exactly once: %+v %v", result, err)
	}
	again, err := SyncPiUsage(database, []string{path})
	if err != nil || again.Imported != 0 {
		t.Fatalf("repeated retry must remain idempotent: %+v %v", again, err)
	}
	stored, err = LoadDiagnostics(database, dir)
	if err != nil || !strings.Contains(strings.Join(stored.Warnings, "\n"), "提交") {
		t.Fatalf("retry must retain the commit diagnostic: %+v %v", stored, err)
	}
}

func TestPiMutationFailurePersistsDiagnosticsAndRetries(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	dir := t.TempDir()
	path := writePiFile(dir, "mutation-failure.jsonl", `{"type":"session","id":"mutation-failure","timestamp":"2026-09-17T10:00:00Z","cwd":"/tmp"}
{"type":"message","id":"valid","timestamp":"2026-09-17T10:01:00Z","message":{"role":"assistant","model":"m","usage":{"input":2,"output":3},"stopReason":"stop"}}
`)
	if _, err := database.DB.Exec(`CREATE TRIGGER fail_pi_usage BEFORE INSERT ON proxy_request_logs BEGIN SELECT RAISE(ABORT, 'injected usage failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncPiUsage(database, []string{path}); err == nil {
		t.Fatal("mutation failure must be returned")
	}
	stored, err := LoadDiagnostics(database, dir)
	if err != nil || !strings.Contains(strings.Join(stored.Warnings, "\n"), "同步") {
		t.Fatalf("mutation failure must be persisted as diagnostics: %+v %v", stored, err)
	}
	if _, err := database.DB.Exec(`DROP TRIGGER fail_pi_usage`); err != nil {
		t.Fatal(err)
	}
	result, err := SyncPiUsage(database, []string{path})
	if err != nil || result.Imported != 1 {
		t.Fatalf("retry must import the valid row exactly once: %+v %v", result, err)
	}
	again, err := SyncPiUsage(database, []string{path})
	if err != nil || again.Imported != 0 {
		t.Fatalf("repeated retry must remain idempotent: %+v %v", again, err)
	}
	stored, err = LoadDiagnostics(database, dir)
	if err != nil || !strings.Contains(strings.Join(stored.Warnings, "\n"), "同步") {
		t.Fatalf("retry must retain the failure diagnostic: %+v %v", stored, err)
	}
}
