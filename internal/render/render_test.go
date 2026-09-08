package render

import (
	"strings"
	"testing"

	"github.com/heihei0299/pi-session-anylize/internal/domain"
)

func TestRenderTotalsShowsUnpricedCostStatus(t *testing.T) {
	output := RenderTotalsTable(domain.Totals{Requests: 1, TotalTokens: 15, CostStatus: "unpriced"})
	if !strings.Contains(output, "unpriced") {
		t.Fatalf("expected unpriced cost status, got %q", output)
	}
}
