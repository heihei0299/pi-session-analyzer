package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenReadOnly(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such.db")
	if _, err := OpenReadOnly(missing); err == nil {
		t.Fatal("read-only open of a missing ledger must fail")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("read-only open must not create the file")
	}

	path := filepath.Join(t.TempDir(), "ledger.db")
	writable, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writable.DB.Exec(`INSERT INTO pi_sessions (session_id, header_ts, cwd, file_name, display_name, is_task) VALUES ('s1','2026-09-10T00:00:00Z','/w','f.jsonl','f',0)`); err != nil {
		t.Fatal(err)
	}
	if err := writable.Close(); err != nil {
		t.Fatal(err)
	}

	readonly, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	var count int
	if err := readonly.DB.QueryRow(`SELECT COUNT(*) FROM pi_sessions`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("read-only snapshot must see committed rows: count=%d err=%v", count, err)
	}
	if _, err := readonly.DB.Exec(`INSERT INTO pi_sessions (session_id) VALUES ('s2')`); err == nil {
		t.Fatal("query_only must reject writes through the read-only handle")
	}
}
