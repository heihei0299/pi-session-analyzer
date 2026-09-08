package main

import (
	"testing"

	"github.com/heihei0299/pi-session-anylize/internal/domain"
	"github.com/heihei0299/pi-session-anylize/internal/sessiondata"
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
