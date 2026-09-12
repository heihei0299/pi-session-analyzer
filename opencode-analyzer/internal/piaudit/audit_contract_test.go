package piaudit

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTotalsForRangeUsesStandalonePiContract(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.jsonl")
	main := `{"type":"session","id":"main","timestamp":"2026-09-01T00:00:00Z","cwd":"/repo"}
{"type":"message","id":"assistant-1","timestamp":"2026-09-10T01:00:00Z","message":{"role":"assistant","provider":"p","model":"m1","usage":{"input":10,"output":5,"cacheRead":2,"cacheWrite":1,"reasoning":3,"cost":{"total":0.1}}}}
{"type":"message","id":"assistant-1","timestamp":"2026-09-10T01:00:00Z","message":{"role":"assistant","provider":"p","model":"m1","usage":{"input":10,"output":8,"cacheRead":2,"cacheWrite":1,"reasoning":3,"cost":{"total":0.1}},"stopReason":"stop"}}
{"type":"message","id":"tool-1","timestamp":"2026-09-10T02:00:00Z","message":{"role":"toolResult","usage":{"input":2,"output":1}}}
{"type":"compaction","id":"compact-1","timestamp":"2026-09-11T01:00:00Z","usage":{"input":3,"output":4,"cacheRead":1,"cacheWrite":2}}
{"type":"branch_summary","id":"branch-1","timestamp":"2026-09-11T02:00:00Z","usage":{"input":4,"output":2,"cacheWrite":1}}
{"type":"message","id":"failed-1","timestamp":"2026-09-11T03:00:00Z","message":{"role":"assistant","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"cost":{"total":0}},"stopReason":"error"}}
{"type":"message","id":"aborted-1","timestamp":"2026-09-11T03:30:00Z","message":{"role":"assistant","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"cost":{"total":0}},"stopReason":"aborted"}}
{"type":"message","timestamp":"2026-09-11T04:00:00Z","message":{"role":"assistant","model":"m1","usage":{"input":5,"output":1},"stopReason":"stop","irrelevant":"first"}}
{"type":"message","timestamp":"2026-09-11T04:00:00Z","message":{"role":"assistant","model":"m1","usage":{"input":5,"output":1},"stopReason":"stop","irrelevant":"second"}}
{"type":"message","id":"outside-month","timestamp":"2026-10-02T00:00:00Z","message":{"role":"assistant","usage":{"input":100,"output":100},"stopReason":"stop"}}
`
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	forkPath := filepath.Join(dir, "fork.jsonl")
	fork := `{"type":"session","id":"fork","timestamp":"2026-09-12T00:00:00Z","parentSession":"/tmp/main.jsonl"}
{"type":"message","id":"copied","timestamp":"2026-09-11T23:59:59Z","message":{"role":"assistant","usage":{"input":999,"output":999},"stopReason":"stop"}}
{"type":"message","id":"fork-1","timestamp":"2026-09-12T01:00:00Z","message":{"role":"assistant","usage":{"input":6,"output":2},"stopReason":"stop"}}
`
	if err := os.WriteFile(forkPath, []byte(fork), 0o644); err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 30, 23, 59, 59, 0, time.UTC)
	totals, err := TotalsForRange(dir, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if totals.Requests != 8 || totals.Input != 30 || totals.Output != 18 || totals.CacheRead != 3 || totals.CacheWrite != 4 || totals.Reasoning != 3 || totals.TotalTokens != 51 || math.Abs(totals.Cost-0.1) > 1e-9 {
		t.Fatalf("standalone Pi audit contract drifted: %+v", totals)
	}
	monthTotals, err := TotalsForMonth(dir, 2026, 9)
	if err != nil {
		t.Fatal(err)
	}
	if monthTotals.Requests != totals.Requests || monthTotals.TotalTokens != totals.TotalTokens {
		t.Fatalf("month boundary must match the September range: range=%+v month=%+v", totals, monthTotals)
	}
}
