package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-session-anylize/internal/db"
	"github.com/klauspost/compress/zstd"
)

func TestSyncRolloutsIgnoresHalfLineAndPlainCompressedSwitch(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "sessions", "2026", "09", "08")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(root, "rollout-2026-09-08T12-00-00-00000000-0000-7000-8000-000000000001.jsonl")
	header := `{"timestamp":"2026-09-08T12:00:00Z","type":"session_meta","payload":{"session_id":"s1","id":"t1","cwd":"/workspace","model_provider":"openai"}}
`
	first := `{"timestamp":"2026-09-08T12:00:01Z","type":"token_usage_record","payload":{"response_id":"r1","usage":{"input_tokens":1,"output_tokens":2}}}
`
	second := `{"timestamp":"2026-09-08T12:00:02Z","type":"token_usage_record","payload":{"response_id":"r2","usage":{"input_tokens":3,"output_tokens":4}}}
`
	if err := os.WriteFile(plain, []byte(header+first), 0o644); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if result, err := SyncRollouts(database, home); err != nil || result.Imported != 1 {
		t.Fatalf("initial sync: %+v %v", result, err)
	}

	partial := strings.TrimSuffix(second, "\n")
	f, err := os.OpenFile(plain, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(partial)
	_ = f.Close()
	if result, err := SyncRollouts(database, home); err != nil || result.Imported != 0 {
		t.Fatalf("half-line must wait for newline: %+v %v", result, err)
	}

	f, err = os.OpenFile(plain, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("\n")
	_ = f.Close()
	if result, err := SyncRollouts(database, home); err != nil || result.Imported != 1 {
		t.Fatalf("completed line should import: %+v %v", result, err)
	}

	content, err := os.ReadFile(plain)
	if err != nil {
		t.Fatal(err)
	}
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	compressed := encoder.EncodeAll(content, nil)
	encoder.Close()
	compressedPath := plain + ".zst"
	if err := os.WriteFile(compressedPath, compressed, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(plain); err != nil {
		t.Fatal(err)
	}
	if result, err := SyncRollouts(database, home); err != nil || result.Imported != 0 {
		t.Fatalf("plain/zstd switch must not duplicate: %+v %v", result, err)
	}
	var count int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM proxy_request_logs WHERE data_source = 'codex'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected two unique responses after switch, got %d", count)
	}
}

func TestSyncRolloutsRescansReplacementAndKeepsOldLedger(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "sessions", "2026", "09", "08")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "rollout-2026-09-08T12-00-00-00000000-0000-7000-8000-000000000001.jsonl")
	header := `{"timestamp":"2026-09-08T12:00:00Z","type":"session_meta","payload":{"session_id":"s1","id":"t1","cwd":"/workspace","model_provider":"openai"}}` + "\n"
	response1 := `{"timestamp":"2026-09-08T12:00:01Z","type":"token_usage_record","payload":{"response_id":"r1","usage":{"input_tokens":1,"output_tokens":2}}}` + "\n"
	response2 := `{"timestamp":"2026-09-08T12:00:02Z","type":"token_usage_record","payload":{"response_id":"r2","usage":{"input_tokens":3,"output_tokens":4}}}` + "\n"
	if err := os.WriteFile(path, []byte(header+response1), 0o644); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	initial, err := SyncRollouts(database, home)
	if err != nil || initial.Imported != 1 {
		t.Fatalf("initial sync: %+v %v", initial, err)
	}

	if err := os.WriteFile(path, []byte(header+response2), 0o644); err != nil {
		t.Fatal(err)
	}
	replaced, err := SyncRollouts(database, home)
	if err != nil || replaced.Imported != 1 {
		t.Fatalf("replacement should import the new response: %+v %v", replaced, err)
	}

	if err := os.WriteFile(path, []byte(header), 0o644); err != nil {
		t.Fatal(err)
	}
	truncated, err := SyncRollouts(database, home)
	if err != nil || truncated.Imported != 0 {
		t.Fatalf("truncate must not delete old ledger rows: %+v %v", truncated, err)
	}

	if err := os.WriteFile(path, []byte(header+response1), 0o644); err != nil {
		t.Fatal(err)
	}
	restored, err := SyncRollouts(database, home)
	if err != nil || restored.Imported != 0 {
		t.Fatalf("restored response must remain idempotent: %+v %v", restored, err)
	}
}
