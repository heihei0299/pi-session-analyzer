package query

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-session-anylize/internal/domain"
	"github.com/heihei0299/pi-session-anylize/internal/sessiondata"
)

func TestQueryCodexAndAllSources(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	rollout := filepath.Join(home, "sessions", "rollout-2026-09-08T12-00-00Z-thread-1.jsonl")
	if err := os.WriteFile(rollout, []byte(`{"timestamp":"2026-09-08T12:00:00Z","type":"session_meta","payload":{"session_id":"s1","id":"t1","cwd":"/workspace","model_provider":"openai"}}
{"timestamp":"2026-09-08T12:00:01Z","type":"token_usage_record","payload":{"response_id":"r1","usage":{"input_tokens":10,"output_tokens":5}}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	piDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(piDir, "pi_s1.jsonl"), []byte(`{"type":"session","id":"pi-1","timestamp":"2026-09-08T12:00:00Z","cwd":"/pi"}
{"type":"message","timestamp":"2026-09-08T12:00:01Z","message":{"role":"assistant","model":"pi-model","usage":{"input":2,"output":3}}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	sd := sessiondata.NewSessionData()
	cfg := Config{PiDir: piDir, CodexDir: home, DBPath: filepath.Join(t.TempDir(), "ledger.db"), Source: "codex"}
	res, err := Query(sd, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	if res.Totals == nil || res.Totals.Requests != 1 || res.Totals.TotalTokens != 15 || res.Totals.CostStatus != "unpriced" {
		t.Fatalf("unexpected codex totals: %+v", res)
	}
	if res.Meta == nil || len(res.Meta.Sources) != 1 || res.Meta.Sources[0] != "codex" {
		t.Fatalf("missing codex source metadata: %+v", res.Meta)
	}

	cfg.Source = "all"
	res, err = Query(sd, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	if res.Totals.Requests != 2 || res.Totals.TotalTokens != 20 {
		t.Fatalf("unexpected all totals: %+v", res.Totals)
	}
	res, err = Query(sd, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewSessions})
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := res.Rows.([]domain.SessionRow)
	if !ok || len(rows) != 2 || rows[0].Source == rows[1].Source {
		t.Fatalf("all sessions should expose both sources: %+v", res.Rows)
	}
	if _, err := Query(sd, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewRequests}); err == nil || err.Error() != ErrRequestsUnsupported.Error() {
		t.Fatalf("expected stable unsupported requests error, got %v", err)
	}
	emptyCodex := t.TempDir()
	cfg.Source = "codex"
	cfg.CodexDir = emptyCodex
	res, err = Query(sd, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	if res.Totals.Requests != 0 {
		t.Fatalf("changing codex directory must not reuse old ledger rows: %+v", res.Totals)
	}
}

// 真实同形基线 fixture（真实命名 + 按日目录 + archived_sessions + revert + 非 canonical 干扰）
// 必须整条路径可见：发现 → 解析 → 账本 → totals/sessions 窗口。
func TestQueryCodexRealShapedFixtureBaseline(t *testing.T) {
	cfg := Config{
		CodexDir: filepath.Join("..", "codex", "testdata", "codex-home"),
		DBPath:   filepath.Join(t.TempDir(), "ledger.db"),
		Source:   "codex",
	}
	sd := sessiondata.NewSessionData()

	totals, err := Query(sd, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewTotals})
	if err != nil {
		t.Fatal(err)
	}
	if totals.Totals == nil || totals.Totals.Requests != 3 || totals.Totals.TotalTokens <= 0 {
		t.Fatalf("real-shaped fixture must produce codex totals: %+v", totals)
	}
	if totals.Totals.CostStatus != "unpriced" {
		t.Fatalf("codex cost must stay unpriced: %+v", totals.Totals)
	}
	if totals.Meta == nil || len(totals.Meta.Sources) != 1 || totals.Meta.Sources[0] != "codex" {
		t.Fatalf("missing codex source metadata: %+v", totals.Meta)
	}
	if !strings.Contains(strings.Join(totals.Meta.Warnings, "\n"), "notes.jsonl") {
		t.Fatalf("non-canonical fixture file must surface as a diagnostic: %+v", totals.Meta.Warnings)
	}

	sessions, err := Query(sd, cfg, sessiondata.Filter{}, sessiondata.View{Kind: sessiondata.ViewSessions})
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := sessions.Rows.([]domain.SessionRow)
	if !ok || len(rows) != 3 {
		t.Fatalf("real-shaped fixture must expose three codex sessions: %+v", sessions.Rows)
	}
	for _, row := range rows {
		if row.Source != "codex" || row.SessionId == "" {
			t.Fatalf("codex session row must carry source and session id: %+v", row)
		}
	}
}
