package query

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/refresh"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

func TestPiDiagnosticsAreVisibleThroughQueryMeta(t *testing.T) {
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("TOKEN_ANALYZER_DB", "")
	piDir := t.TempDir()
	project := filepath.Join(piDir, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"type":"session","id":"diagnostics","timestamp":"2026-09-10T00:00:00Z","cwd":"/tmp"}
not-json
{"type":"message","id":"valid","timestamp":"2026-09-10T01:00:00Z","message":{"role":"assistant","model":"m","usage":{"input":2,"output":3},"stopReason":"stop"}}
`
	if err := os.WriteFile(filepath.Join(project, "diagnostics.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{PiDir: piDir, DBPath: filepath.Join(t.TempDir(), "ledger.db"), Source: "pi"}
	if err := refresh.Refresh(refresh.Config{PiDir: piDir, DBPath: cfg.DBPath, Source: "pi"}); err != nil {
		t.Fatal(err)
	}
	for _, view := range []sessiondata.View{{Kind: sessiondata.ViewMeta}, {Kind: sessiondata.ViewTotals}} {
		res, err := Query(cfg, sessiondata.Filter{}, view)
		if err != nil {
			t.Fatal(err)
		}
		if res.Meta == nil || !strings.Contains(strings.Join(res.Meta.Warnings, "\n"), "坏 JSON 行") {
			t.Fatalf("query view %s must expose Pi diagnostics: %+v", view.Kind, res)
		}
	}
}
