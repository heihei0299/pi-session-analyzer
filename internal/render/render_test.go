package render

import (
	"strings"
	"testing"

	"github.com/heihei0299/token-analyzer/internal/domain"
)

func TestRenderTotalsShowsUnpricedCostStatus(t *testing.T) {
	output := RenderTotalsTable(domain.Totals{Requests: 1, TotalTokens: 15, CostStatus: "unpriced"})
	if !strings.Contains(output, "unpriced") {
		t.Fatalf("expected unpriced cost status, got %q", output)
	}
}

func TestRenderTotalsShowsPartialCostAnnotation(t *testing.T) {
	output := RenderTotalsTable(domain.Totals{Requests: 2, TotalTokens: 44, Cost: 0.25, CostStatus: "unpriced"})
	if !strings.Contains(output, "$0.2500*") {
		t.Fatalf("partial all-source cost must keep known amount: %q", output)
	}
	if !strings.Contains(output, "含 unpriced 源") {
		t.Fatalf("partial all-source cost must explain unpriced source: %q", output)
	}
}
