package pi

import (
	"bytes"
	"os"
	"path/filepath"
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
