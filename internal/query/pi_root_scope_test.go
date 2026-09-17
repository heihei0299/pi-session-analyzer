package query

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/refresh"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

func TestPiLedgerDoesNotMixRoots(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("TOKEN_ANALYZER_DB", "")
	rootA := writePiRoot(t, "root-a", "session-a", "model-a")
	rootB := writePiRoot(t, "root-b", "session-b", "model-b")
	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	cfgA := Config{PiDir: rootA, DBPath: dbPath, Source: "pi"}
	cfgB := Config{PiDir: rootB, DBPath: dbPath, Source: "pi"}

	if err := refresh.Refresh(refresh.Config{PiDir: rootA, DBPath: dbPath, Source: "pi"}); err != nil {
		t.Fatal(err)
	}
	first, err := Query(cfgA, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewSessions})
	if err != nil {
		t.Fatal(err)
	}
	if rows := first.Rows.([]domain.SessionRow); len(rows) != 1 || rows[0].SessionId != "session-a" {
		t.Fatalf("first Pi root must expose only its session: %+v", first.Rows)
	}

	if err := refresh.Refresh(refresh.Config{PiDir: rootB, DBPath: dbPath, Source: "pi"}); err == nil {
		t.Fatal("a ledger bound to one Pi root must reject a different root")
	}
	if _, err := Query(cfgB, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals}); err == nil {
		t.Fatal("querying a different Pi root must be rejected")
	}
	if _, err := QueryDetail(cfgB, "session-a"); err == nil {
		t.Fatal("query detail for a different Pi root must be rejected")
	}
	stillA, err := Query(cfgA, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	if stillA.Totals == nil || stillA.Totals.Requests != 1 || stillA.Totals.TotalTokens != 3 {
		t.Fatalf("rejecting the second root must preserve the first root history: %+v", stillA.Totals)
	}
}

func TestLegacyPiLedgerWithoutBindingIsNotClaimed(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("TOKEN_ANALYZER_DB", "")
	dbPath := filepath.Join(t.TempDir(), "legacy-v2.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.Exec(`INSERT INTO pi_sessions (session_id, header_ts, cwd, file_name, display_name) VALUES ('legacy-session', '2026-09-10T00:00:00Z', '/legacy', 'legacy.jsonl', 'legacy')`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`DROP TABLE source_root_bindings`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if _, err := raw.Exec(`PRAGMA user_version = 2`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	currentRoot := t.TempDir()
	cfg := Config{PiDir: currentRoot, DBPath: dbPath, Source: "pi"}
	if err := refresh.Refresh(refresh.Config{PiDir: currentRoot, DBPath: dbPath, Source: "pi"}); !errors.Is(err, db.ErrSourceRootBindingRequired) {
		t.Fatalf("legacy Pi rows without a binding must not be silently claimed by Refresh: %v", err)
	}
	if _, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals}); err == nil {
		t.Fatal("legacy Pi rows without a binding must fail closed in Query")
	}
	if _, err := QueryDetail(cfg, "legacy-session"); err == nil {
		t.Fatal("legacy Pi rows without a binding must fail closed in QueryDetail")
	}

	raw, err = sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM pi_sessions WHERE session_id = 'legacy-session'`).Scan(&count); err != nil || count != 1 {
		raw.Close()
		t.Fatalf("legacy Pi history must remain intact: count=%d err=%v", count, err)
	}
	var bindings int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM source_root_bindings WHERE data_source = 'pi'`).Scan(&bindings); err != nil || bindings != 0 {
		raw.Close()
		t.Fatalf("legacy Pi history must not be silently claimed: bindings=%d err=%v", bindings, err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
}

func writePiRoot(t *testing.T, rootName, sessionID, model string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), rootName)
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf(`{"type":"session","id":%q,"timestamp":"2026-09-10T00:00:00Z","cwd":"/%s"}
{"type":"message","id":%q,"timestamp":"2026-09-10T01:00:00Z","message":{"role":"assistant","model":%q,"usage":{"input":1,"output":2}},"stopReason":"stop"}
`, sessionID, rootName, sessionID+"-message", model)
	if err := os.WriteFile(filepath.Join(project, sessionID+".jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
