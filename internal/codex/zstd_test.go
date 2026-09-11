package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestParseRolloutReadsZstd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-2026-09-08T12-00-00-00000000-0000-7000-8000-000000000001.jsonl.zst")
	content := []byte(`{"timestamp":"2026-09-08T12:00:00Z","type":"session_meta","payload":{"session_id":"s1","id":"t1","cwd":"/workspace","model_provider":"openai"}}
{"timestamp":"2026-09-08T12:00:01Z","type":"token_usage_record","payload":{"response_id":"r1","usage":{"input_tokens":3,"output_tokens":2}}}
`)
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	compressed := encoder.EncodeAll(content, nil)
	encoder.Close()
	if err := os.WriteFile(path, compressed, 0o644); err != nil {
		t.Fatal(err)
	}

	parsed, _, err := ParseRollout(RolloutFile{Path: path, PhysicalID: strings.TrimSuffix(path, ".zst"), Compressed: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Usage) != 1 || parsed.Usage[0].Usage.TotalTokens != 5 {
		t.Fatalf("zstd rollout not parsed: %+v", parsed.Usage)
	}
}
