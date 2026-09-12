package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/domain"
	"github.com/heihei0299/token-analyzer/internal/query"
	"github.com/heihei0299/token-analyzer/internal/refresh"
	"github.com/heihei0299/token-analyzer/internal/sessiondata"
)

func TestJSONOutputDataIncludesMetaForGroupedAndPeriodResults(t *testing.T) {
	meta := &sessiondata.QueryMeta{Sources: []string{"codex"}, Warnings: []string{"fixture warning"}}
	tests := []struct {
		name   string
		result *sessiondata.QueryResult
	}{
		{
			name: "grouped",
			result: &sessiondata.QueryResult{
				Window: "totals",
				By:     domain.GroupByModel,
				Rows:   []domain.GroupRow{},
				Meta:   meta,
			},
		},
		{
			name: "period",
			result: &sessiondata.QueryResult{
				Window: "totals",
				Period: domain.PeriodDay,
				Rows:   []domain.PeriodRow{},
				Meta:   meta,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output := jsonOutputData(tc.result)
			if output["meta"] != meta {
				t.Fatalf("JSON output lost query metadata: %#v", output)
			}
		})
	}
}

// Watch totals 与同一时刻普通 Query totals 完全一致：两者走同一映射，
// 这里追加写入后分别经 watchTotalsOnce 与 Refresh+Query 取数并逐字段比对。
func TestWatchTotalsMatchQueryTotals(t *testing.T) {
	piDir := t.TempDir()
	piFile := filepath.Join(piDir, "project", "w.jsonl")
	if err := os.MkdirAll(filepath.Dir(piFile), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "{\"type\":\"session\",\"id\":\"w\",\"timestamp\":\"2026-09-10T00:00:00Z\",\"cwd\":\"/w\"}\n" +
		"{\"type\":\"message\",\"id\":\"a1\",\"timestamp\":\"2026-09-10T01:00:00Z\",\"message\":{\"role\":\"assistant\",\"model\":\"m\",\"usage\":{\"input\":4,\"output\":5}},\"stopReason\":\"stop\"}\n"
	if err := os.WriteFile(piFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := t.TempDir() + "/ledger.db"
	filter := sessiondata.Filter{Source: "pi"}

	equalTotals := func(a, b *domain.Totals) bool {
		return a.Requests == b.Requests && a.Input == b.Input && a.Output == b.Output &&
			a.CacheRead == b.CacheRead && a.CacheWrite == b.CacheWrite &&
			a.Reasoning == b.Reasoning && a.TotalTokens == b.TotalTokens && a.Cost == b.Cost
	}
	queryTotals := func() *domain.Totals {
		t.Helper()
		if err := refresh.Refresh(refresh.Config{PiDir: piDir, DBPath: dbPath, Source: "pi"}); err != nil {
			t.Fatal(err)
		}
		res, err := query.Query(query.Config{PiDir: piDir, DBPath: dbPath, Source: "pi"}, filter, sessiondata.View{Kind: sessiondata.ViewTotals})
		if err != nil || res.Totals == nil {
			t.Fatalf("query totals: %+v %v", res, err)
		}
		return res.Totals
	}

	watched, err := watchTotalsOnce(piDir, "", dbPath, filter)
	if err != nil {
		t.Fatal(err)
	}
	if plain := queryTotals(); !equalTotals(watched, plain) {
		t.Fatalf("watch totals must equal query totals: %+v vs %+v", watched, plain)
	}

	f, err := os.OpenFile(piFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("{\"type\":\"message\",\"id\":\"a2\",\"timestamp\":\"2026-09-10T02:00:00Z\",\"message\":{\"role\":\"assistant\",\"model\":\"m\",\"usage\":{\"input\":10,\"output\":20}},\"stopReason\":\"stop\"}\n")
	_ = f.Close()

	watched, err = watchTotalsOnce(piDir, "", dbPath, filter)
	if err != nil {
		t.Fatal(err)
	}
	if plain := queryTotals(); !equalTotals(watched, plain) {
		t.Fatalf("watch totals must equal query totals after append: %+v vs %+v", watched, plain)
	}
	if watched.Requests != 2 || watched.Input != 14 || watched.Output != 25 {
		t.Fatalf("unexpected watch totals after append: %+v", watched)
	}
}
