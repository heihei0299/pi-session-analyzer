package pi

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/db"
)

func TestPiDiagnosticsReplayOnCursorHit(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	path := writePiFile(t.TempDir(), "diagnostics.jsonl", `{"type":"session","id":"diagnostics","timestamp":"2026-09-10T00:00:00Z","cwd":"/tmp"}
not-json
{"type":"message","id":"valid","timestamp":"2026-09-10T01:00:00Z","message":{"role":"assistant","model":"m","usage":{"input":2,"output":3},"stopReason":"stop"}}
`)
	first, err := SyncPiUsage(database, []string{path})
	if err != nil || first.Imported != 1 || !strings.Contains(strings.Join(first.Diagnostics.Warnings, "\n"), "坏 JSON 行") {
		t.Fatalf("first sync must import valid usage and expose malformed record: %+v %v", first, err)
	}
	second, err := SyncPiUsage(database, []string{path})
	if err != nil || second.Imported != 0 || !strings.Contains(strings.Join(second.Diagnostics.Warnings, "\n"), "坏 JSON 行") {
		t.Fatalf("cursor hit must replay the file diagnostic: %+v %v", second, err)
	}
	var offset int64
	var summary string
	if err := database.DB.QueryRow(`SELECT last_byte_offset, diagnostics_summary FROM session_log_sync WHERE file_path = ?`, path).Scan(&offset, &summary); err != nil {
		t.Fatal(err)
	}
	if offset == 0 || !strings.Contains(summary, "坏 JSON 行") {
		t.Fatalf("diagnostic persistence must not advance a zero cursor: offset=%d summary=%s", offset, summary)
	}
}

func TestPiPricingReadFailureIsReturned(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.DB.Exec(`DROP TABLE model_pricing`); err != nil {
		t.Fatal(err)
	}
	_, err = SyncPiUsage(database, nil)
	if err == nil || !strings.Contains(err.Error(), "定价表") {
		t.Fatalf("pricing table read failure must be observable: %v", err)
	}
}
