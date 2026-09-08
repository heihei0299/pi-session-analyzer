package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRolloutMapsMetadataAndReliableUsageOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-2026-09-08T12-00-00Z-thread-1.jsonl")
	content := `{"timestamp":"2026-09-08T12:00:00Z","type":"session_meta","payload":{"session_id":"root-1","id":"thread-1","cwd":"/workspace","originator":"codex_cli_rs","cli_version":"1.2.3","model_provider":"openai","parent_thread_id":"parent-1","forked_from_id":"base-1","forked_from_ordinal_exclusive":7,"subagent_history_start_ordinal":9,"source":"subagent"}}
{"timestamp":"2026-09-08T12:00:01Z","type":"turn_context","payload":{"turn_id":"turn-1","model":"gpt-5"}}
{"timestamp":"2026-09-08T12:00:02Z","type":"token_usage_record","payload":{"response_id":"resp-1","turn_id":"turn-1","usage":{"input_tokens":10,"cached_input_tokens":2,"cache_write_input_tokens":3,"output_tokens":4,"reasoning_output_tokens":1,"total_tokens":19}}}
{"timestamp":"2026-09-08T12:00:03Z","type":"token_usage_record","payload":{"response_id":"resp-2","usage":{"input_tokens":20,"output_tokens":5,"total_tokens":25}}}
{"timestamp":"2026-09-08T12:00:04Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":999,"output_tokens":999}}}}
{"timestamp":"2026-09-08T12:00:05Z","type":"response_item","payload":{"response_id":"resp-no-usage"}}
{"timestamp":"2026-09-08T12:00:06Z","type":"token_usage_record","payload":{"response_id":"resp-invalid","usage":{"total_tokens":999}}}
{"timestamp":"2026-09-08T12:00:07Z","type":"model_reroute","payload":{"from_model":"gpt-5","to_model":"gpt-5-mini"}}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	parsed, diagnostics, err := ParseRollout(RolloutFile{Path: path, PhysicalID: path})
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics.Skipped != 1 || len(diagnostics.Warnings) != 2 {
		t.Fatalf("expected one unreliable usage diagnostic: %+v", diagnostics)
	}
	if parsed.Meta.SessionID != "root-1" || parsed.Meta.ThreadID != "thread-1" {
		t.Fatalf("metadata identity not mapped: %+v", parsed.Meta)
	}
	if parsed.Meta.Cwd != "/workspace" || parsed.Meta.ModelProvider != "openai" || parsed.Meta.ParentThreadID != "parent-1" {
		t.Fatalf("metadata fields not mapped: %+v", parsed.Meta)
	}
	if parsed.Meta.ForkedFromID != "base-1" || parsed.Meta.ForkedFromOrdinal == nil || *parsed.Meta.ForkedFromOrdinal != 7 || parsed.Meta.SubagentHistoryStartOrdinal == nil || *parsed.Meta.SubagentHistoryStartOrdinal != 9 {
		t.Fatalf("fork metadata not mapped: %+v", parsed.Meta)
	}
	if len(parsed.Usage) != 2 {
		t.Fatalf("expected 2 reliable usage records, got %d: %+v", len(parsed.Usage), parsed.Usage)
	}
	first := parsed.Usage[0]
	if first.ResponseID != "resp-1" || first.Model != "gpt-5" || first.Provider != "openai" {
		t.Fatalf("response attribution not mapped: %+v", first)
	}
	if first.Usage.Input != 10 || first.Usage.CacheRead != 2 || first.Usage.CacheWrite != 3 || first.Usage.Output != 4 || first.Usage.Reasoning != 1 || first.Usage.TotalTokens != 16 {
		t.Fatalf("usage mapping wrong: %+v", first.Usage)
	}
	if first.Timestamp != "2026-09-08T12:00:02Z" || first.CostStatus != "unpriced" {
		t.Fatalf("usage metadata wrong: %+v", first)
	}
	if parsed.Usage[1].Model != "unknown" {
		t.Fatalf("missing model should fall back to unknown: %+v", parsed.Usage[1])
	}
}
