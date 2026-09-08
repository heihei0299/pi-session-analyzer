package codex

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/heihei0299/pi-session-anylize/internal/db"
)

func TestSyncRolloutsWritesCodexLedgerIdempotently(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "sessions")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "rollout-2026-09-08T12-00-00Z-thread-1.jsonl")
	base := `{"timestamp":"2026-09-08T12:00:00Z","type":"session_meta","payload":{"session_id":"root-1","id":"thread-1","cwd":"/workspace","model_provider":"openai"}}
{"timestamp":"2026-09-08T12:00:01Z","type":"token_usage_record","payload":{"response_id":"resp-1","turn_id":"turn-1","usage":{"input_tokens":10,"cached_input_tokens":2,"output_tokens":4}}}
{"timestamp":"2026-09-08T12:00:02Z","type":"token_usage_record","payload":{"response_id":"resp-1","turn_id":"turn-1","usage":{"input_tokens":999,"output_tokens":999}}}
`
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	first, err := SyncRollouts(database, home)
	if err != nil {
		t.Fatal(err)
	}
	if first.Imported != 1 || first.Skipped != 1 {
		t.Fatalf("unexpected first sync: %+v", first)
	}
	second, err := SyncRollouts(database, home)
	if err != nil {
		t.Fatal(err)
	}
	if second.Imported != 0 {
		t.Fatalf("same revision must be idempotent: %+v", second)
	}

	var count int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM proxy_request_logs WHERE data_source = 'codex'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one codex ledger row, got %d", count)
	}
	var provider, model, session, cwd, pricing string
	var input, cacheRead, output int
	if err := database.DB.QueryRow(`SELECT provider_id, model, session_id, cwd, pricing_model, input_tokens, cache_read_tokens, output_tokens FROM proxy_request_logs WHERE data_source = 'codex'`).Scan(&provider, &model, &session, &cwd, &pricing, &input, &cacheRead, &output); err != nil {
		t.Fatal(err)
	}
	if provider != "openai" || model != "unknown" || session != "thread-1" || cwd != "/workspace" || pricing != "unpriced" || input != 10 || cacheRead != 2 || output != 4 {
		t.Fatalf("ledger mapping wrong: provider=%q model=%q session=%q cwd=%q pricing=%q usage=%d/%d/%d", provider, model, session, cwd, pricing, input, cacheRead, output)
	}
	var storedSession, storedThread, storedParent, storedFork string
	if err := database.DB.QueryRow(`SELECT session_id, thread_id, parent_thread_id, forked_from_id FROM source_sessions WHERE data_source = 'codex'`).Scan(&storedSession, &storedThread, &storedParent, &storedFork); err != nil {
		t.Fatal(err)
	}
	if storedSession != "root-1" || storedThread != "thread-1" || storedParent != "" || storedFork != "" {
		t.Fatalf("source session metadata wrong: %q %q %q %q", storedSession, storedThread, storedParent, storedFork)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"timestamp":"2026-09-08T12:00:03Z","type":"token_usage_record","payload":{"response_id":"resp-2","usage":{"input_tokens":5,"output_tokens":6}}}` + "\n")
	_ = f.Close()
	third, err := SyncRollouts(database, home)
	if err != nil {
		t.Fatal(err)
	}
	if third.Imported != 1 {
		t.Fatalf("append should import one new response: %+v", third)
	}
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM proxy_request_logs WHERE data_source = 'codex'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected two codex ledger rows after append, got %d", count)
	}

	var syncCount int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM session_log_sync WHERE file_path = ?`, path).Scan(&syncCount); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if syncCount != 1 {
		t.Fatalf("expected one sync cursor, got %d", syncCount)
	}
}
