package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/db"
)

func TestUnreadableRolloutIsReturnedAndRetryable(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "sessions", "2026", "09", "08")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	base := "rollout-2026-09-08T12-00-00-00000000-0000-7000-8000-000000000099"
	compressed := filepath.Join(root, base+".jsonl.zst")
	if err := os.WriteFile(compressed, []byte("not zstd"), 0o644); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if _, err := SyncRollouts(database, home); err == nil || !strings.Contains(err.Error(), "读取") {
		t.Fatalf("unreadable rollout must fail refresh: %v", err)
	}
	var offset int64
	if err := database.DB.QueryRow(`SELECT last_byte_offset FROM session_log_sync WHERE file_path = ?`, compressed).Scan(&offset); err != nil {
		t.Fatal(err)
	}
	if offset != 0 {
		t.Fatalf("failed rollout must not advance its cursor: %d", offset)
	}

	if err := os.Remove(compressed); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(root, base+".jsonl")
	content := `{"timestamp":"2026-09-08T12:00:00Z","type":"session_meta","payload":{"session_id":"retry-session","id":"retry-thread","cwd":"/tmp"}}
{"timestamp":"2026-09-08T12:00:01Z","type":"token_usage_record","payload":{"response_id":"retry-response","usage":{"input_tokens":10,"output_tokens":5}}}
`
	if err := os.WriteFile(plain, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := SyncRollouts(database, home)
	if err != nil || result.Imported != 1 {
		t.Fatalf("valid replacement must be retryable: %+v %v", result, err)
	}
	var count int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM proxy_request_logs WHERE data_source = 'codex'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("retry must import exactly one usage row: %d", count)
	}
}
