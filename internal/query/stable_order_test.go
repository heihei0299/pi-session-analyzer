package query

import (
	"path/filepath"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

func TestDefaultSessionOrderIsStableBeforePagination(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	piDir := t.TempDir()
	bindPiRoot(t, database, piDir)
	for _, row := range []struct {
		id, timestamp string
	}{
		{"old", "2026-09-10T00:00:00Z"},
		{"tie-a", "2026-09-12T00:00:00Z"},
		{"tie-b", "2026-09-12T00:00:00Z"},
	} {
		if _, err := database.DB.Exec(`INSERT INTO pi_sessions (session_id, header_ts, cwd, file_name, display_name) VALUES (?, ?, '/tmp', ?, ?)`, row.id, row.timestamp, row.id+".jsonl", row.id); err != nil {
			database.Close()
			t.Fatal(err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := Config{PiDir: piDir, DBPath: dbPath, Source: "pi"}
	page := func(number int) []string {
		result, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewSessions, Page: number, Size: 1})
		if err != nil {
			t.Fatal(err)
		}
		actual, ok := result.Rows.([]domain.SessionRow)
		if !ok || len(actual) != 1 {
			t.Fatalf("unexpected session page: %#v", result.Rows)
		}
		return []string{actual[0].SessionId, actual[0].Timestamp}
	}
	first := page(1)
	second := page(2)
	if first[1] != "2026-09-12T00:00:00Z" || second[1] != "2026-09-12T00:00:00Z" || first[0] == second[0] {
		t.Fatalf("default order must put newest rows first and tie-break them: first=%v second=%v", first, second)
	}
	if got := page(1); got[0] != first[0] || got[1] != first[1] {
		t.Fatalf("repeated page changed order: first=%v repeated=%v", first, got)
	}
}
