package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRolloutReturnsPartialResultsWithDiagnostics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-2026-09-08T12-00-00-00000000-0000-7000-8000-000000000001.jsonl")
	content := "{" + `"timestamp":"2026-09-08T12:00:00Z","type":"session_meta","payload":{"id":"t1"}}` + "\n" +
		"not-json\n" +
		`{"timestamp":"2026-09-08T12:00:01Z","type":"future_event","payload":{}}` + "\n" +
		`{"timestamp":"2026-09-08T12:00:02Z","type":"token_usage_record","payload":{"response_id":"r1","usage":{"input_tokens":1,"output_tokens":2}}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	parsed, diagnostics, err := ParseRollout(RolloutFile{Path: path, PhysicalID: path})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Usage) != 1 || diagnostics.Skipped != 2 || len(diagnostics.Warnings) != 2 {
		t.Fatalf("expected partial result and two diagnostics: usage=%d diagnostics=%+v", len(parsed.Usage), diagnostics)
	}
}
