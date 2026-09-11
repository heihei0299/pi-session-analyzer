package codex

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-session-anylize/internal/db"
)

func TestSyncRolloutsWritesCodexLedgerIdempotently(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "sessions", "2026", "09", "08")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "rollout-2026-09-08T12-00-00-00000000-0000-7000-8000-000000000001.jsonl")
	base := `{"timestamp":"2026-09-08T12:00:00Z","type":"session_meta","payload":{"session_id":"root-1","id":"thread-1","cwd":"/workspace","model_provider":"openai"}}
{"timestamp":"2026-09-08T12:00:01Z","type":"token_usage_record","payload":{"response_id":"resp-1","turn_id":"turn-1","usage":{"input_tokens":10,"cached_input_tokens":2,"output_tokens":4,"total_tokens":14}}}
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
	// 账本 input 记非缓存输入（10 - 2），cacheRead 记 cached_input_tokens。
	if provider != "openai" || model != "unknown" || session != "thread-1" || cwd != "/workspace" || pricing != "unpriced" || input != 8 || cacheRead != 2 || output != 4 {
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

// 游标命中时不再重扫文件，但覆盖率诊断必须重放，否则漏算提示只在首次查询出现。
func TestSyncRolloutsReplaysFileDiagnosticsWhenCursorMatches(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, "sessions", "2026", "09", "08")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "rollout-2026-09-08T13-00-00-00000000-0000-7000-8000-000000000004.jsonl")
	content := `{"timestamp":"2026-09-08T13:00:00Z","type":"session_meta","payload":{"session_id":"snapshot-session","id":"snapshot-thread"}}
{"timestamp":"2026-09-08T13:00:01Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"output_tokens":20}}}}
{"timestamp":"2026-09-08T13:00:02Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":200,"output_tokens":40}}}}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	for run := 1; run <= 2; run++ {
		result, err := SyncRollouts(database, home)
		if err != nil {
			t.Fatal(err)
		}
		if result.Imported != 0 {
			t.Fatalf("run %d: cumulative snapshots must never become usage rows: %+v", run, result)
		}
		if result.Diagnostics.UncountedSnapshots != 2 {
			t.Fatalf("run %d: uncounted snapshot coverage must survive a cursor hit: %+v", run, result.Diagnostics)
		}
		if !strings.Contains(strings.Join(result.Diagnostics.Warnings, "\n"), "未计入统计") {
			t.Fatalf("run %d: coverage warning must be replayed: %+v", run, result.Diagnostics.Warnings)
		}
	}
}
