package pi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/heihei0299/pi-session-anylize/internal/db"
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

func TestSyncIncremental(t *testing.T) {
	dir := tempDirPi()
	dbDir := tempDirPi()
	defer os.RemoveAll(dir)
	defer os.RemoveAll(dbDir)
	dbPath := filepath.Join(dbDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil { t.Fatalf("open db %v", err) }
	defer database.Close()
	header := `{"type":"session","version":3,"id":"sess-1","timestamp":"2026-07-31T01:55:30.577Z","cwd":"/tmp"}`
	entry1 := `{"type":"message","id":"a1","timestamp":"2026-07-31T01:58:29.810Z","message":{"role":"assistant","provider":"p","model":"m","usage":{"input":10,"output":5,"cacheRead":3,"cacheWrite":2,"totalTokens":20,"cost":{"total":0.1}},"stopReason":"stop"}}`
	entry2 := `{"type":"message","id":"a2","timestamp":"2026-07-31T01:58:30.810Z","message":{"role":"assistant","provider":"p","model":"m","usage":{"input":10,"output":5,"cacheRead":3,"cacheWrite":2,"totalTokens":20,"cost":{"total":0.1}},"stopReason":"stop"}}`
	path := writePiFile(dir, "s.jsonl", header+"\n"+entry1+"\n")
	r1, _ := SyncPiUsage(database, []string{path})
	if r1.Imported != 1 { t.Fatalf("first import should be 1 got %d", r1.Imported) }
	r2, _ := SyncPiUsage(database, []string{path})
	if r2.Imported != 0 { t.Fatalf("second import should be 0 got %d", r2.Imported) }
	// append
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString(entry2 + "\n")
	f.Close()
	r3, _ := SyncPiUsage(database, []string{path})
	if r3.Imported != 1 { t.Fatalf("third import should be 1 got %d", r3.Imported) }
}
