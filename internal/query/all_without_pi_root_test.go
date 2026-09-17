package query

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/heihei0299/token-analyzer/internal/db"
	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/refresh"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

func TestAllWithoutPiRootRejectsLegacyPiHistory(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("HOME", t.TempDir())
	dbPath := filepath.Join(t.TempDir(), "ledger.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.Exec(`INSERT INTO pi_sessions (session_id, header_ts, cwd, file_name, display_name) VALUES ('legacy-session', '2026-09-10T00:00:00Z', '/legacy', 'legacy.jsonl', 'legacy')`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if _, err := database.DB.Exec(`DELETE FROM source_root_bindings WHERE data_source = 'pi'`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := Config{PiDir: "", CodexDir: t.TempDir(), DBPath: dbPath, Source: "all"}
	if err := refresh.Refresh(refresh.Config{PiDir: cfg.PiDir, CodexDir: cfg.CodexDir, DBPath: cfg.DBPath, Source: cfg.Source}); !errors.Is(err, db.ErrSourceRootBindingRequired) {
		t.Fatalf("legacy Pi history without binding must fail closed in Refresh: %v", err)
	}
	if _, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals}); !errors.Is(err, db.ErrSourceRootBindingRequired) {
		t.Fatalf("legacy Pi history without binding must fail closed in Query: %v", err)
	}
	check, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var count int
	if err := check.DB.QueryRow(`SELECT COUNT(*) FROM pi_sessions WHERE session_id = 'legacy-session'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("legacy history rows must remain unchanged: count=%d err=%v", count, err)
	}
	var bindings int
	if err := check.DB.QueryRow(`SELECT COUNT(*) FROM source_root_bindings WHERE data_source = 'pi'`).Scan(&bindings); err != nil || bindings != 0 {
		t.Fatalf("legacy history must not be auto-claimed: bindings=%d err=%v", bindings, err)
	}
}

func TestAllWithoutPiRootReturnsCodexOnly(t *testing.T) {
	t.Setenv("TOKEN_ANALYZER_DB", "")
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("HOME", t.TempDir())
	codexDir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	rolloutDir := filepath.Join(codexDir, "sessions", now.Format("2006"), now.Format("01"), now.Format("02"))
	if err := os.MkdirAll(rolloutDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rollout := filepath.Join(rolloutDir, "rollout-"+now.Format("2006-01-02T15-04-05")+"-00000000-0000-7000-8000-000000000001.jsonl")
	content := `{"timestamp":"` + now.Format(time.RFC3339) + `","type":"session_meta","payload":{"session_id":"codex-session","id":"codex-thread","cwd":"/workspace","model_provider":"openai"}}
{"timestamp":"` + now.Add(time.Second).Format(time.RFC3339) + `","type":"token_usage_record","payload":{"response_id":"codex-request","usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}}
`
	if err := os.WriteFile(rollout, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{PiDir: "", CodexDir: codexDir, DBPath: filepath.Join(t.TempDir(), "ledger.db"), Source: "all"}
	if err := refresh.Refresh(refresh.Config{PiDir: cfg.PiDir, CodexDir: cfg.CodexDir, DBPath: cfg.DBPath, Source: cfg.Source}); err != nil {
		t.Fatal(err)
	}
	views := []sessiondata.View{
		{Kind: sessiondata.ViewTotals},
		{Kind: sessiondata.ViewSessions},
		{Kind: sessiondata.ViewGroups, By: domain.GroupByModel},
		{Kind: sessiondata.ViewPeriod, Period: domain.PeriodDay},
		{Kind: sessiondata.ViewMeta},
	}
	for _, view := range views {
		result, err := Query(cfg, sessiondata.Filter{}, view)
		if err != nil {
			t.Fatalf("all view %s failed without a Pi root: %v", view.Kind, err)
		}
		if result == nil || result.Meta == nil || result.Meta.SessionCount != 1 {
			t.Fatalf("all view %s must expose Codex-only metadata: %+v", view.Kind, result)
		}
	}
	result, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil || result.Totals == nil || result.Totals.Requests != 1 || result.Totals.TotalTokens != 15 || result.Totals.CostStatus != "unpriced" {
		t.Fatalf("all totals must expose Codex data without Pi binding: %+v %v", result, err)
	}
	sessions, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewSessions})
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := sessions.Rows.([]domain.SessionRow)
	if !ok || len(rows) != 1 || rows[0].Source != "codex" {
		t.Fatalf("all sessions must contain only Codex rows without Pi root: %+v", sessions.Rows)
	}
	if _, err := Query(cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewRequests}); err != ErrRequestsUnsupported {
		t.Fatalf("all requests must remain explicitly unsupported: %v", err)
	}
}
