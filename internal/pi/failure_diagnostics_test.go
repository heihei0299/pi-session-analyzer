package pi

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/db"
)

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
