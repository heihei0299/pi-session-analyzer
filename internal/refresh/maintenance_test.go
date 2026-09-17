package refresh

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/heihei0299/token-analyzer/internal/db"
)

// bindPiRoot mirrors production: canonicalize once, then use only the pinned
// binding API.
func bindPiRoot(t *testing.T, database *db.Database, root string) {
	t.Helper()
	canonical, err := db.CanonicalSourceRoot(root)
	if err != nil {
		t.Fatalf("canonicalize Pi root %q: %v", root, err)
	}
	if err := db.BindPinnedSourceRoot(database, "pi", canonical); err != nil {
		t.Fatalf("bind Pi root %q: %v", root, err)
	}
}

func TestRefreshRunsRollupMaintenanceAfterSourceSync(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	piDir := t.TempDir()
	bindPiRoot(t, database, piDir)
	oldAt := time.Now().Add(-31 * 24 * time.Hour)
	if _, err := database.DB.Exec(`INSERT INTO proxy_request_logs (request_id, provider_id, app_type, model, request_model, pricing_model, input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens, total_cost_usd, latency_ms, status_code, session_id, created_at, data_source, kind, cwd, timestamp_text) VALUES ('old', 'provider', 'pi', 'model', 'request-model', 'pricing-model', 4, 5, 1, 2, '0.10', 100, 200, 'session', ?, 'pi_session', 'assistant', '/workspace', ?)`, oldAt.Unix(), oldAt.UTC().Format(time.RFC3339)); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	if err := Refresh(Config{PiDir: piDir, DBPath: dbPath, Source: "pi"}); err != nil {
		t.Fatal(err)
	}
	check, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var rawCount, rollupCount int
	if err := check.DB.QueryRow(`SELECT COUNT(*) FROM proxy_request_logs WHERE request_id = 'old'`).Scan(&rawCount); err != nil {
		t.Fatal(err)
	}
	if err := check.DB.QueryRow(`SELECT COUNT(*) FROM usage_daily_rollups WHERE model = 'model'`).Scan(&rollupCount); err != nil {
		t.Fatal(err)
	}
	if rawCount != 0 || rollupCount != 1 {
		t.Fatalf("refresh must maintain expired usage: raw=%d rollups=%d", rawCount, rollupCount)
	}
}
